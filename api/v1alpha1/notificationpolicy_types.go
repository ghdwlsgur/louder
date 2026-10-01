package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type NotificationPolicySpec struct {
	// +kubebuilder:validation:Enum=teams
	Type          string                      `json:"type"`
	CredentialRef corev1.LocalObjectReference `json:"credentialRef"`
	// Events identifies the event types this policy receives.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Enum=DailySummary;BudgetWarning;BudgetExceeded;BudgetThreshold;CostAnomaly;CollectionFailed
	Events []string `json:"events"`
	// +optional
	Selector map[string]string `json:"selector,omitempty"`
}

type NotificationPolicyStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// NotificationPolicy configures a provider-neutral notification destination.
type NotificationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NotificationPolicySpec   `json:"spec,omitempty"`
	Status            NotificationPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// NotificationPolicyList contains NotificationPolicy objects.
type NotificationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NotificationPolicy `json:"items"`
}

func (in *NotificationPolicy) DeepCopyInto(out *NotificationPolicy) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec.Selector != nil {
		out.Spec.Selector = make(map[string]string, len(in.Spec.Selector))
		for key, value := range in.Spec.Selector {
			out.Spec.Selector[key] = value
		}
	}
	if in.Spec.Events != nil {
		out.Spec.Events = make([]string, len(in.Spec.Events))
		copy(out.Spec.Events, in.Spec.Events)
	}
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		copy(out.Status.Conditions, in.Status.Conditions)
	}
}

func (in *NotificationPolicy) DeepCopy() *NotificationPolicy {
	if in == nil {
		return nil
	}
	out := new(NotificationPolicy)
	in.DeepCopyInto(out)
	return out
}

func (in *NotificationPolicy) DeepCopyObject() runtime.Object {
	if copy := in.DeepCopy(); copy != nil {
		return copy
	}
	return nil
}

func (in *NotificationPolicyList) DeepCopyInto(out *NotificationPolicyList) {
	*out = *in
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]NotificationPolicy, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *NotificationPolicyList) DeepCopy() *NotificationPolicyList {
	if in == nil {
		return nil
	}
	out := new(NotificationPolicyList)
	in.DeepCopyInto(out)
	return out
}

func (in *NotificationPolicyList) DeepCopyObject() runtime.Object {
	if copy := in.DeepCopy(); copy != nil {
		return copy
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&NotificationPolicy{}, &NotificationPolicyList{})
}
