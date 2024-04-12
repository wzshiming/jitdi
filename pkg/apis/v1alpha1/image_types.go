package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ImageKind is the kind for image.
	ImageKind = "Image"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:rbac:groups=jitdi.zsm.io,resources=images,verbs=create;delete;get;list;patch;update;watch

// Image is the Schema for the images API
type Image struct {
	//+k8s:conversion-gen=false
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata"`
	// Spec defines the desired state of Image
	Spec ImageSpec `json:"spec"`
	// Status defines the observed state of Image
	Status ImageStatus `json:"status,omitempty"`
}

// ImageStatus holds status for image
type ImageStatus struct {
	// Conditions holds conditions for image.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// ImageSpec holds the specification for image
type ImageSpec struct {
	Match     string   `json:"match,omitempty"`
	BaseImage string   `json:"baseImage,omitempty"`
	Mutates   []Mutate `json:"mutates,omitempty"`
}

// Mutate holds the mutate information
type Mutate struct {
	File   *File   `json:"file,omitempty"`
	Ollama *Ollama `json:"ollama,omitempty"`
}

// File holds the file information
type File struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode,omitempty"`
}

// Ollama holds the ollama information
type Ollama struct {
	Model   string `json:"model"`
	WorkDir string `json:"workDir"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// ImageList is a list of Image.
type ImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Image `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Image{}, &ImageList{})
}
