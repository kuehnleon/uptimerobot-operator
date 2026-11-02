package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
	"github.com/kuehnleon/uptimerobot-operator/internal/uptimerobot"
)

const finalizer = "uptimerobot.kuehnleon.github.io/finalizer"

type object interface {
	client.Object
	GetResourceStatus() *v1alpha1.ResourceStatus
}

// syncer reconciles a custom resource with its UptimeRobot counterpart at path.
type syncer[T object] struct {
	client.Client
	// reader reads uncached from the API server.
	reader   client.Reader
	ur       *uptimerobot.Client
	events   events.EventRecorder
	interval time.Duration

	path   string
	newObj func() T
	// desired returns the API request body.
	desired func(ctx context.Context, obj T) (map[string]any, error)
	// drift lists body keys compared against the remote object to detect changes made outside the cluster.
	drift []string
	// createOmit lists body keys not accepted on create; they are applied by a follow-up update.
	createOmit []string
	// after is called with the up-to-date remote object and may return a modified one.
	after func(ctx context.Context, obj T, remote uptimerobot.Object) (uptimerobot.Object, error)
	// observe copies kind specific fields of the remote object into the status.
	observe func(obj T, remote uptimerobot.Object)

	retries retries
}

func (s *syncer[T]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := s.newObj()
	if err := s.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if d := s.retries.wait(req.NamespacedName, obj.GetGeneration()); d > 0 {
		return ctrl.Result{RequeueAfter: d}, nil
	}
	st := obj.GetResourceStatus()

	if !obj.GetDeletionTimestamp().IsZero() {
		if !controllerutil.ContainsFinalizer(obj, finalizer) {
			return ctrl.Result{}, nil
		}
		if st.ID != 0 {
			if err := s.ur.Delete(ctx, s.path, st.ID); err != nil {
				return s.fail(ctx, obj, obj.DeepCopyObject().(T), err)
			}
			s.events.Eventf(obj, nil, corev1.EventTypeNormal, "Deleted", "Delete", "Deleted from UptimeRobot (ID %d)", st.ID)
		}
		s.retries.reset(req.NamespacedName)
		controllerutil.RemoveFinalizer(obj, finalizer)
		return ctrl.Result{}, client.IgnoreNotFound(s.Update(ctx, obj))
	}
	if controllerutil.AddFinalizer(obj, finalizer) {
		if err := s.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}
	orig := obj.DeepCopyObject().(T)
	if c := meta.FindStatusCondition(st.Conditions, "Ready"); c == nil || c.ObservedGeneration != obj.GetGeneration() {
		setReady(obj, metav1.ConditionUnknown, "Progressing", "Syncing with UptimeRobot")
		if err := s.Status().Patch(ctx, obj, client.MergeFrom(orig)); err != nil {
			return ctrl.Result{}, err
		}
		orig = obj.DeepCopyObject().(T)
	}

	body, err := s.desired(ctx, obj)
	if err != nil {
		return s.fail(ctx, obj, orig, err)
	}
	hash := s.ur.Hash(body)

	var remote uptimerobot.Object
	var action string
	if st.ID != 0 {
		if remote, err = s.ur.Get(ctx, s.path, st.ID); err != nil {
			return s.fail(ctx, obj, orig, err)
		}
		if remote == nil {
			ctrl.LoggerFrom(ctx).Info("remote object not found, recreating", "id", st.ID)
		}
	}
	if remote != nil && (st.Hash != hash || drifted(s.drift, body, remote)) {
		remote, err = s.ur.Update(ctx, s.path, st.ID, body)
		action = "Updated"
		if uptimerobot.IsNotFound(err) {
			remote, err = nil, nil
		}
	}
	if err == nil && remote == nil {
		// The cache may not yet contain the ID stored by the previous reconcile; never create twice.
		latest := s.newObj()
		if err := s.reader.Get(ctx, client.ObjectKeyFromObject(obj), latest); err != nil {
			return s.fail(ctx, obj, orig, err)
		}
		if latest.GetResourceStatus().ID != st.ID {
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
		create := maps.Clone(body)
		for _, k := range s.createOmit {
			delete(create, k)
		}
		remote, err = s.ur.Create(ctx, s.path, create)
		action = "Created"
		if err == nil && drifted(s.createOmit, body, remote) {
			remote, err = s.ur.Update(ctx, s.path, remote.ID(), body)
		}
	}
	if err == nil && s.after != nil {
		remote, err = s.after(ctx, obj, remote)
	}
	if err != nil {
		return s.fail(ctx, obj, orig, err)
	}

	s.retries.reset(client.ObjectKeyFromObject(obj))
	st.ID = remote.ID()
	st.Hash = hash
	st.ObservedGeneration = obj.GetGeneration()
	setReady(obj, metav1.ConditionTrue, "Synced", fmt.Sprintf("Synced with UptimeRobot (ID %d)", st.ID))
	if action != "" {
		s.events.Eventf(obj, nil, corev1.EventTypeNormal, action, strings.TrimSuffix(action, "d"), "%s in UptimeRobot (ID %d)", action, st.ID)
	}
	if s.observe != nil {
		s.observe(obj, remote)
	}
	return ctrl.Result{RequeueAfter: s.interval}, s.Status().Patch(ctx, obj, client.MergeFrom(orig))
}

// fail records err in the Ready condition and schedules a retry with exponential backoff
// starting at the sync interval, to protect the API rate limit.
func (s *syncer[T]) fail(ctx context.Context, obj, orig T, err error) (ctrl.Result, error) {
	reason := failureReason(err)
	setReady(obj, metav1.ConditionFalse, reason, err.Error())
	s.events.Eventf(obj, nil, corev1.EventTypeWarning, reason, "Sync", "%v", err)
	if perr := s.Status().Patch(ctx, obj, client.MergeFrom(orig)); perr != nil {
		return ctrl.Result{}, errors.Join(err, perr)
	}
	ctrl.LoggerFrom(ctx).Error(err, "sync failed")
	return ctrl.Result{RequeueAfter: s.retries.failed(client.ObjectKeyFromObject(obj), obj.GetGeneration(), s.interval)}, nil
}

func setReady(obj object, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&obj.GetResourceStatus().Conditions, metav1.Condition{
		Type: "Ready", Status: status, Reason: reason, Message: message, ObservedGeneration: obj.GetGeneration(),
	})
}

