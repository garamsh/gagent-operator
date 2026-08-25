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
	// conditions report what the controller observed of this Agent. One type is
	// set:
	//
	// - "Synced": True when the cluster carries the workload this Agent's spec
	//   asks for. False when the Secret named by credentialsSecretName does not
	//   exist, and False when the spec was edited in a way the running workload
	//   cannot take. The reason says which.
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

	// agent is the garam resource name of the agent this Agent was constructed
	// for, and is empty on an Agent a user wrote. garam mints it when an
	// organization defines an agent and this operator is admitted to it by
	// claiming the definition, so it is reported here and never asked for in
	// the spec.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Agent string `json:"agent,omitempty"`
}

// ConditionSynced is the condition type reporting whether the cluster carries
// the workload an Agent's spec asks for.
const ConditionSynced = "Synced"

// Reasons for the Synced condition.
const (
	// ReasonWorkloadReconciled is set when the workload matches the spec.
	ReasonWorkloadReconciled = "WorkloadReconciled"

	// ReasonCredentialsSecretMissing is set when the Secret the spec names does
	// not exist, which leaves the workload unbuilt.
	ReasonCredentialsSecretMissing = "CredentialsSecretMissing"

	// ReasonStorageSizeImmutable is set when the spec asks for a volume size the
	// workload cannot be changed to.
	ReasonStorageSizeImmutable = "StorageSizeImmutable"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:validation:XValidation:rule="self.metadata.name.size() <= 52",message="metadata.name must be 52 characters or fewer, because the Pods of this Agent's workload carry the name with a suffix of up to 11 characters in a label, and a label value stops at 63"
// +kubebuilder:printcolumn:name="Synced",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].reason`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

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
