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
	// image and the tag or digest to run explicitly. A digest names one build and
	// a tag is accepted; what the holder of a tag accepts is that a restart can
	// bring different bytes under the same name, because every container of this
	// Pod is pulled at every start rather than read from a node's cache. An image
	// no registry serves cannot be run here for the same reason, however it
	// reached the node. It must run as uid 65532, which is the user every
	// container of the agent's Pod is given: the agent's credential is delivered
	// as a file that user owns, and the process that reads it is refused a file
	// owned by anyone else. It must also need no Linux capability, no privilege
	// it did not start with, and nothing the runtime's default seccomp profile
	// blocks, because the Pod this operator builds is one PodSecurity restricted
	// admits and grants none of the three.
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
	// conditions report what the controller observed of this Agent. Two types
	// are set, and they answer different questions:
	//
	// - "Synced": True when the cluster carries the workload this Agent's spec
	//   asks for. False when the Secret named by credentialsSecretName does not
	//   exist, and False when the spec was edited in a way the running workload
	//   cannot take. The reason says which.
	//
	// - "Available": True when the workload reports a ready replica. False when
	//   it reports none, which covers a replica still starting as much as one
	//   that cannot start. Unknown when the controller did not get as far as
	//   reading the workload. A replica is ready once its containers are
	//   running, and this workload carries no readiness probe, so no part of
	//   this says whether the agent inside those containers works.
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

// ConditionAvailable is the condition type reporting whether the workload an
// Agent's spec asks for is running. It is the name a Deployment carries for the
// same split: ConditionSynced reports on the declaration, and this reports on
// what the cluster is running behind it.
const ConditionAvailable = "Available"

// Reasons for the Available condition.
const (
	// ReasonReplicaReady is set when the workload reports a ready replica.
	ReasonReplicaReady = "ReplicaReady"

	// ReasonReplicaNotReady is set when the workload reports no ready replica.
	ReasonReplicaNotReady = "ReplicaNotReady"

	// ReasonWorkloadNotObserved is set when the controller stopped before
	// reconciling a workload, so it read none and observed no readiness.
	ReasonWorkloadNotObserved = "WorkloadNotObserved"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:validation:XValidation:rule="self.metadata.name.size() <= 52",message="metadata.name must be 52 characters or fewer, because the Pods of this Agent's workload carry the name with a suffix of up to 11 characters in a label, and a label value stops at 63"
// +kubebuilder:printcolumn:name="Synced",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].reason`
// +kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.conditions[?(@.type=="Available")].status`
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
