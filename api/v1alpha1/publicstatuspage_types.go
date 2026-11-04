package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PublicStatusPageSpec defines the desired state of a PublicStatusPage.
type PublicStatusPageSpec struct {
	ResourcePolicySpec `json:",inline"`
	// Defaults to the resource name.
	// +optional
	FriendlyName string `json:"friendlyName,omitempty"`
	// Selects Monitors in the same namespace. An empty selector selects all Monitors; if unset, assigned monitors are not managed.
	// +optional
	MonitorSelector *metav1.LabelSelector `json:"monitorSelector,omitempty"`
	// +optional
	CustomDomain string `json:"customDomain,omitempty"`
	// +optional
	PasswordSecretRef *corev1.SecretKeySelector `json:"passwordSecretRef,omitempty"`
	// +kubebuilder:validation:Enum=FriendlyNameAsc;FriendlyNameDesc;StatusUpDownPaused;StatusDownUpPaused
	// +optional
	Sort string `json:"sort,omitempty"`
	// +optional
	HideURLLinks *bool `json:"hideUrlLinks,omitempty"`
	// +optional
	NoIndex *bool `json:"noIndex,omitempty"`
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// PublicStatusPageStatus defines the observed state of a PublicStatusPage.
type PublicStatusPageStatus struct {
	ResourceStatus `json:",inline"`
	// +optional
	URL string `json:"url,omitempty"`
}

// PublicStatusPage is an UptimeRobot public status page.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=uptimerobot,shortName=psp
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type PublicStatusPage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PublicStatusPageSpec   `json:"spec,omitempty"`
	Status PublicStatusPageStatus `json:"status,omitempty"`
}

func (p *PublicStatusPage) GetResourceStatus() *ResourceStatus { return &p.Status.ResourceStatus }
func (p *PublicStatusPage) GetDeletionPolicy() string          { return p.Spec.DeletionPolicy }

// +kubebuilder:object:root=true
type PublicStatusPageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PublicStatusPage `json:"items"`
}
