package controller

import (
	"context"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type CloudAccountReconciler struct {
	client.Client
	APIReader client.Reader
}

func (r *CloudAccountReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var account v1alpha1.CloudAccount
	if err := r.Get(ctx, req.NamespacedName, &account); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	var secret corev1.Secret
	secretReader := r.APIReader
	if secretReader == nil {
		secretReader = r.Client
	}
	err := secretReader.Get(ctx, types.NamespacedName{
		Namespace: account.Namespace,
		Name:      account.Spec.CredentialRef.Name,
	}, &secret)
	if apierrors.IsNotFound(err) {
		changed := apiMeta.SetStatusCondition(&account.Status.Conditions, metav1.Condition{
			Type:               "CredentialsReady",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: account.Generation,
			Reason:             "SecretNotFound",
			Message:            "The referenced credential Secret does not exist in the CloudAccount namespace.",
		})
		if changed {
			return reconcile.Result{}, r.Status().Update(ctx, &account)
		}
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	changed := apiMeta.SetStatusCondition(&account.Status.Conditions, metav1.Condition{
		Type:               "CredentialsReady",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: account.Generation,
		Reason:             "SecretFound",
		Message:            "The referenced credential Secret exists in the CloudAccount namespace.",
	})
	if changed {
		return reconcile.Result{}, r.Status().Update(ctx, &account)
	}

	return reconcile.Result{}, nil
}

func (r *CloudAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.APIReader = mgr.GetAPIReader()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.CloudAccount{}).
		WatchesMetadata(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.requestsForCredentialSecret)).
		Complete(r)
}

func (r *CloudAccountReconciler) requestsForCredentialSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*metav1.PartialObjectMetadata)
	if !ok {
		return nil
	}

	var accounts v1alpha1.CloudAccountList
	if err := r.List(ctx, &accounts, client.InNamespace(secret.Namespace)); err != nil {
		log.FromContext(ctx).Error(err, "unable to map credential Secret event to CloudAccounts")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(accounts.Items))
	for i := range accounts.Items {
		account := &accounts.Items[i]
		if account.Spec.CredentialRef.Name == secret.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: account.Namespace, Name: account.Name},
			})
		}
	}
	return requests
}
