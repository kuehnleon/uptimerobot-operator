package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// MonitorSpec defines the desired state of a Monitor.
// +kubebuilder:validation:XValidation:rule="self.type == 'HEARTBEAT' || has(self.url)",message="url is required"
// +kubebuilder:validation:XValidation:rule="self.type != 'KEYWORD' || has(self.keyword)",message="keyword is required for KEYWORD monitors"
// +kubebuilder:validation:XValidation:rule="!(self.type in ['PORT', 'UDP']) || has(self.port)",message="port is required for PORT and UDP monitors"
// +kubebuilder:validation:XValidation:rule="!(has(self.groupId) && has(self.groupRef))",message="groupId and groupRef are mutually exclusive"
type MonitorSpec struct {
	// +kubebuilder:validation:Enum=HTTP;KEYWORD;PING;PORT;HEARTBEAT;DNS;API;UDP
	Type string `json:"type"`
	// Defaults to the resource name.
	// +optional
	FriendlyName string `json:"friendlyName,omitempty"`
	// URL, hostname or IP to monitor. Not used by HEARTBEAT monitors.
	// +optional
	URL string `json:"url,omitempty"`
	// Check interval in seconds.
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=30
	// +optional
	Interval int32 `json:"interval,omitempty"`
	// Timeout in seconds. Defaults to 30.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=60
	// +optional
	Timeout *int32 `json:"timeout,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port *int32 `json:"port,omitempty"`
	// Seconds to wait past the expected heartbeat before alerting (HEARTBEAT only).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=86400
	// +optional
	GracePeriod *int32 `json:"gracePeriod,omitempty"`
	// +optional
	Keyword *Keyword `json:"keyword,omitempty"`
	// +kubebuilder:validation:Enum=HEAD;GET;POST;PUT;PATCH;DELETE;OPTIONS
	// +optional
	HTTPMethod string `json:"httpMethod,omitempty"`
	// +optional
	Auth *HTTPAuth `json:"auth,omitempty"`
	// +optional
	CustomHTTPHeaders map[string]string `json:"customHttpHeaders,omitempty"`
	// e.g. ["2xx", "3xx"]
	// +optional
	SuccessHTTPResponseCodes []string `json:"successHttpResponseCodes,omitempty"`
	// +optional
	CheckSSLErrors *bool `json:"checkSSLErrors,omitempty"`
	// +optional
	FollowRedirections *bool `json:"followRedirections,omitempty"`
	// +optional
	SSLExpirationReminder *bool `json:"sslExpirationReminder,omitempty"`
	// +optional
	DomainExpirationReminder *bool `json:"domainExpirationReminder,omitempty"`
	// +optional
	AlertContacts []MonitorAlertContact `json:"alertContacts,omitempty"`
	// +optional
	Tags []string `json:"tags,omitempty"`
	// ID of a monitor group in UptimeRobot.
	// +optional
	GroupID *int64 `json:"groupId,omitempty"`
	// MonitorGroup in the same namespace.
	// +optional
	GroupRef *corev1.LocalObjectReference `json:"groupRef,omitempty"`
	// Type specific configuration passed as-is to the API, e.g. dnsRecords or apiAssertions.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	// +optional
	Config *runtime.RawExtension `json:"config,omitempty"`
	// +optional
	Paused bool `json:"paused,omitempty"`
}

type Keyword struct {
	// +kubebuilder:validation:Enum=ALERT_EXISTS;ALERT_NOT_EXISTS
	Type  string `json:"type"`
	Value string `json:"value"`
	// +optional
	CaseSensitive bool `json:"caseSensitive,omitempty"`
}

type HTTPAuth struct {
	// +kubebuilder:validation:Enum=HTTP_BASIC;DIGEST;BEARER
	Type string `json:"type"`
	// +optional
	Username string `json:"username,omitempty"`
	// Password or bearer token.
	PasswordSecretRef corev1.SecretKeySelector `json:"passwordSecretRef"`
}

// +kubebuilder:validation:XValidation:rule="has(self.id) ? (!has(self.integrationRef) && !has(self.alertContactRef)) : (has(self.integrationRef) != has(self.alertContactRef))",message="exactly one of id, integrationRef or alertContactRef is required"
type MonitorAlertContact struct {
	// ID of an alert contact or integration in UptimeRobot.
	// +optional
	ID int64 `json:"id,omitempty"`
	// Integration in the same namespace.
	// +optional
	IntegrationRef *corev1.LocalObjectReference `json:"integrationRef,omitempty"`
	// AlertContact in the same namespace.
	// +optional
	AlertContactRef *corev1.LocalObjectReference `json:"alertContactRef,omitempty"`
	// Minutes to wait before notifying (paid plans only).
	// +optional
	Threshold int32 `json:"threshold,omitempty"`
	// Repeat notifications every X minutes (paid plans only).
	// +optional
	Recurrence int32 `json:"recurrence,omitempty"`
}

// MonitorStatus defines the observed state of a Monitor.
type MonitorStatus struct {
	ResourceStatus `json:",inline"`
	// Current state as reported by UptimeRobot, e.g. UP, DOWN or PAUSED.
	// +optional
	State string `json:"state,omitempty"`
	// +optional
	HeartbeatURL string `json:"heartbeatURL,omitempty"`
}

// Monitor is an UptimeRobot monitor.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=uptimerobot
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Monitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MonitorSpec   `json:"spec"`
	Status MonitorStatus `json:"status,omitempty"`
}

func (m *Monitor) GetResourceStatus() *ResourceStatus { return &m.Status.ResourceStatus }

// +kubebuilder:object:root=true
type MonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Monitor `json:"items"`
}
