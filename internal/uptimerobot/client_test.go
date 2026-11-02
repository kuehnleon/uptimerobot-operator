package uptimerobot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient(t *testing.T) {
	var lists int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("missing auth header")
		}
		switch r.Method + " " + r.URL.RequestURI() {
		case "GET /monitors":
			lists++
			_, _ = w.Write([]byte(`{"data":[{"id":1}],"nextLink":"http://` + r.Host + `/monitors?cursor=1"}`))
		case "GET /monitors?cursor=1":
			_, _ = w.Write([]byte(`{"data":[{"id":2,"status":"UP"}],"nextLink":null}`))
		case "PATCH /monitors/2":
			_, _ = w.Write([]byte(`{"id":2,"status":"DOWN"}`))
		case "DELETE /monitors/3":
			w.WriteHeader(http.StatusNotFound)
		case "POST /monitors":
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"code":"000-001","message":"slow down"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	c := New(srv.URL, "key", 6000, time.Hour)

	if o, err := c.Get(ctx, "/monitors", 2); err != nil || o.String("status") != "UP" {
		t.Fatalf("Get across pages: %v %v", o, err)
	}
	if o, _ := c.Get(ctx, "/monitors", 42); o != nil {
		t.Fatalf("expected nil for unknown id, got %v", o)
	}
	if _, err := c.Update(ctx, "/monitors", 2, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if o, _ := c.Get(ctx, "/monitors", 2); o.String("status") != "DOWN" || lists != 1 {
		t.Fatalf("cache not updated or refetched: %v, lists=%d", o, lists)
	}
	if err := c.Delete(ctx, "/monitors", 3); err != nil {
		t.Fatalf("404 on delete should be ignored: %v", err)
	}
	_, err := c.Create(ctx, "/monitors", map[string]any{})
	if e, ok := err.(*APIError); !ok || e.StatusCode != 429 || e.Message != "slow down" {
		t.Fatalf("unexpected error %#v", err)
	}
	if d := time.Until(time.Unix(0, c.pausedUntil.Load())); d < 6*time.Second || d > 7*time.Second {
		t.Fatalf("expected requests to be paused for Retry-After, got %v", d)
	}
}

func TestHashIsKeyed(t *testing.T) {
	v := map[string]any{"a": 1}
	if New("", "a", 1, 0).Hash(v) == New("", "b", 1, 0).Hash(v) {
		t.Fatal("hash must depend on the API key")
	}
}
