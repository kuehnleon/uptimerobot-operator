package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
	"github.com/kuehnleon/uptimerobot-operator/internal/controller"
	"github.com/kuehnleon/uptimerobot-operator/internal/uptimerobot"
)

func main() {
	var (
		o                      controller.Options
		apiURL                 string
		rateLimit              int
		metricsAddr, probeAddr string
		leaderElect            bool
	)
	flag.DurationVar(&o.SyncInterval, "sync-interval", time.Minute, "Interval in which resources are synced with UptimeRobot.")
	flag.IntVar(&rateLimit, "api-rate-limit", 10, "Maximum UptimeRobot API requests per minute (FREE plan: 10).")
	flag.StringVar(&apiURL, "api-url", uptimerobot.DefaultURL, "UptimeRobot API URL.")
	flag.BoolVar(&o.Ingress, "ingress", true, "Create monitors for annotated Ingresses.")
	flag.BoolVar(&o.Traefik, "traefik", true, "Create monitors for annotated Traefik IngressRoutes (if the CRD is installed).")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Metrics endpoint address, 0 to disable.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Health probe endpoint address.")
	flag.BoolVar(&leaderElect, "leader-elect", false, "Enable leader election.")
	zapOpts := zap.Options{}
	zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
	log := ctrl.Log.WithName("setup")

	apiKey := os.Getenv("UPTIMEROBOT_API_KEY")
	if apiKey == "" {
		log.Info("UPTIMEROBOT_API_KEY must be set")
		os.Exit(1)
	}
	o.Client = uptimerobot.New(apiURL, apiKey, rateLimit, o.SyncInterval)

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                        scheme,
		Metrics:                       metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress:        probeAddr,
		LeaderElection:                leaderElect,
		LeaderElectionID:              "uptimerobot-operator.kuehnleon.github.io",
		LeaderElectionReleaseOnCancel: true,
		Client:                        client.Options{Cache: &client.CacheOptions{Unstructured: true}},
	})
	if err != nil {
		log.Error(err, "unable to create manager")
		os.Exit(1)
	}
	if err := controller.Setup(mgr, o); err != nil {
		log.Error(err, "unable to set up controllers")
		os.Exit(1)
	}
	_ = mgr.AddHealthzCheck("healthz", healthz.Ping)
	_ = mgr.AddReadyzCheck("readyz", healthz.Ping)

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "manager exited")
		os.Exit(1)
	}
}
