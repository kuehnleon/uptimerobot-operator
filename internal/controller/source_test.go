package controller

import (
	"context"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
	if u, err := ingressURL(context.Background(), nil, ing); err != nil || u != "https://a.example.com" {
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
		got, _ := ingressRouteURL(context.Background(), nil, u)
		if got != want {
			t.Errorf("%s: got %q, want %q", match, got, want)
		}
	}
}

func TestHTTPRouteURL(t *testing.T) {
	gatewayGVK := schema.GroupVersion{Group: gatewayAPIGroup, Version: "v1"}.WithKind("Gateway")
	gateway := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": gatewayAPIGroup + "/v1",
		"kind":       "Gateway",
		"metadata":   map[string]any{"name": "edge", "namespace": "default"},
		"spec": map[string]any{"listeners": []any{
			map[string]any{"name": "https", "hostname": "*.example.com", "port": int64(443), "protocol": "HTTPS"},
		}},
	}}
	gateway.SetGroupVersionKind(gatewayGVK)
	reader := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(gateway).Build()
	route := testHTTPRoute("api.example.com", "https")

	got, err := httpRouteURL(context.Background(), reader, route, gatewayGVK)
	if err != nil || got != "https://api.example.com" {
		t.Fatalf("got %q, %v", got, err)
	}

	listener := map[string]any{"name": "http", "hostname": "api.example.com", "port": int64(8080), "protocol": "HTTP"}
	if err := unstructured.SetNestedSlice(gateway.Object, []any{listener}, "spec", "listeners"); err != nil {
		t.Fatal(err)
	}
	if err := reader.Update(context.Background(), gateway); err != nil {
		t.Fatal(err)
	}
	route = testHTTPRoute("api.example.com", "http")
	got, err = httpRouteURL(context.Background(), reader, route, gatewayGVK)
	if err != nil || got != "http://api.example.com:8080" {
		t.Fatalf("got %q, %v", got, err)
	}

	if err := unstructured.SetNestedSlice(route.Object, []any{
		map[string]any{"parentRef": map[string]any{"group": gatewayAPIGroup, "kind": "Gateway", "namespace": "default", "name": "edge", "sectionName": "http"},
			"conditions": []any{map[string]any{"type": "Accepted", "status": "False"}}},
	}, "status", "parents"); err != nil {
		t.Fatal(err)
	}
	if _, err := httpRouteURL(context.Background(), reader, route, gatewayGVK); err == nil {
		t.Fatal("expected an error for an unaccepted HTTPRoute")
	}
}

func testHTTPRoute(hostname, sectionName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": gatewayAPIGroup + "/v1",
		"kind":       "HTTPRoute",
		"metadata":   map[string]any{"name": "api", "namespace": "default"},
		"spec": map[string]any{
			"hostnames":  []any{hostname},
			"parentRefs": []any{map[string]any{"name": "edge", "sectionName": sectionName}},
		},
		"status": map[string]any{"parents": []any{
			map[string]any{
				"parentRef": map[string]any{
					"group": gatewayAPIGroup, "kind": "Gateway", "namespace": "default", "name": "edge", "sectionName": sectionName,
				},
				"conditions": []any{map[string]any{"type": "Accepted", "status": "True"}},
			},
		}},
	}}
}
