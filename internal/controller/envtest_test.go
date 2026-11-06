package controller

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
	"github.com/kuehnleon/uptimerobot-operator/internal/uptimerobot"
)

// TestIntegration runs all controllers against a real API server (envtest) and a fake UptimeRobot API.
// Requires KUBEBUILDER_ASSETS, see setup-envtest.
func TestIntegration(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set")
	}
	env := &envtest.Environment{CRDDirectoryPaths: []string{"../../charts/uptimerobot-operator/crds"}, ErrorIfCRDPathMissing: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = env.Stop() }()

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	api := newFakeAPI()
	srv := httptest.NewServer(api)
	defer srv.Close()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, Metrics: metricsserver.Options{BindAddress: "0"}})
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Client: uptimerobot.New(srv.URL, "key", 6000, time.Hour), SyncInterval: time.Minute, Ingress: true, Traefik: true, GatewayAPI: true}
	if err := Setup(mgr, o); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = mgr.Start(ctx) }()
	c, _ := client.New(cfg, client.Options{Scheme: scheme})

	eventually := func(what string, cond func() error) {
		t.Helper()
		var err error
		for range 100 {
			if err = cond(); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("%s: %v", what, err)
	}

	invalid := &v1alpha1.Monitor{ObjectMeta: metav1.ObjectMeta{Name: "invalid", Namespace: "default"}, Spec: v1alpha1.MonitorSpec{Type: "KEYWORD", URL: "x"}}
	if err := c.Create(ctx, invalid); err == nil || !strings.Contains(err.Error(), "keyword is required") {
		t.Fatalf("expected CEL validation error, got %v", err)
	}
	invalidContact := &v1alpha1.Monitor{ObjectMeta: metav1.ObjectMeta{Name: "invalid", Namespace: "default"},
		Spec: v1alpha1.MonitorSpec{Type: "HTTP", URL: "x", AlertContacts: []v1alpha1.MonitorAlertContact{
			{ID: 1, IntegrationRef: &corev1.LocalObjectReference{Name: "hook"}}}}}
	if err := c.Create(ctx, invalidContact); err == nil || !strings.Contains(err.Error(), "exactly one of id, integrationRef or alertContactRef") {
		t.Fatalf("expected CEL validation error, got %v", err)
	}
	invalidMW := &v1alpha1.MaintenanceWindow{ObjectMeta: metav1.ObjectMeta{Name: "invalid", Namespace: "default"},
		Spec: v1alpha1.MaintenanceWindowSpec{Interval: "weekly", Date: "2026-01-01", Time: "03:00:00", Duration: 30}}
	if err := c.Create(ctx, invalidMW); err == nil || !strings.Contains(err.Error(), "date is required for once") ||
		!strings.Contains(err.Error(), "days are required") {
		t.Fatalf("expected CEL validation errors, got %v", err)
	}

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", Labels: map[string]string{"app": "web"},
			Annotations: map[string]string{AnnotationPrefix + "enabled": "true", AnnotationPrefix + "interval": "60",
				AnnotationPrefix + "alertContacts": `[{"integrationRef": {"name": "hook"}}, {"alertContactRef": {"name": "mobile"}}]`,
				AnnotationPrefix + "groupRef.name": "team"}},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{Host: "web.example.com"}},
			TLS:   []networkingv1.IngressTLS{{Hosts: []string{"web.example.com"}}},
		},
	}
	if err := c.Create(ctx, ing); err != nil {
		t.Fatal(err)
	}
	// Created after the referencing Monitor, which must wait for it.
	hook := &v1alpha1.Integration{ObjectMeta: metav1.ObjectMeta{Name: "hook", Namespace: "default"},
		Spec: v1alpha1.IntegrationSpec{Type: "Webhook", Config: &runtime.RawExtension{Raw: []byte(`{"postValue": "down"}`)},
			SecretConfig: map[string]corev1.SecretKeySelector{"urlToNotify": {
				LocalObjectReference: corev1.LocalObjectReference{Name: "hook"}, Key: "url"}}}}
	hookSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "hook", Namespace: "default"},
		StringData: map[string]string{"url": "https://hooks.example.com/secret"}}
	pushSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "mobile", Namespace: "default"},
		StringData: map[string]string{"token": "private-push-token"}}
	contact := &v1alpha1.AlertContact{ObjectMeta: metav1.ObjectMeta{Name: "mobile", Namespace: "default"},
		Spec: v1alpha1.AlertContactSpec{Type: "MobileAppIOS", PushTokenSecretRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "mobile"}, Key: "token"}, DeviceName: "test device"}}
	email := &v1alpha1.AlertContact{ObjectMeta: metav1.ObjectMeta{Name: "on-call", Namespace: "default"},
		Spec: v1alpha1.AlertContactSpec{Type: "Email", Value: "on-call@example.com"}}
	team := &v1alpha1.MonitorGroup{ObjectMeta: metav1.ObjectMeta{Name: "team", Namespace: "default"}}
	for _, obj := range []client.Object{hookSecret, hook, pushSecret, contact, email, team} {
		if err := c.Create(ctx, obj); err != nil {
			t.Fatal(err)
		}
	}
	eventually("personal alert contact synced", func() error {
		_ = c.Get(ctx, client.ObjectKeyFromObject(contact), contact)
		r := api.get("alert-contacts", contact.Status.ID)
		if r["pushToken"] != "private-push-token" || r["platform"] != "ios" {
			return fmt.Errorf("remote alert contact %v, status %+v", r, contact.Status)
		}
		if strings.Contains(fmt.Sprint(contact.Status), "private-push-token") {
			return fmt.Errorf("push token leaked into status: %+v", contact.Status)
		}
		return nil
	})
	eventually("email alert contact synced", func() error {
		_ = c.Get(ctx, client.ObjectKeyFromObject(email), email)
		if r := api.get("alert-contacts", email.Status.ID); r["value"] != "on-call@example.com" || r["type"] != "Email" {
			return fmt.Errorf("remote alert contact %v", r)
		}
		return nil
	})
	contact.Spec.Paused = true
	if err := c.Update(ctx, contact); err != nil {
		t.Fatal(err)
	}
	eventually("personal alert contact updated", func() error {
		_ = c.Get(ctx, client.ObjectKeyFromObject(contact), contact)
		if r := api.get("alert-contacts", contact.Status.ID); r["isActive"] != false {
			return fmt.Errorf("remote alert contact %v", r)
		}
		return nil
	})
	eventually("integration synced", func() error {
		_ = c.Get(ctx, client.ObjectKeyFromObject(hook), hook)
		r := api.get("integrations", hook.Status.ID)
		if data, _ := r["data"].(map[string]any); data["urlToNotify"] != "https://hooks.example.com/secret" || data["postValue"] != "down" {
			return fmt.Errorf("remote %v", r)
		}
		return nil
	})
	m := &v1alpha1.Monitor{}
	eventually("monitor synced", func() error {
		if err := c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "ingress-web"}, m); err != nil {
			return err
		}
		_ = c.Get(ctx, client.ObjectKeyFromObject(team), team)
		r := api.get("monitors", m.Status.ID)
		contacts := fmt.Sprint(r["assignedAlertContacts"])
		if r["url"] != "https://web.example.com" || r["interval"] != float64(60) || m.Status.State != "UP" ||
			contacts != fmt.Sprintf("[map[alertContactId:%d recurrence:0 threshold:0] map[alertContactId:%d recurrence:0 threshold:0]]", hook.Status.ID, contact.Status.ID) ||
			r["groupId"] != float64(team.Status.ID) || api.get("monitor-groups", team.Status.ID)["name"] != "team" {
			return fmt.Errorf("remote %v, status %+v", r, m.Status)
		}
		return nil
	})

	psp := &v1alpha1.PublicStatusPage{ObjectMeta: metav1.ObjectMeta{Name: "status", Namespace: "default"},
		Spec: v1alpha1.PublicStatusPageSpec{MonitorSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}}}}
	mw := &v1alpha1.MaintenanceWindow{ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "default"},
		Spec: v1alpha1.MaintenanceWindowSpec{Interval: "daily", Time: "03:00:00", Duration: 30, Paused: true,
			MonitorSelector: &metav1.LabelSelector{}}}
	for _, obj := range []client.Object{psp, mw} {
		if err := c.Create(ctx, obj); err != nil {
			t.Fatal(err)
		}
	}
	wantIDs := fmt.Sprint([]any{float64(m.Status.ID)})
	eventually("psp synced", func() error {
		_ = c.Get(ctx, client.ObjectKeyFromObject(psp), psp)
		if r := api.get("psps", psp.Status.ID); fmt.Sprint(r["monitorIds"]) != wantIDs || r["friendlyName"] != "status" {
			return fmt.Errorf("remote %v", r)
		}
		return nil
	})
	eventually("maintenance window synced", func() error {
		_ = c.Get(ctx, client.ObjectKeyFromObject(mw), mw)
		if r := api.get("maintenance-windows", mw.Status.ID); fmt.Sprint(r["monitorIds"]) != wantIDs || r["status"] != "paused" {
			return fmt.Errorf("remote %v", r)
		}
		return nil
	})
	announcement := &v1alpha1.PublicStatusPageAnnouncement{
		ObjectMeta: metav1.ObjectMeta{Name: "maintenance", Namespace: "default"},
		Spec: v1alpha1.PublicStatusPageAnnouncementSpec{
			PublicStatusPageRef: corev1.LocalObjectReference{Name: "status"},
			Title:               "Scheduled maintenance", Content: "A brief maintenance window.",
			Type: "Maintenance", Status: "Published", StartDate: "2026-01-01T03:00:00Z", Pinned: true,
		},
	}
	if err := c.Create(ctx, announcement); err != nil {
		t.Fatal(err)
	}
	eventually("announcement synced and pinned", func() error {
		_ = c.Get(ctx, client.ObjectKeyFromObject(announcement), announcement)
		r := api.get("psps/"+fmt.Sprint(psp.Status.ID)+"/announcements", announcement.Status.ID)
		page := api.get("psps", psp.Status.ID)
		if r["title"] != "Scheduled maintenance" || r["status"] != "Published" ||
			page["pinnedAnnouncementId"] != announcement.Status.ID || announcement.Status.PublicStatusPageID != psp.Status.ID {
			return fmt.Errorf("announcement %v, page %v, status %+v", r, page, announcement.Status)
		}
		return nil
	})
	announcement.Spec.Title = "Maintenance in progress"
	announcement.Spec.Pinned = false
	if err := c.Update(ctx, announcement); err != nil {
		t.Fatal(err)
	}
	eventually("announcement updated and unpinned", func() error {
		_ = c.Get(ctx, client.ObjectKeyFromObject(announcement), announcement)
		r := api.get("psps/"+fmt.Sprint(psp.Status.ID)+"/announcements", announcement.Status.ID)
		page := api.get("psps", psp.Status.ID)
		if r["title"] != "Maintenance in progress" || page["pinnedAnnouncementId"] != int64(0) {
			return fmt.Errorf("announcement %v, page %v", r, page)
		}
		return nil
	})
	if err := c.Delete(ctx, announcement); err != nil {
		t.Fatal(err)
	}
	eventually("announcement deleted", func() error {
		if r := api.get("psps/"+fmt.Sprint(psp.Status.ID)+"/announcements", announcement.Status.ID); r != nil {
			return fmt.Errorf("remote announcement still exists: %v", r)
		}
		return nil
	})

	ing.Annotations[AnnotationPrefix+"enabled"] = "false"
	if err := c.Update(ctx, ing); err != nil {
		t.Fatal(err)
	}
	eventually("monitor deleted", func() error {
		if r := api.get("monitors", m.Status.ID); r != nil {
			return fmt.Errorf("remote monitor still exists: %v", r)
		}
		return nil
	})
	t.Logf("API calls: %v", api.takeCalls())
}
