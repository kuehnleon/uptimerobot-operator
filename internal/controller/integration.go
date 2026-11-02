package controller

import (
	"context"
	"encoding/json"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
)

func setupIntegration(mgr ctrl.Manager, o Options) error {
	r := &syncer[*v1alpha1.Integration]{
		Client:   mgr.GetClient(),
		reader:   mgr.GetAPIReader(),
		ur:       o.Client,
		events:   mgr.GetEventRecorder("uptimerobot-operator"),
		interval: o.SyncInterval,
		path:     "/integrations",
		newObj:   func() *v1alpha1.Integration { return &v1alpha1.Integration{} },
		desired: func(ctx context.Context, i *v1alpha1.Integration) (map[string]any, error) {
			return integrationBody(ctx, secretGetter(mgr.GetAPIReader(), i.Namespace), i)
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Integration{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

func integrationBody(ctx context.Context, secret secretFunc, i *v1alpha1.Integration) (map[string]any, error) {
	s := i.Spec
	data := map[string]any{}
	if s.Config != nil && len(s.Config.Raw) > 0 {
		if err := json.Unmarshal(s.Config.Raw, &data); err != nil {
			return nil, err
		}
	}
	for key, ref := range s.SecretConfig {
		v, err := secret(ctx, &ref)
		if err != nil {
			return nil, err
		}
		data[key] = v
	}
	data["friendlyName"] = or(s.FriendlyName, i.Name)
	data["enableNotificationsFor"] = or(s.EnableNotificationsFor, "UpAndDown")
	opt(data, "sslExpirationReminder", s.SSLExpirationReminder)
	return map[string]any{"type": s.Type, "data": data}, nil
}
