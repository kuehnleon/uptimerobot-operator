package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// MaintenanceWindowSpec defines the desired state of a MaintenanceWindow.
type MaintenanceWindowSpec struct {
	// Defaults to the resource name.
	// +optional
	FriendlyName string `json:"friendlyName,omitempty"`
	// +kubebuilder:validation:Enum=once;daily;weekly;monthly
	Interval string `json:"interval"`
	// Start date (YYYY-MM-DD).
	// +kubebuilder:validation:Pattern=`^\d{4}-\d{2}-\d{2}$`
	Date string `json:"date"`
	// Start time (HH:mm:ss).
	// +kubebuilder:validation:Pattern=`^([01]\d|2[0-3]):[0-5]\d:[0-5]\d$`
	Time string `json:"time"`
	// Duration in minutes.
	// +kubebuilder:validation:Minimum=1
	Duration int32 `json:"duration"`
	// Weekdays (1-7) for weekly or days of month (-1 for last day) for monthly windows.
	// +optional
	Days []int32 `json:"days,omitempty"`
	// Selects Monitors in the same namespace. An empty selector selects all Monitors; if unset, assigned monitors are not managed.
	// +optional
	MonitorSelector *metav1.LabelSelector `json:"monitorSelector,omitempty"`
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// MaintenanceWindow is an UptimeRobot maintenance window.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=uptimerobot
// +kubebuilder:printcolumn:name="Interval",type=string,JSONPath=`.spec.interval`
// +kubebuilder:printcolumn:name="Time",type=string,JSONPath=`.spec.time`
// +kubebuilder:printcolumn:name="Duration",type=integer,JSONPath=`.spec.duration`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type MaintenanceWindow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MaintenanceWindowSpec `json:"spec"`
	Status ResourceStatus        `json:"status,omitempty"`
}

func (m *MaintenanceWindow) GetResourceStatus() *ResourceStatus { return &m.Status }

// +kubebuilder:object:root=true
type MaintenanceWindowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MaintenanceWindow `json:"items"`
}
