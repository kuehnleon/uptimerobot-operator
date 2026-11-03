package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
	"github.com/kuehnleon/uptimerobot-operator/internal/uptimerobot"
)

func TestDeleteRetry(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set")
	}
	env := &envtest.Environment{CRDDirectoryPaths: []string{"../../charts/uptimerobot-operator/crds"}, ErrorIfCRDPathMissing: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = env.Stop() })
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	api := newFakeAPI()
	var dropped atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && dropped.CompareAndSwap(false, true) {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		api.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	// Controller names are global across managers in the test process.
	skipNameValidation := true
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme, Metrics: metricsserver.Options{BindAddress: "0"},
		Controller: config.Controller{SkipNameValidation: &skipNameValidation},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Setup(mgr, Options{Client: uptimerobot.New(srv.URL, "key", 6000, time.Hour), SyncInterval: 2 * time.Second}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("manager stopped with error: %v", err)
		}
	})
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	m := &v1alpha1.Monitor{ObjectMeta: metav1.ObjectMeta{Name: "m", Namespace: "default"},
		Spec: v1alpha1.MonitorSpec{Type: "HEARTBEAT", Interval: 300}}
	if err := c.Create(ctx, m); err != nil {
		t.Fatal(err)
	}
	wait := func(what string, timeout time.Duration, cond func() error) {
		t.Helper()
		var err error
		for start := time.Now(); time.Since(start) < timeout; time.Sleep(100 * time.Millisecond) {
			if err = cond(); err == nil {
				return
			}
		}
		t.Fatalf("%s: %v", what, err)
	}
	wait("created", 10*time.Second, func() error {
		if err := c.Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
			return err
		}
		if m.Status.ID == 0 {
			return fmt.Errorf("no id")
		}
		return nil
	})
	if err := c.Delete(ctx, m); err != nil {
		t.Fatal(err)
	}
	wait("deleted after retry", 10*time.Second, func() error {
		if !dropped.Load() {
			return fmt.Errorf("delete not attempted")
		}
		if r := api.get("monitors", m.Status.ID); r != nil {
			return fmt.Errorf("remote still exists")
		}
		if err := c.Get(ctx, client.ObjectKeyFromObject(m), m); !apierrors.IsNotFound(err) {
			return fmt.Errorf("resource still exists (get error: %v)", err)
		}
		return nil
	})
}
