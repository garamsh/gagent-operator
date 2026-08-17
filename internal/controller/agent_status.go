package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// something the object does not already carry. The comparison is against the
// status the API server holds, not against a decision this reconcile made:
// Status is observed state and never an input to what Reconcile does.
func (r *AgentReconciler) writeStatus(ctx context.Context, agent *agentv1alpha1.Agent, held agentv1alpha1.AgentStatus) error {
	if equality.Semantic.DeepEqual(held, agent.Status) {
		return nil
	}

	if err := r.Status().Update(ctx, agent); err != nil {
		return fmt.Errorf("update agent status: %w", err)
	}

	if synced := meta.FindStatusCondition(agent.Status.Conditions, agentv1alpha1.ConditionSynced); synced != nil {
		logf.FromContext(ctx).Info("Reported on the Agent", "synced", synced.Status, "reason", synced.Reason)
	}

	return nil
}
