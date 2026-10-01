package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type BudgetAmount struct {
	Value    int64  `json:"value"`
	Currency string `json:"currency"`
}

type BudgetForecastSpec struct {
	Enabled bool `json:"enabled"`
}

type BudgetPolicySpec struct {
	Selector   map[string]string `json:"selector"`
	Amount     BudgetAmount      `json:"amount"`
	Thresholds []int32           `json:"thresholds,omitempty"`
	// Schedule opts this policy into recurring Analyzer evaluation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Schedule string              `json:"schedule,omitempty"`
	Forecast *BudgetForecastSpec `json:"forecast,omitempty"`
}

type BudgetPolicyStatus struct {
	// LastNotifiedMonth is the UTC billing month for the stored threshold receipts.
	// +optional
	LastNotifiedMonth string `json:"lastNotifiedMonth,omitempty"`
	// NotifiedThresholds contains thresholds delivered during LastNotifiedMonth.
	// +optional
	NotifiedThresholds []int32 `json:"notifiedThresholds,omitempty"`
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// BudgetPolicy defines provider-neutral cost thresholds.
type BudgetPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BudgetPolicySpec   `json:"spec,omitempty"`
	Status            BudgetPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// BudgetPolicyList contains BudgetPolicy objects.
type BudgetPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BudgetPolicy `json:"items"`
}

func (in *BudgetPolicy) DeepCopyInto(out *BudgetPolicy) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec.Selector != nil {
		out.Spec.Selector = make(map[string]string, len(in.Spec.Selector))
		for key, value := range in.Spec.Selector {
			out.Spec.Selector[key] = value
		}
	}
	if in.Spec.Thresholds != nil {
		out.Spec.Thresholds = append([]int32(nil), in.Spec.Thresholds...)
	}
	if in.Spec.Forecast != nil {
		out.Spec.Forecast = new(BudgetForecastSpec)
		*out.Spec.Forecast = *in.Spec.Forecast
	}
	if in.Status.NotifiedThresholds != nil {
		out.Status.NotifiedThresholds = append([]int32(nil), in.Status.NotifiedThresholds...)
	}
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		copy(out.Status.Conditions, in.Status.Conditions)
	}
}

func (in *BudgetPolicy) DeepCopy() *BudgetPolicy {
	if in == nil {
		return nil
	}
	out := new(BudgetPolicy)
	in.DeepCopyInto(out)
	return out
}

func (in *BudgetPolicy) DeepCopyObject() runtime.Object {
	if copy := in.DeepCopy(); copy != nil {
		return copy
	}
	return nil
}

func (in *BudgetPolicyList) DeepCopyInto(out *BudgetPolicyList) {
	*out = *in
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]BudgetPolicy, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *BudgetPolicyList) DeepCopy() *BudgetPolicyList {
	if in == nil {
		return nil
	}
	out := new(BudgetPolicyList)
	in.DeepCopyInto(out)
	return out
}

func (in *BudgetPolicyList) DeepCopyObject() runtime.Object {
	if copy := in.DeepCopy(); copy != nil {
		return copy
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&BudgetPolicy{}, &BudgetPolicyList{})
}
