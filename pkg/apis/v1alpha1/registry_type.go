package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// RegistryKind is the kind for registry.
	RegistryKind = "Registry"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:rbac:groups=jitdi.zsm.io,resources=registries,verbs=create;delete;get;list;patch;update;watch

// Registry is the Schema for the registrys API
type Registry struct {
	//+k8s:conversion-gen=false
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata"`
	// Spec defines the desired state of Registry
	Spec RegistrySpec `json:"spec"`
	// Status defines the observed state of Registry
	Status RegistryStatus `json:"status,omitempty"`
}

// RegistryStatus holds status for registry
type RegistryStatus struct {
	// Conditions holds conditions for registry.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// RegistrySpec holds the specification for registry
type RegistrySpec struct {
	Endpoint       string          `json:"endpoint,omitempty"`
	Insecure       bool            `json:"insecure,omitempty"`
	Authentication *Authentication `json:"authentication,omitempty"`
}

type Authentication struct {
	BaseAuth *BaseAuth `json:"baseAuth,omitempty"`
}

type BaseAuth struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// RegistryList is a list of Registry.
type RegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Registry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Registry{}, &RegistryList{})
}
