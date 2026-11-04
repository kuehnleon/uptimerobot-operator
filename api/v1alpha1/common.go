package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ResourcePolicySpec controls lifecycle behavior shared by all UptimeRobot resources.
type ResourcePolicySpec struct {
	// What to do with the UptimeRobot object when this resource is deleted. Defaults to Delete.
	// +kubebuilder:validation:Enum=Delete;Orphan
	// +kubebuilder:default=Delete
	// +optional
	DeletionPolicy string `json:"deletionPolicy,omitempty"`
}

// ResourceStatus is the status shared by all UptimeRobot resources.
type ResourceStatus struct {
	// ID of the resource in UptimeRobot.
	// +optional
	ID int64 `json:"id,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Hash of the last applied configuration.
	// +optional
	Hash string `json:"hash,omitempty"`
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
