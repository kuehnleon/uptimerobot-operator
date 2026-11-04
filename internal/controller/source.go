package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
)

// AnnotationPrefix is the prefix of annotations configuring monitors on supported route resources.
// "<prefix>enabled: true" enables monitoring, any other "<prefix><field>" sets the Monitor spec field,
// e.g. "<prefix>interval: 60" or "<prefix>keyword.value: OK". Values are parsed as JSON if possible.
const AnnotationPrefix = "uptimerobot.kuehnleon.github.io/"

// sourceReconciler creates a Monitor for each annotated object of a kind.
type sourceReconciler struct {
	client.Client
	events events.EventRecorder
	newObj func() client.Object
	prefix string
	url    func(context.Context, client.Reader, client.Object) (string, error)
}

func (r *sourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := r.newObj()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	m := &v1alpha1.Monitor{ObjectMeta: metav1.ObjectMeta{Name: r.prefix + obj.GetName(), Namespace: obj.GetNamespace()}}

	if obj.GetAnnotations()[AnnotationPrefix+"enabled"] != "true" {
		if err := r.Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		if metav1.IsControlledBy(m, obj) {
			return ctrl.Result{}, client.IgnoreNotFound(r.Delete(ctx, m))
		}
		return ctrl.Result{}, nil
	}

	var spec v1alpha1.MonitorSpec
	u, err := r.url(ctx, r.Client, obj)
	if v, ok := obj.GetAnnotations()[AnnotationPrefix+"url"]; ok {
		u, err = v, nil
	}
	if err == nil {
		spec, err = monitorSpec(u, obj.GetAnnotations())
	}
	if err != nil {
		r.events.Eventf(obj, nil, corev1.EventTypeWarning, "InvalidMonitor", "CreateMonitor", "%v", err)
		return ctrl.Result{}, reconcile.TerminalError(err)
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, m, func() error {
		if !m.CreationTimestamp.IsZero() && !metav1.IsControlledBy(m, obj) {
			return apierrors.NewAlreadyExists(v1alpha1.GroupVersion.WithResource("monitors").GroupResource(), m.Name)
		}
		m.Labels = obj.GetLabels()
		m.Spec = spec
		return controllerutil.SetControllerReference(obj, m, r.Scheme())
	})
	return ctrl.Result{}, err
}

// monitorSpec builds a Monitor spec for rawURL, overridden by annotations.
func monitorSpec(rawURL string, annotations map[string]string) (v1alpha1.MonitorSpec, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return v1alpha1.MonitorSpec{}, err
	}
	fields := map[string]any{"type": "HTTP", "url": rawURL, "friendlyName": u.Host, "interval": DefaultMonitorInterval}
	raw := map[string]string{}
	for k, v := range annotations {
		path, ok := strings.CutPrefix(k, AnnotationPrefix)
		if !ok || path == "enabled" {
			continue
		}
		var val any = v
		if json.Unmarshal([]byte(v), &val) != nil {
			val = v
		}
		raw[path] = v
		setPath(fields, strings.Split(path, "."), val)
	}

	// Retry with the raw string for string fields given values that parse as JSON numbers or booleans.
	for {
		var spec v1alpha1.MonitorSpec
		b, _ := json.Marshal(fields)
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		err := dec.Decode(&spec)
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Type.Kind() == reflect.String {
			if v, ok := raw[typeErr.Field]; ok {
				setPath(fields, strings.Split(typeErr.Field, "."), v)
				delete(raw, typeErr.Field)
				continue
			}
		}
		return spec, err
	}
}

