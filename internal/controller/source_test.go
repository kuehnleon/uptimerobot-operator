package controller

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestMonitorSpec(t *testing.T) {
	spec, err := monitorSpec("https://example.com", map[string]string{
		AnnotationPrefix + "enabled":       "true",
		AnnotationPrefix + "interval":      "60",
		AnnotationPrefix + "type":          "KEYWORD",
		AnnotationPrefix + "keyword.type":  "ALERT_NOT_EXISTS",
		AnnotationPrefix + "keyword.value": "200",
		AnnotationPrefix + "alertContacts": `[{"id": 7}]`,
		"other/annotation":                 "ignored",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Type != "KEYWORD" || spec.Interval != 60 || spec.URL != "https://example.com" || spec.FriendlyName != "example.com" ||
		spec.Keyword.Value != "200" || spec.Keyword.Type != "ALERT_NOT_EXISTS" || spec.AlertContacts[0].ID != 7 {
		t.Fatalf("unexpected spec %+v", spec)
	}

	if _, err := monitorSpec("https://example.com", map[string]string{AnnotationPrefix + "intervall": "60"}); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestIngressURL(t *testing.T) {
	ing := &networkingv1.Ingress{Spec: networkingv1.IngressSpec{
		Rules: []networkingv1.IngressRule{{Host: "*.example.com"}, {Host: "a.example.com"}},
		TLS:   []networkingv1.IngressTLS{{Hosts: []string{"a.example.com"}}},
	}}
	if u, err := ingressURL(ing); err != nil || u != "https://a.example.com" {
		t.Fatalf("got %q, %v", u, err)
	}
}

func TestIngressRouteURL(t *testing.T) {
	for match, want := range map[string]string{
		"Host(`a.example.com`) && PathPrefix(`/api`)": "http://a.example.com",
		"PathPrefix(`/`) && Host(\"b.example.com\")":  "http://b.example.com",
		"Host(`c.example.com`, `d.example.com`)":      "http://c.example.com",
		"HostRegexp(`.*`)":                            "",
	} {
		u := &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{
			"routes": []any{map[string]any{"match": match}},
		}}}
		got, _ := ingressRouteURL(u)
		if got != want {
			t.Errorf("%s: got %q, want %q", match, got, want)
		}
	}
}
