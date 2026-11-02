// Package v1alpha1 contains the API types of the uptimerobot.kuehnleon.github.io group.
// +kubebuilder:object:generate=true
// +groupName=uptimerobot.kuehnleon.github.io
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "uptimerobot.kuehnleon.github.io", Version: "v1alpha1"}
	SchemeBuilder = runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion,
			&Monitor{}, &MonitorList{},
			&MaintenanceWindow{}, &MaintenanceWindowList{},
			&PublicStatusPage{}, &PublicStatusPageList{},
			&Integration{}, &IntegrationList{},
			&MonitorGroup{}, &MonitorGroupList{},
		)
		metav1.AddToGroupVersion(s, GroupVersion)
		return nil
	})
	AddToScheme = SchemeBuilder.AddToScheme
)