func setPath(m map[string]any, path []string, v any) {
	for _, p := range path[:len(path)-1] {
		next, ok := m[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[path[len(path)-1]] = v
}

func ingressURL(_ context.Context, _ client.Reader, o client.Object) (string, error) {
	ing := o.(*networkingv1.Ingress)
	for _, rule := range ing.Spec.Rules {
		if rule.Host == "" || strings.Contains(rule.Host, "*") {
			continue
		}
		scheme := "http"
		if slices.ContainsFunc(ing.Spec.TLS, func(t networkingv1.IngressTLS) bool { return slices.Contains(t.Hosts, rule.Host) }) {
			scheme = "https"
		}
		return scheme + "://" + rule.Host, nil
	}
	return "", errors.New("no host found in ingress rules, set the url annotation")
}

var (
	hostRule = regexp.MustCompile(`\bHost\(([^)]*)\)`)
	quoted   = regexp.MustCompile("[`\"]([^`\"]+)[`\"]")
)

func ingressRouteURL(_ context.Context, _ client.Reader, o client.Object) (string, error) {
	u := o.(*unstructured.Unstructured)
	routes, _, _ := unstructured.NestedSlice(u.Object, "spec", "routes")
	scheme := "http"
	if _, ok, _ := unstructured.NestedFieldNoCopy(u.Object, "spec", "tls"); ok {
		scheme = "https"
	}
	for _, route := range routes {
		match, _, _ := unstructured.NestedString(route.(map[string]any), "match")
		if m := hostRule.FindStringSubmatch(match); m != nil {
			if h := quoted.FindStringSubmatch(m[1]); h != nil {
				return scheme + "://" + h[1], nil
			}
		}
	}
	return "", errors.New("no Host() rule found in routes, set the url annotation")
}

type sourceURL func(context.Context, client.Reader, client.Object) (string, error)

func setupSource(mgr ctrl.Manager, name, prefix string, newObj func() client.Object, url sourceURL) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(newObj()).
		Owns(&v1alpha1.Monitor{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(&sourceReconciler{
			Client: mgr.GetClient(), events: mgr.GetEventRecorder("uptimerobot-operator"), newObj: newObj, prefix: prefix, url: url,
		})
}

func setupIngressRoute(mgr ctrl.Manager, gvk schema.GroupVersionKind) error {
	if _, err := mgr.GetRESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version); err != nil {
		ctrl.Log.Info("Traefik IngressRoute CRD not found, skipping", "group", gvk.Group)
		return nil
	}
	newObj := func() client.Object {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		return u
	}
	return setupSource(mgr, "ingressroute."+gvk.Group, "ingressroute-", newObj, ingressRouteURL)
}

const gatewayAPIGroup = "gateway.networking.k8s.io"

func setupHTTPRouteSource(mgr ctrl.Manager) error {
	mapper := mgr.GetRESTMapper()
	routeGVK, ok := mappedGVK(mapper, "HTTPRoute", []string{"v1", "v1beta1"})
	if !ok {
		ctrl.Log.Info("Gateway API HTTPRoute CRD not found, skipping")
		return nil
	}
	gatewayGVK, ok := mappedGVK(mapper, "Gateway", []string{"v1", "v1beta1"})
	if !ok {
		ctrl.Log.Info("Gateway API Gateway CRD not found, skipping HTTPRoute source")
		return nil
	}
	newRoute := func() client.Object {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(routeGVK)
		return u
	}
	newGateway := func() client.Object {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gatewayGVK)
		return u
	}
	r := &sourceReconciler{
		Client: mgr.GetClient(), events: mgr.GetEventRecorder("uptimerobot-operator"),
		newObj: newRoute, prefix: "httproute-",
		url: func(ctx context.Context, reader client.Reader, route client.Object) (string, error) {
			return httpRouteURL(ctx, reader, route.(*unstructured.Unstructured), gatewayGVK)
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("httproute").
		For(newRoute()).
		Owns(&v1alpha1.Monitor{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(newGateway(), enqueueGatewayRoutes(mgr.GetClient(), routeGVK)).
		Complete(r)
}

func mappedGVK(mapper meta.RESTMapper, kind string, versions []string) (schema.GroupVersionKind, bool) {
	gk := schema.GroupKind{Group: gatewayAPIGroup, Kind: kind}
	for _, version := range versions {
		if _, err := mapper.RESTMapping(gk, version); err == nil {
			return schema.GroupVersion{Group: gatewayAPIGroup, Version: version}.WithKind(kind), true
		}
	}
	return schema.GroupVersionKind{}, false
}

func enqueueGatewayRoutes(c client.Client, routeGVK schema.GroupVersionKind) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, gateway client.Object) []reconcile.Request {
		routes := &unstructured.UnstructuredList{}
		routes.SetGroupVersionKind(routeGVK.GroupVersion().WithKind("HTTPRouteList"))
		if err := c.List(ctx, routes); err != nil {
			return nil
		}
		var requests []reconcile.Request
		for i := range routes.Items {
			route := &routes.Items[i]
			if referencesGateway(route, gateway) {
				requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(route)})
			}
		}
		return requests
	})
}

func referencesGateway(route *unstructured.Unstructured, gateway client.Object) bool {
	refs, _, _ := unstructured.NestedSlice(route.Object, "spec", "parentRefs")
	for _, value := range refs {
		ref, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if parentRefValue(ref, "group", gatewayAPIGroup) == gatewayAPIGroup &&
			parentRefValue(ref, "kind", "Gateway") == "Gateway" &&
			parentRefValue(ref, "namespace", route.GetNamespace()) == gateway.GetNamespace() &&
			parentRefValue(ref, "name", "") == gateway.GetName() {
			return true
		}
	}
	return false
}

