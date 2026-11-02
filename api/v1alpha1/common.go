package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
