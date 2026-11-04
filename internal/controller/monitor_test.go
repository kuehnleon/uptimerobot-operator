package controller

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
	"github.com/kuehnleon/uptimerobot-operator/internal/uptimerobot"
)

// fakeAPI is an in-memory UptimeRobot API recording all mutating calls.
type fakeAPI struct {
	mu    sync.Mutex
	data  map[string]map[int64]map[string]any // collection -> id -> object
	calls []string
	ids   int64
}

func newFakeAPI() *fakeAPI { return &fakeAPI{data: map[string]map[int64]map[string]any{}} }

func (f *fakeAPI) with(fn func(data map[string]map[int64]map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.data)
}

func (f *fakeAPI) get(collection string, id int64) map[string]any {
	var o map[string]any
	f.with(func(d map[string]map[int64]map[string]any) { o = maps.Clone(d[collection][id]) })
	return o
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	collection, idIndex, actionIndex := parts[0], 1, 2
	if len(parts) >= 3 && parts[0] == "psps" && parts[2] == "announcements" {
		collection = strings.Join(parts[:3], "/")
		idIndex, actionIndex = 3, 4
	}
	items := f.data[collection]
	if items == nil {
		items = map[int64]map[string]any{}
		f.data[collection] = items
	}
	var id int64
	if len(parts) > idIndex {
		id, _ = strconv.ParseInt(parts[idIndex], 10, 64)
	}
	if r.Method != http.MethodGet {
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	switch {
	case r.Method == http.MethodGet:
		data := []any{}
		for _, o := range items {
			data = append(data, o)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	case r.Method == http.MethodPost && id == 0:
		f.ids++
		body["id"] = f.ids
		if body["status"] == nil {
			body["status"] = "UP"
		}
		items[f.ids] = body
		_ = json.NewEncoder(w).Encode(body)
	case items[id] == nil:
		w.WriteHeader(http.StatusNotFound)
	case r.Method == http.MethodPost && len(parts) > actionIndex && (parts[actionIndex] == "pin" || parts[actionIndex] == "unpin"):
		pageID, _ := strconv.ParseInt(parts[1], 10, 64)
		if page := f.data["psps"][pageID]; page != nil {
			if parts[actionIndex] == "pin" {
				page["pinnedAnnouncementId"] = id
			} else {
				page["pinnedAnnouncementId"] = int64(0)
			}
		}
	case r.Method == http.MethodPost && len(parts) > actionIndex && parts[actionIndex] == "pause":
		items[id]["status"] = "PAUSED"
		_ = json.NewEncoder(w).Encode(items[id])
	case r.Method == http.MethodPatch:
		for k, v := range body {
			items[id][k] = v
		}
		_ = json.NewEncoder(w).Encode(items[id])
	case r.Method == http.MethodDelete:
		delete(items, id)
	}
}

func (f *fakeAPI) takeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls
	f.calls = nil
	return c
}

func TestMonitorSync(t *testing.T) {
	api := newFakeAPI()
	srv := httptest.NewServer(api)
	defer srv.Close()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	m := &v1alpha1.Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: v1alpha1.MonitorSpec{
			Type: "HTTP", URL: "https://example.com", Interval: 60,
			Auth: &v1alpha1.HTTPAuth{Type: "HTTP_BASIC", Username: "u", PasswordSecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "auth"}, Key: "password"}},
		},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"}, Data: map[string][]byte{"password": []byte("s3cret")}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(m, secret).WithStatusSubresource(m).Build()

	ur := uptimerobot.New(srv.URL, "key", 6000, time.Hour)
	r := &syncer[*v1alpha1.Monitor]{
		Client: c, reader: c, ur: ur, events: &events.FakeRecorder{}, interval: time.Minute, path: "/monitors",
		newObj: func() *v1alpha1.Monitor { return &v1alpha1.Monitor{} },
		drift:  []string{"url", "interval"},
		desired: func(ctx context.Context, m *v1alpha1.Monitor) (map[string]any, error) {
			return monitorBody(ctx, c, secretGetter(c, m.Namespace), m)
		},
	}
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(m)}
	reconcile := func() {
		t.Helper()
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatal(err)
		}
		_ = c.Get(ctx, req.NamespacedName, m)
	}
	expectCalls := func(want ...string) {
		t.Helper()
		if got := api.takeCalls(); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("calls: got %v, want %v", got, want)
		}
	}

	reconcile()
	expectCalls("POST /monitors")
	if m.Status.ID != 1 || api.get("monitors", 1)["httpPassword"] != "s3cret" || m.Status.Conditions[0].Reason != "Synced" {
		t.Fatalf("unexpected status %+v / remote %v", m.Status, api.get("monitors", 1))
	}

	reconcile()
	expectCalls()

	// A stale cache without the ID must not lead to a second create.
	stale := m.DeepCopy()
	stale.Status = v1alpha1.MonitorStatus{}
	r.Client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(stale, secret).WithStatusSubresource(stale).Build()
	r.ur = uptimerobot.New(srv.URL, "key", 6000, time.Hour)
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	expectCalls()
	r.Client = c

	m.Spec.Interval = 120
	_ = c.Update(ctx, m)
	reconcile()
	expectCalls("PATCH /monitors/1")

	api.with(func(d map[string]map[int64]map[string]any) { d["monitors"][1]["url"] = "https://changed.example.com" })
	ur = uptimerobot.New(srv.URL, "key", 6000, time.Hour) // drop cache
	r.ur = ur
	reconcile()
	expectCalls("PATCH /monitors/1")

	api.with(func(d map[string]map[int64]map[string]any) { delete(d["monitors"], 1) })
	r.ur = uptimerobot.New(srv.URL, "key", 6000, time.Hour)
	reconcile()
	expectCalls("POST /monitors")

	_ = c.Delete(ctx, m)
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	expectCalls("DELETE /monitors/2")
	if err := c.Get(ctx, req.NamespacedName, m); err == nil {
		t.Fatal("finalizer not removed")
	}
}
