package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// MonitorGroupSpec defines the desired state of a MonitorGroup.
type MonitorGroupSpec struct {
	ResourcePolicySpec `json:",inline"`
	// Defaults to the resource name.
	// +optional
	FriendlyName string `json:"friendlyName,omitempty"`
}

// MonitorGroup is an UptimeRobot monitor group. Monitors join it via spec.groupRef.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=uptimerobot
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type MonitorGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MonitorGroupSpec `json:"spec,omitempty"`
	Status ResourceStatus   `json:"status,omitempty"`
}

func (g *MonitorGroup) GetResourceStatus() *ResourceStatus { return &g.Status }
func (g *MonitorGroup) GetDeletionPolicy() string          { return g.Spec.DeletionPolicy }

// +kubebuilder:object:root=true
type MonitorGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MonitorGroup `json:"items"`
}
