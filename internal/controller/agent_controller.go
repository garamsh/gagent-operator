package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/garamsh/gagent-operator/api/v1alpha1"
)

// AgentReconciler reconciles a Agent object
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.garam.sh,resources=agents,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile drives the workload an Agent describes toward the Agent's spec.
func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var agent agentv1alpha1.Agent
	if err := r.Get(ctx, req.NamespacedName, &agent); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("get agent: %w", err)
	}

	credentials := client.ObjectKey{Namespace: agent.Namespace, Name: agent.Spec.CredentialsSecretName}
	if err := r.credentialsExist(ctx, credentials); err != nil {
		if apierrors.IsNotFound(err) {
			// Creating the Secret is not a spec edit and wakes nothing on its
			// own; the watch on Secrets is what brings this Agent back.
			logf.FromContext(ctx).Info("Waiting for the credentials Secret", "secret", credentials.Name)

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("get credentials secret: %w", err)
	}

	if err := r.reconcileStatefulSet(ctx, &agent); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// credentialsExist reads the metadata of the Secret an Agent names, and nothing
// else of it. The agent reads its credentials as mounted files, so no part of
// this operator — its cache included — holds the key material.
func (r *AgentReconciler) credentialsExist(ctx context.Context, key client.ObjectKey) error {
	secret := &metav1.PartialObjectMetadata{}
	secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))

	return r.Get(ctx, key, secret)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.Agent{}).
		Owns(&appsv1.StatefulSet{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.agentsNamingSecret), builder.OnlyMetadata).
		Named("agent").
		Complete(r)
}

// agentsNamingSecret maps a Secret to the Agents whose spec names it, so that
// the Secret's arrival wakes the Agents that were waiting for it.
func (r *AgentReconciler) agentsNamingSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	var agents agentv1alpha1.AgentList
	if err := r.List(ctx, &agents, client.InNamespace(secret.GetNamespace())); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list Agents for a Secret", "secret", secret.GetName())

		return nil
	}

	var requests []reconcile.Request
	for i := range agents.Items {
		if agents.Items[i].Spec.CredentialsSecretName == secret.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&agents.Items[i])})
		}
	}

	return requests
}
