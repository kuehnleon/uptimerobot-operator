package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
)

func setupMaintenanceWindow(mgr ctrl.Manager, o Options) error {
	r := &syncer[*v1alpha1.MaintenanceWindow]{
		Client:     mgr.GetClient(),
		ur:         o.Client,
		events:     mgr.GetEventRecorder("uptimerobot-operator"),
		interval:   o.SyncInterval,
		path:       "/maintenance-windows",
		newObj:     func() *v1alpha1.MaintenanceWindow { return &v1alpha1.MaintenanceWindow{} },
		drift:      []string{"name", "interval", "time", "duration", "status"},
		createOmit: []string{"status"},
		desired: func(ctx context.Context, w *v1alpha1.MaintenanceWindow) (map[string]any, error) {
			s := w.Spec
			b := map[string]any{
				"name":     or(s.FriendlyName, w.Name),
				"interval": s.Interval,
				"date":     s.Date,
				"time":     s.Time,
				"duration": s.Duration,
				"status":   map[bool]string{true: "paused", false: "active"}[s.Paused],
			}
			if len(s.Days) > 0 {
				b["days"] = s.Days
			}
			if s.MonitorSelector != nil {
				ids, err := monitorIDs(ctx, mgr.GetClient(), w.Namespace, s.MonitorSelector)
				if err != nil {
					return nil, err
				}
				b["monitorIds"] = ids
			}
			return b, nil
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.MaintenanceWindow{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&v1alpha1.Monitor{}, enqueueAll(mgr.GetClient(), func() client.ObjectList { return &v1alpha1.MaintenanceWindowList{} })).
		Complete(r)
}
