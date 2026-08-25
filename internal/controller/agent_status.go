package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/garamsh/gagent-operator/api/v1alpha1"
)

// setSynced records on the Agent what this reconcile observed of the workload
// its spec asks for.
func setSynced(agent *agentv1alpha1.Agent, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
		Type:               agentv1alpha1.ConditionSynced,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: agent.Generation,
	})
}

// writeStatus writes the status this reconcile observed, and only when it says
// something the object does not already carry. held is the Agent as this
// reconcile read it, so the comparison is against the status the API server
// holds and not against a decision this reconcile made: Status is observed state
// and never an input to what Reconcile does.
//
// A patch and not an update: the poller reports status.agent on an Agent this
// operator constructed, and an update carries the resource version its object
// was read at, so a write landing between the read and it is refused. A merge
// patch carries no resource version and names only the fields this reconcile
// decided, which is what makes the two writers' disjoint fields disjoint writes.
func (r *AgentReconciler) writeStatus(ctx context.Context, agent, held *agentv1alpha1.Agent) error {
	if equality.Semantic.DeepEqual(held.Status, agent.Status) {
		return nil
	}

	if err := r.Status().Patch(ctx, agent, client.MergeFrom(held)); err != nil {
		return fmt.Errorf("patch agent status: %w", err)
	}

	if synced := meta.FindStatusCondition(agent.Status.Conditions, agentv1alpha1.ConditionSynced); synced != nil {
		logf.FromContext(ctx).Info("Reported on the Agent", "synced", synced.Status, "reason", synced.Reason)
	}

	return nil
}
