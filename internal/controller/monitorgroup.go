package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
)

func setupMonitorGroup(mgr ctrl.Manager, o Options) error {
	r := &syncer[*v1alpha1.MonitorGroup]{
		Client:   mgr.GetClient(),
		reader:   mgr.GetAPIReader(),
		ur:       o.Client,
		events:   mgr.GetEventRecorder("uptimerobot-operator"),
		interval: o.SyncInterval,
		path:     "/monitor-groups",
		newObj:   func() *v1alpha1.MonitorGroup { return &v1alpha1.MonitorGroup{} },
		drift:    []string{"name"},
		desired: func(_ context.Context, g *v1alpha1.MonitorGroup) (map[string]any, error) {
			return map[string]any{"name": or(g.Spec.FriendlyName, g.Name)}, nil
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.MonitorGroup{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
