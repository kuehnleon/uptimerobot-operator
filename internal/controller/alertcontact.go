package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
)

func setupAlertContact(mgr ctrl.Manager, o Options) error {
	r := &syncer[*v1alpha1.AlertContact]{
		Client:               mgr.GetClient(),
		reader:               mgr.GetAPIReader(),
		ur:                   o.Client,
		events:               mgr.GetEventRecorder("uptimerobot-operator"),
		interval:             o.SyncInterval,
		path:                 "/alert-contacts",
		newObj:               func() *v1alpha1.AlertContact { return &v1alpha1.AlertContact{} },
		drift:                []string{"friendlyName", "enableNotificationsFor", "sslExpirationReminder", "isActive"},
		hashOmit:             []string{"type", "value", "pushToken", "platform", "deviceName"},
		createOmit:           []string{"sslExpirationReminder", "isActive"},
		createUpdateRequired: []string{"sslExpirationReminder", "isActive"},
		updateOmit:           []string{"type", "value", "pushToken", "platform", "deviceName"},
		desired: func(ctx context.Context, a *v1alpha1.AlertContact) (map[string]any, error) {
			s := a.Spec
			b := map[string]any{
				"type":         s.Type,
				"friendlyName": or(s.FriendlyName, a.Name),
				"isActive":     !s.Paused,
			}
			opt(b, "enableNotificationsFor", s.EnableNotificationsFor)
			opt(b, "sslExpirationReminder", s.SSLExpirationReminder)
			if s.Type == "Email" {
				b["value"] = s.Value
			} else {
				pushToken, err := secretGetter(mgr.GetAPIReader(), a.Namespace)(ctx, s.PushTokenSecretRef)
				if err != nil {
					return nil, err
				}
				b["pushToken"] = pushToken
				b["platform"] = map[string]string{"MobileAppIOS": "ios", "MobileAppAndroid": "android"}[s.Type]
				if s.DeviceName != "" {
					b["deviceName"] = s.DeviceName
				}
			}
			return b, nil
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AlertContact{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
