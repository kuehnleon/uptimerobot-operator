package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AlertContactSpec defines a personal email or mobile push alert contact.
// +kubebuilder:validation:XValidation:rule="(self.type == 'Email' && has(self.value) && !has(self.pushTokenSecretRef)) || (self.type in ['MobileAppIOS', 'MobileAppAndroid'] && !has(self.value) && has(self.pushTokenSecretRef))",message="Email requires value; mobile push requires pushTokenSecretRef"
// +kubebuilder:validation:XValidation:rule="self.type == oldSelf.type && has(self.value) == has(oldSelf.value) && (!has(self.value) || self.value == oldSelf.value) && has(self.pushTokenSecretRef) == has(oldSelf.pushTokenSecretRef) && (!has(self.pushTokenSecretRef) || self.pushTokenSecretRef == oldSelf.pushTokenSecretRef) && has(self.deviceName) == has(oldSelf.deviceName) && (!has(self.deviceName) || self.deviceName == oldSelf.deviceName)",message="type, value, pushTokenSecretRef and deviceName are immutable; create a new AlertContact instead"
type AlertContactSpec struct {
	ResourcePolicySpec `json:",inline"`
	// +kubebuilder:validation:Enum=Email;MobileAppIOS;MobileAppAndroid
	Type string `json:"type"`
	// Defaults to the resource name.
	// +optional
	FriendlyName string `json:"friendlyName,omitempty"`
	// Email address. Required for Email contacts.
	// +optional
	Value string `json:"value,omitempty"`
	// Push token, read only from this Secret key. Required for mobile push contacts.
	// +optional
	PushTokenSecretRef *corev1.SecretKeySelector `json:"pushTokenSecretRef,omitempty"`
	// Device name for mobile push contacts.
	// +optional
	DeviceName string `json:"deviceName,omitempty"`
	// Which monitor events this contact is notified for (API values 0-3).
	// +kubebuilder:validation:Enum=0;1;2;3
	// +optional
	EnableNotificationsFor *int32 `json:"enableNotificationsFor,omitempty"`
	// Send notifications about SSL certificate expiration.
	// +optional
	SSLExpirationReminder *bool `json:"sslExpirationReminder,omitempty"`
	// Pause notifications for this contact.
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// AlertContact is a personal UptimeRobot email or mobile push alert contact.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=uptimerobot,shortName=ac
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type AlertContact struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AlertContactSpec `json:"spec"`
	Status ResourceStatus   `json:"status,omitempty"`
}

func (a *AlertContact) GetResourceStatus() *ResourceStatus { return &a.Status }
func (a *AlertContact) GetDeletionPolicy() string          { return a.Spec.DeletionPolicy }

// +kubebuilder:object:root=true
type AlertContactList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AlertContact `json:"items"`
}
