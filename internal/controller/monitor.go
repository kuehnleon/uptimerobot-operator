package controller

import (
	"context"
	"encoding/json"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
	"github.com/kuehnleon/uptimerobot-operator/internal/uptimerobot"
)

const DefaultMonitorInterval = 300

func setupMonitor(mgr ctrl.Manager, o Options) error {
	r := &syncer[*v1alpha1.Monitor]{
		Client:   mgr.GetClient(),
		reader:   mgr.GetAPIReader(),
		ur:       o.Client,
		events:   mgr.GetEventRecorder("uptimerobot-operator"),
		interval: o.SyncInterval,
		path:     "/monitors",
		newObj:   func() *v1alpha1.Monitor { return &v1alpha1.Monitor{} },
		drift: []string{"friendlyName", "url", "interval", "timeout", "port", "gracePeriod",
			"keywordType", "keywordValue", "httpMethodType", "checkSSLErrors", "followRedirections"},
		desired: func(ctx context.Context, m *v1alpha1.Monitor) (map[string]any, error) {
			return monitorBody(ctx, secretGetter(mgr.GetAPIReader(), m.Namespace), m)
		},
		after: func(ctx context.Context, m *v1alpha1.Monitor, remote uptimerobot.Object) (uptimerobot.Object, error) {
			action, state := "", remote.String("status")
			switch {
			case m.Spec.Paused && state != "PAUSED":
				action = "pause"
			case !m.Spec.Paused && state == "PAUSED":
				action = "start"
			default:
				return remote, nil
			}
			o, err := o.Client.Action(ctx, "/monitors", remote.ID(), action)
			if err != nil || o.ID() != 0 {
				return o, err
			}
			remote["status"] = map[string]string{"pause": "PAUSED", "start": "STARTED"}[action]
			return remote, nil
		},
		observe: func(m *v1alpha1.Monitor, remote uptimerobot.Object) {
			m.Status.State = remote.String("status")
			m.Status.HeartbeatURL = remote.String("heartbeatUrl")
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Monitor{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

func monitorBody(ctx context.Context, secret secretFunc, m *v1alpha1.Monitor) (map[string]any, error) {
	s := m.Spec
	b := map[string]any{
		"type":         s.Type,
		"friendlyName": or(s.FriendlyName, m.Name),
		"interval":     or(s.Interval, DefaultMonitorInterval),
	}
	if s.Type != "HEARTBEAT" {
		b["url"] = s.URL
	}
	if s.Type != "HEARTBEAT" && s.Type != "DNS" {
		b["timeout"] = or(deref(s.Timeout), 30)
	}
	opt(b, "port", s.Port)
	opt(b, "gracePeriod", s.GracePeriod)
	opt(b, "checkSSLErrors", s.CheckSSLErrors)
	opt(b, "followRedirections", s.FollowRedirections)
	opt(b, "sslExpirationReminder", s.SSLExpirationReminder)
	opt(b, "domainExpirationReminder", s.DomainExpirationReminder)
	opt(b, "groupId", s.GroupID)
	if s.Keyword != nil {
		b["keywordType"] = s.Keyword.Type
		b["keywordValue"] = s.Keyword.Value
		b["keywordCaseType"] = map[bool]int{true: 0, false: 1}[s.Keyword.CaseSensitive]
	}
	if s.HTTPMethod != "" {
		b["httpMethodType"] = s.HTTPMethod
	}
	if s.Auth != nil {
		pw, err := secret(ctx, &s.Auth.PasswordSecretRef)
		if err != nil {
			return nil, err
		}
		b["authType"] = s.Auth.Type
		b["httpUsername"] = s.Auth.Username
		b["httpPassword"] = pw
	}
	if len(s.CustomHTTPHeaders) > 0 {
		b["customHttpHeaders"] = s.CustomHTTPHeaders
	}
	if len(s.SuccessHTTPResponseCodes) > 0 {
		b["successHttpResponseCodes"] = s.SuccessHTTPResponseCodes
	}
	if len(s.Tags) > 0 {
		b["tagNames"] = s.Tags
	}
	if len(s.AlertContacts) > 0 {
		contacts := make([]map[string]any, len(s.AlertContacts))
		for i, c := range s.AlertContacts {
			contacts[i] = map[string]any{"alertContactId": c.ID, "threshold": c.Threshold, "recurrence": c.Recurrence}
		}
		b["assignedAlertContacts"] = contacts
	}
	if s.Config != nil && len(s.Config.Raw) > 0 {
		if !json.Valid(s.Config.Raw) {
			return nil, fmt.Errorf("spec.config is not valid JSON")
		}
		b["config"] = json.RawMessage(s.Config.Raw)
	}
	return b, nil
}
