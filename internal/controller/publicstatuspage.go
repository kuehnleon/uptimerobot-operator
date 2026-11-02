package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
	"github.com/kuehnleon/uptimerobot-operator/internal/uptimerobot"
)

func setupPublicStatusPage(mgr ctrl.Manager, o Options) error {
	r := &syncer[*v1alpha1.PublicStatusPage]{
		Client:   mgr.GetClient(),
		ur:       o.Client,
		events:   mgr.GetEventRecorder("uptimerobot-operator"),
		interval: o.SyncInterval,
		path:     "/psps",
		newObj:   func() *v1alpha1.PublicStatusPage { return &v1alpha1.PublicStatusPage{} },
		drift:    []string{"friendlyName", "customDomain", "status"},
		desired: func(ctx context.Context, p *v1alpha1.PublicStatusPage) (map[string]any, error) {
			s := p.Spec
			b := map[string]any{
				"friendlyName": or(s.FriendlyName, p.Name),
				"status":       map[bool]string{true: "PAUSED", false: "ENABLED"}[s.Paused],
			}
			if s.CustomDomain != "" {
				b["customDomain"] = s.CustomDomain
			}
			if s.Sort != "" {
				b["sort"] = s.Sort
			}
			opt(b, "hideUrlLinks", s.HideURLLinks)
			opt(b, "noIndex", s.NoIndex)
			if s.PasswordSecretRef != nil {
				pw, err := secretGetter(mgr.GetAPIReader(), p.Namespace)(ctx, s.PasswordSecretRef)
				if err != nil {
					return nil, err
				}
				b["password"] = pw
			}
			if s.MonitorSelector != nil {
				ids, err := monitorIDs(ctx, mgr.GetClient(), p.Namespace, s.MonitorSelector)
				if err != nil {
					return nil, err
				}
				b["monitorIds"] = ids
			}
			return b, nil
		},
		observe: func(p *v1alpha1.PublicStatusPage, remote uptimerobot.Object) {
			p.Status.URL = "https://stats.uptimerobot.com/" + remote.String("urlKey")
			if d := remote.String("customDomain"); d != "" {
				p.Status.URL = "https://" + d
			}
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PublicStatusPage{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&v1alpha1.Monitor{}, enqueueAll(mgr.GetClient(), func() client.ObjectList { return &v1alpha1.PublicStatusPageList{} })).
		Complete(r)
}
