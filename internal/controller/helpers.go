package controller

import (
	"context"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
)

type secretFunc func(ctx context.Context, ref *corev1.SecretKeySelector) (string, error)

// secretGetter reads secrets uncached, so the operator does not need to list and watch all secrets.
func secretGetter(r client.Reader, namespace string) secretFunc {
	return func(ctx context.Context, ref *corev1.SecretKeySelector) (string, error) {
		var s corev1.Secret
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &s); err != nil {
			return "", err
		}
		v, ok := s.Data[ref.Key]
		if !ok {
			return "", fmt.Errorf("key %q not found in secret %s/%s", ref.Key, namespace, ref.Name)
		}
		return string(v), nil
	}
}

// monitorIDs returns the sorted UptimeRobot IDs of the Monitors in namespace matching selector.
func monitorIDs(ctx context.Context, c client.Reader, namespace string, selector *metav1.LabelSelector) ([]int64, error) {
	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}
	var list v1alpha1.MonitorList
	if err := c.List(ctx, &list, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: sel}); err != nil {
		return nil, err
	}
	ids := []int64{}
	for _, m := range list.Items {
		if m.Status.ID != 0 {
			ids = append(ids, m.Status.ID)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// enqueueAll returns a handler enqueueing all objects of list in the namespace of the event's object.
func enqueueAll(c client.Client, newList func() client.ObjectList) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
		list := newList()
		if err := c.List(ctx, list, client.InNamespace(o.GetNamespace())); err != nil {
			return nil
		}
		var reqs []reconcile.Request
		_ = meta.EachListItem(list, func(item runtime.Object) error {
			obj := item.(client.Object)
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(obj)})
			return nil
		})
		return reqs
	})
}

func opt[V any](b map[string]any, key string, v *V) {
	if v != nil {
		b[key] = *v
	}
}

func deref[V any](v *V) V {
	var zero V
	if v == nil {
		return zero
	}
	return *v
}

func or[V comparable](v, fallback V) V {
	var zero V
	if v == zero {
		return fallback
	}
	return v
}
