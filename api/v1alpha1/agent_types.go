package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AgentSpec defines the desired state of Agent
type AgentSpec struct {
	// image is the container image the agent runs. It has no default: name the
	// image and the tag or digest to run explicitly.
	// +required
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// credentialsSecretName is the name of a Secret in the Agent's namespace
	// holding the agent's credential material. The Secret is created outside this
	// operator, and its keys are mounted into the agent as files.
	// +required
	// +kubebuilder:validation:MinLength=1
	CredentialsSecretName string `json:"credentialsSecretName"`

	// storageSize is the size of the persistent volume the agent keeps its state on.
	// +required
	// +kubebuilder:validation:XValidation:rule="quantity(string(self)).isGreaterThan(quantity('0'))",message="storageSize must be greater than zero"
	StorageSize resource.Quantity `json:"storageSize"`

	// storageClassName is the StorageClass the agent's persistent volume is
	// provisioned from. Unset means the cluster's default StorageClass.
	// +optional
	// +kubebuilder:validation:MinLength=1
	StorageClassName *string `json:"storageClassName,omitempty"`

	// resources are the compute resources the agent container requests and is
	// limited to. Unset leaves the container without requests or limits.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitzero"`
}

// AgentStatus defines the observed state of Agent.
type AgentStatus struct {
	// conditions represent the current state of the Agent resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the metadata.generation this status was last computed
	// from. A value behind metadata.generation means the status is stale.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Agent is the Schema for the agents API
type Agent struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Agent
	// +required
	Spec AgentSpec `json:"spec"`

	// status defines the observed state of Agent
	// +optional
	Status AgentStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AgentList contains a list of Agent
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Agent{}, &AgentList{})
		return nil
	})
}
