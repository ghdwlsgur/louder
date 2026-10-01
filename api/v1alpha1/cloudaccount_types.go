package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// CloudAccountSpec defines the desired state for one billing account.
type CloudAccountSpec struct {
	// Provider identifies the cloud adapter used for collection.
	// +kubebuilder:validation:Enum=aws;gcp;azure
	Provider string `json:"provider"`

	// AccountID identifies the billing scope at the provider.
	AccountID string `json:"accountId"`

	// CredentialRef names a Kubernetes Secret populated through Vault and ESO.
	CredentialRef corev1.LocalObjectReference `json:"credentialRef"`

	// ProviderConfig contains non-secret settings interpreted by the selected provider.
	// +optional
	ProviderConfig map[string]string `json:"providerConfig,omitempty"`

	// Collection controls the collector schedule.
	Collection CollectionSpec `json:"collection"`

	// Metadata contains organization-level cost dimensions.
	// +optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

type CollectionSpec struct {
	Enabled  bool   `json:"enabled"`
	Schedule string `json:"schedule"`
}

// CloudAccountStatus reports control-plane and collection state.
type CloudAccountStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	LastCollectionTime *metav1.Time `json:"lastCollectionTime,omitempty"`
	// +optional
	LastSuccessfulCollectionTime *metav1.Time `json:"lastSuccessfulCollectionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ca
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=".spec.provider"
// +kubebuilder:printcolumn:name="Credentials",type="string",JSONPath=".status.conditions[?(@.type=='CredentialsReady')].status"
// CloudAccount declares a cloud billing account to collect.
type CloudAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudAccountSpec   `json:"spec,omitempty"`
	Status CloudAccountStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// CloudAccountList contains CloudAccount objects.
type CloudAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudAccount `json:"items"`
}

func (in *CloudAccount) DeepCopyInto(out *CloudAccount) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec.Metadata != nil {
		out.Spec.Metadata = make(map[string]string, len(in.Spec.Metadata))
		for key, value := range in.Spec.Metadata {
			out.Spec.Metadata[key] = value
		}
	}
	if in.Spec.ProviderConfig != nil {
		out.Spec.ProviderConfig = make(map[string]string, len(in.Spec.ProviderConfig))
		for key, value := range in.Spec.ProviderConfig {
			out.Spec.ProviderConfig[key] = value
		}
	}
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		copy(out.Status.Conditions, in.Status.Conditions)
	}
	if in.Status.LastCollectionTime != nil {
		out.Status.LastCollectionTime = in.Status.LastCollectionTime.DeepCopy()
	}
	if in.Status.LastSuccessfulCollectionTime != nil {
		out.Status.LastSuccessfulCollectionTime = in.Status.LastSuccessfulCollectionTime.DeepCopy()
	}
}

func (in *CloudAccount) DeepCopy() *CloudAccount {
	if in == nil {
		return nil
	}
	out := new(CloudAccount)
	in.DeepCopyInto(out)
	return out
}

func (in *CloudAccount) DeepCopyObject() runtime.Object {
	if copy := in.DeepCopy(); copy != nil {
		return copy
	}
	return nil
}

func (in *CloudAccountList) DeepCopyInto(out *CloudAccountList) {
	*out = *in
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]CloudAccount, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *CloudAccountList) DeepCopy() *CloudAccountList {
	if in == nil {
		return nil
	}
	out := new(CloudAccountList)
	in.DeepCopyInto(out)
	return out
}

func (in *CloudAccountList) DeepCopyObject() runtime.Object {
	if copy := in.DeepCopy(); copy != nil {
		return copy
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&CloudAccount{}, &CloudAccountList{})
}
