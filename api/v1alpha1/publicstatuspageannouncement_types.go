package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PublicStatusPageAnnouncementSpec defines the desired state of a status-page announcement.
// +kubebuilder:validation:XValidation:rule="self.publicStatusPageRef == oldSelf.publicStatusPageRef",message="publicStatusPageRef is immutable"
type PublicStatusPageAnnouncementSpec struct {
	ResourcePolicySpec `json:",inline"`
	// PublicStatusPage in the same namespace that owns this announcement.
	PublicStatusPageRef corev1.LocalObjectReference `json:"publicStatusPageRef"`
	// +kubebuilder:validation:MaxLength=255
	Title string `json:"title"`
	// +kubebuilder:validation:MaxLength=2000
	Content string `json:"content"`
	// +kubebuilder:validation:Enum=Info;Maintenance;Issue
	// +kubebuilder:default=Info
	Type string `json:"type,omitempty"`
	// +kubebuilder:validation:Enum=Offline;Pending;Published;Archived
	// +kubebuilder:default=Offline
	Status string `json:"status,omitempty"`
	// +kubebuilder:validation:Format=date-time
	StartDate string `json:"startDate"`
	// +kubebuilder:validation:Format=date-time
	// +optional
	EndDate string `json:"endDate,omitempty"`
	// Pin this announcement to the public status page.
	// +optional
	Pinned bool `json:"pinned,omitempty"`
}

// PublicStatusPageAnnouncementStatus defines the observed state of an announcement.
type PublicStatusPageAnnouncementStatus struct {
	ResourceStatus `json:",inline"`
	// ID of the owning PublicStatusPage in UptimeRobot, retained for cleanup if the page is removed.
	// +optional
	PublicStatusPageID int64 `json:"publicStatusPageID,omitempty"`
	// Current announcement status reported by UptimeRobot.
	// +optional
	RemoteStatus string `json:"remoteStatus,omitempty"`
}

// PublicStatusPageAnnouncement is an UptimeRobot public status page announcement.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=uptimerobot,shortName=pspa
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.remoteStatus`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type PublicStatusPageAnnouncement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PublicStatusPageAnnouncementSpec   `json:"spec"`
	Status PublicStatusPageAnnouncementStatus `json:"status,omitempty"`
}

func (a *PublicStatusPageAnnouncement) GetResourceStatus() *ResourceStatus {
	return &a.Status.ResourceStatus
}

func (a *PublicStatusPageAnnouncement) GetDeletionPolicy() string { return a.Spec.DeletionPolicy }

// +kubebuilder:object:root=true
type PublicStatusPageAnnouncementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PublicStatusPageAnnouncement `json:"items"`
}