func httpRouteURL(ctx context.Context, reader client.Reader, route *unstructured.Unstructured, gatewayGVK schema.GroupVersionKind) (string, error) {
	refs, _, _ := unstructured.NestedSlice(route.Object, "spec", "parentRefs")
	parents, _, _ := unstructured.NestedSlice(route.Object, "status", "parents")
	hostnames, _, _ := unstructured.NestedStringSlice(route.Object, "spec", "hostnames")
	for _, value := range refs {
		ref, ok := value.(map[string]any)
		if !ok || parentRefValue(ref, "group", gatewayAPIGroup) != gatewayAPIGroup || parentRefValue(ref, "kind", "Gateway") != "Gateway" {
			continue
		}
		if !httpRouteParentAccepted(ref, parents, route.GetNamespace()) {
			continue
		}
		name := parentRefValue(ref, "name", "")
		if name == "" {
			continue
		}
		namespace := parentRefValue(ref, "namespace", route.GetNamespace())
		gateway := &unstructured.Unstructured{}
		gateway.SetGroupVersionKind(gatewayGVK)
		if err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, gateway); err != nil {
			return "", fmt.Errorf("get parent Gateway %s/%s: %w", namespace, name, err)
		}
		listeners, _, _ := unstructured.NestedSlice(gateway.Object, "spec", "listeners")
		sectionName := parentRefValue(ref, "sectionName", "")
		for _, listenerValue := range listeners {
			listener, ok := listenerValue.(map[string]any)
			if !ok || (sectionName != "" && nestedString(listener, "name") != sectionName) {
				continue
			}
			protocol := nestedString(listener, "protocol")
			scheme := map[string]string{"HTTP": "http", "HTTPS": "https"}[protocol]
			if scheme == "" {
				continue
			}
			listenerHostname := nestedString(listener, "hostname")
			host := routeHostname(hostnames, listenerHostname)
			if host == "" && len(hostnames) == 0 && listenerHostname == "" {
				host = gatewayAddress(gateway)
			}
			if host == "" {
				continue
			}
			port, _, _ := unstructured.NestedInt64(listener, "port")
			return gatewayURL(scheme, host, port), nil
		}
	}
	return "", errors.New("no accepted HTTPRoute Gateway listener with a concrete hostname found, set the url annotation")
}

func httpRouteParentAccepted(ref map[string]any, parents []any, routeNamespace string) bool {
	for _, value := range parents {
		parent, ok := value.(map[string]any)
		if !ok {
			continue
		}
		statusRef, ok := parent["parentRef"].(map[string]any)
		if !ok || !sameParentRef(ref, statusRef, routeNamespace) {
			continue
		}
		conditions, _, _ := unstructured.NestedSlice(parent, "conditions")
		for _, conditionValue := range conditions {
			condition, ok := conditionValue.(map[string]any)
			if ok && nestedString(condition, "type") == "Accepted" && nestedString(condition, "status") == "True" {
				return true
			}
		}
	}
	return false
}

func sameParentRef(a, b map[string]any, routeNamespace string) bool {
	for key, def := range map[string]string{
		"group": gatewayAPIGroup, "kind": "Gateway", "namespace": routeNamespace, "name": "",
	} {
		if parentRefValue(a, key, def) != parentRefValue(b, key, def) {
			return false
		}
	}
	left, right := parentRefValue(a, "sectionName", ""), parentRefValue(b, "sectionName", "")
	if left != "" && right != "" && left != right {
		return false
	}
	left, right = numericString(a["port"]), numericString(b["port"])
	if left != "" && right != "" && left != right {
		return false
	}
	return true
}

func parentRefValue(ref map[string]any, key, fallback string) string {
	value, ok := ref[key].(string)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func nestedString(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return value
}

func numericString(v any) string {
	switch value := v.(type) {
	case int64:
		return strconv.FormatInt(value, 10)
	case int32:
		return strconv.FormatInt(int64(value), 10)
	case int:
		return strconv.Itoa(value)
	case float64:
		return strconv.FormatInt(int64(value), 10)
	default:
		return ""
	}
}

func routeHostname(routeHostnames []string, listenerHostname string) string {
	if len(routeHostnames) == 0 {
		if concreteHostname(listenerHostname) {
			return listenerHostname
		}
		return ""
	}
	for _, routeHost := range routeHostnames {
		if listenerHostname == "" && concreteHostname(routeHost) {
			return routeHost
		}
		if concreteHostname(routeHost) && hostnameMatches(listenerHostname, routeHost) {
			return routeHost
		}
		if concreteHostname(listenerHostname) && hostnameMatches(routeHost, listenerHostname) {
			return listenerHostname
		}
	}
	return ""
}

func concreteHostname(host string) bool {
	return host != "" && !strings.Contains(host, "*")
}

func hostnameMatches(pattern, host string) bool {
	pattern, host = strings.ToLower(pattern), strings.ToLower(host)
	if pattern == host {
		return true
	}
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}
	suffix := pattern[1:]
	if !strings.HasSuffix(host, suffix) {
		return false
	}
	label := strings.TrimSuffix(host, suffix)
	return label != "" && !strings.Contains(label, ".")
}

func gatewayAddress(gateway *unstructured.Unstructured) string {
	addresses, _, _ := unstructured.NestedSlice(gateway.Object, "status", "addresses")
	for _, value := range addresses {
		address, ok := value.(map[string]any)
		if !ok {
			continue
		}
		kind := nestedString(address, "type")
		if kind == "" || kind == "Hostname" || kind == "IPAddress" {
			if host := nestedString(address, "value"); host != "" {
				return host
			}
		}
	}
	return ""
}

func gatewayURL(scheme, host string, port int64) string {
	defaultPort := int64(80)
	if scheme == "https" {
		defaultPort = 443
	}
	if port != 0 && port != defaultPort {
		host = net.JoinHostPort(host, strconv.FormatInt(port, 10))
	} else if net.ParseIP(host) != nil && strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return (&url.URL{Scheme: scheme, Host: host}).String()
}
