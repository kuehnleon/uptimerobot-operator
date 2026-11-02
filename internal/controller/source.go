package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
)

// AnnotationPrefix is the prefix of annotations configuring monitors on ingress resources.
// "<prefix>enabled: true" enables monitoring, any other "<prefix><field>" sets the Monitor spec field,
// e.g. "<prefix>interval: 60" or "<prefix>keyword.value: OK". Values are parsed as JSON if possible.
const AnnotationPrefix = "uptimerobot.kuehnleon.github.io/"

// sourceReconciler creates a Monitor for each annotated object of a kind.
type sourceReconciler struct {
	client.Client
	events events.EventRecorder
	newObj func() client.Object
	prefix string
	url    func(client.Object) (string, error)
}

func (r *sourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := r.newObj()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	m := &v1alpha1.Monitor{ObjectMeta: metav1.ObjectMeta{Name: r.prefix + obj.GetName(), Namespace: obj.GetNamespace()}}

	if obj.GetAnnotations()[AnnotationPrefix+"enabled"] != "true" {
		if err := r.Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		if metav1.IsControlledBy(m, obj) {
			return ctrl.Result{}, client.IgnoreNotFound(r.Delete(ctx, m))
		}
		return ctrl.Result{}, nil
	}

	var spec v1alpha1.MonitorSpec
	u, err := r.url(obj)
	if v, ok := obj.GetAnnotations()[AnnotationPrefix+"url"]; ok {
		u, err = v, nil
	}
	if err == nil {
		spec, err = monitorSpec(u, obj.GetAnnotations())
	}
	if err != nil {
		r.events.Eventf(obj, nil, corev1.EventTypeWarning, "InvalidMonitor", "CreateMonitor", "%v", err)
		return ctrl.Result{}, reconcile.TerminalError(err)
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, m, func() error {
		if !m.CreationTimestamp.IsZero() && !metav1.IsControlledBy(m, obj) {
			return apierrors.NewAlreadyExists(v1alpha1.GroupVersion.WithResource("monitors").GroupResource(), m.Name)
		}
		m.Labels = obj.GetLabels()
		m.Spec = spec
		return controllerutil.SetControllerReference(obj, m, r.Scheme())
	})
	return ctrl.Result{}, err
}

// monitorSpec builds a Monitor spec for rawURL, overridden by annotations.
func monitorSpec(rawURL string, annotations map[string]string) (v1alpha1.MonitorSpec, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return v1alpha1.MonitorSpec{}, err
	}
	fields := map[string]any{"type": "HTTP", "url": rawURL, "friendlyName": u.Host, "interval": DefaultMonitorInterval}
	raw := map[string]string{}
	for k, v := range annotations {
		path, ok := strings.CutPrefix(k, AnnotationPrefix)
		if !ok || path == "enabled" {
			continue
		}
		var val any = v
		if json.Unmarshal([]byte(v), &val) != nil {
			val = v
		}
		raw[path] = v
		setPath(fields, strings.Split(path, "."), val)
	}

	// Retry with the raw string for string fields given values that parse as JSON numbers or booleans.
	for {
		var spec v1alpha1.MonitorSpec
		b, _ := json.Marshal(fields)
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		err := dec.Decode(&spec)
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Type.Kind() == reflect.String {
			if v, ok := raw[typeErr.Field]; ok {
				setPath(fields, strings.Split(typeErr.Field, "."), v)
				delete(raw, typeErr.Field)
				continue
			}
		}
		return spec, err
	}
}

func setPath(m map[string]any, path []string, v any) {
	for _, p := range path[:len(path)-1] {
		next, ok := m[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[path[len(path)-1]] = v
}

func ingressURL(o client.Object) (string, error) {
	ing := o.(*networkingv1.Ingress)
	for _, rule := range ing.Spec.Rules {
		if rule.Host == "" || strings.Contains(rule.Host, "*") {
			continue
		}
		scheme := "http"
		if slices.ContainsFunc(ing.Spec.TLS, func(t networkingv1.IngressTLS) bool { return slices.Contains(t.Hosts, rule.Host) }) {
			scheme = "https"
		}
		return scheme + "://" + rule.Host, nil
	}
	return "", errors.New("no host found in ingress rules, set the url annotation")
}

var (
	hostRule = regexp.MustCompile(`\bHost\(([^)]*)\)`)
	quoted   = regexp.MustCompile("[`\"]([^`\"]+)[`\"]")
)

func ingressRouteURL(o client.Object) (string, error) {
	u := o.(*unstructured.Unstructured)
	routes, _, _ := unstructured.NestedSlice(u.Object, "spec", "routes")
	scheme := "http"
	if _, ok, _ := unstructured.NestedFieldNoCopy(u.Object, "spec", "tls"); ok {
		scheme = "https"
	}
	for _, route := range routes {
		match, _, _ := unstructured.NestedString(route.(map[string]any), "match")
		if m := hostRule.FindStringSubmatch(match); m != nil {
			if h := quoted.FindStringSubmatch(m[1]); h != nil {
				return scheme + "://" + h[1], nil
			}
		}
	}
	return "", errors.New("no Host() rule found in routes, set the url annotation")
}

func setupSource(mgr ctrl.Manager, name, prefix string, newObj func() client.Object, url func(client.Object) (string, error)) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(newObj()).
		Owns(&v1alpha1.Monitor{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(&sourceReconciler{
			Client: mgr.GetClient(), events: mgr.GetEventRecorder("uptimerobot-operator"), newObj: newObj, prefix: prefix, url: url,
		})
}

func setupIngressRoute(mgr ctrl.Manager, gvk schema.GroupVersionKind) error {
	if _, err := mgr.GetRESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version); err != nil {
		ctrl.Log.Info("Traefik IngressRoute CRD not found, skipping", "group", gvk.Group)
		return nil
	}
	newObj := func() client.Object {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		return u
	}
	return setupSource(mgr, "ingressroute."+gvk.Group, "ingressroute-", newObj, ingressRouteURL)
}
