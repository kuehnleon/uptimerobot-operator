package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// IntegrationSpec defines the desired state of an Integration.
type IntegrationSpec struct {
	// +kubebuilder:validation:Enum=Slack;MSTeams;Webhook;Discord;Mattermost;GoogleChat;Pagerduty;Pushover;PushBullet;Splunk;Zapier
	Type string `json:"type"`
	// Defaults to the resource name.
	// +optional
	FriendlyName string `json:"friendlyName,omitempty"`
	// +kubebuilder:validation:Enum=UpAndDown;Down;Up;None
	// +kubebuilder:default=UpAndDown
	// +optional
	EnableNotificationsFor string `json:"enableNotificationsFor,omitempty"`
	// Send notifications about SSL and domain expiry.
	// +optional
	SSLExpirationReminder *bool `json:"sslExpirationReminder,omitempty"`
	// Type specific settings passed as-is to the API, e.g. webhookURL, customValue, urlToNotify or postValue.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	// +optional
	Config *runtime.RawExtension `json:"config,omitempty"`
	// Type specific settings read from secrets, e.g. webhookURL. Merged into config.
	// +optional
	SecretConfig map[string]corev1.SecretKeySelector `json:"secretConfig,omitempty"`
}

// Integration is an UptimeRobot integration (notification channel), assignable to Monitors.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=uptimerobot
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Integration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IntegrationSpec `json:"spec"`
	Status ResourceStatus  `json:"status,omitempty"`
}

func (i *Integration) GetResourceStatus() *ResourceStatus { return &i.Status }

// +kubebuilder:object:root=true
type IntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Integration `json:"items"`
}
