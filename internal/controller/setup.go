package controller

import (
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kuehnleon/uptimerobot-operator/internal/uptimerobot"
)

type Options struct {
	Client       *uptimerobot.Client
	SyncInterval time.Duration
	Ingress      bool
	Traefik      bool
}

// Setup registers all controllers with the manager.
func Setup(mgr ctrl.Manager, o Options) error {
	for _, setup := range []func(ctrl.Manager, Options) error{setupMonitor, setupMaintenanceWindow, setupPublicStatusPage} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	if o.Ingress {
		newIngress := func() client.Object { return &networkingv1.Ingress{} }
		if err := setupSource(mgr, "ingress", "ingress-", newIngress, ingressURL); err != nil {
			return err
		}
	}
	if o.Traefik {
		for _, group := range []string{"traefik.io", "traefik.containo.us"} {
			if err := setupIngressRoute(mgr, schema.GroupVersionKind{Group: group, Version: "v1alpha1", Kind: "IngressRoute"}); err != nil {
				return err
			}
		}
	}
	return nil
}