func failureReason(err error) string {
	var e *uptimerobot.APIError
	switch {
	case !errors.As(err, &e) || e.StatusCode >= 500:
		return "Failed"
	case e.StatusCode == http.StatusTooManyRequests:
		return "RateLimited"
	default:
		return "Rejected"
	}
}

// retries tracks failed objects so that unrelated events do not trigger further API calls before the next retry.
type retries struct {
	mu sync.Mutex
	m  map[types.NamespacedName]retry
}

type retry struct {
	at         time.Time
	n          int
	generation int64
}

// wait returns the time until the next retry, or 0 if there is none or the object changed since it failed.
func (r *retries) wait(key types.NamespacedName, generation int64) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.m[key]; ok && e.generation == generation {
		return time.Until(e.at)
	}
	return 0
}

func (r *retries) failed(key types.NamespacedName, generation int64, interval time.Duration) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = map[types.NamespacedName]retry{}
	}
	e := r.m[key]
	d := interval << min(e.n, 5)
	r.m[key] = retry{at: time.Now().Add(d), n: e.n + 1, generation: generation}
	return d
}

func (r *retries) reset(key types.NamespacedName) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, key)
}

func drifted(keys []string, want map[string]any, got uptimerobot.Object) bool {
	return slices.ContainsFunc(keys, func(k string) bool {
		w, ok := want[k]
		return ok && got[k] != nil && normalize(k, w) != normalize(k, got[k])
	})
}

func normalize(key string, v any) string {
	if s, ok := v.(string); ok && key == "url" {
		s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
		return strings.TrimSuffix(s, "/")
	}
	b, _ := json.Marshal(v)
	return string(b)
}
