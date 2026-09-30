package controller

import (
	"context"
	"testing"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileReportsMissingCredentialSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	account := &v1alpha1.CloudAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs"},
		Spec: v1alpha1.CloudAccountSpec{
			Provider:      "aws",
			AccountID:     "123456789012",
			CredentialRef: corev1.LocalObjectReference{Name: "aws-prod-credentials"},
		},
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(account).
		WithObjects(account).
		Build()

	reconciler := &CloudAccountReconciler{Client: client}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "costs", Name: "aws-prod"},
	}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var actual v1alpha1.CloudAccount
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: "aws-prod"}, &actual); err != nil {
		t.Fatal(err)
	}
	condition := apiMeta.FindStatusCondition(actual.Status.Conditions, "CredentialsReady")
	if condition == nil {
		t.Fatal("CredentialsReady condition was not set")
	}
	if condition.Status != metav1.ConditionFalse || condition.Reason != "SecretNotFound" {
		t.Fatalf("CredentialsReady = (%s, %s), want (False, SecretNotFound)", condition.Status, condition.Reason)
	}
}

func TestReconcileReportsCredentialSecretReady(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	account := &v1alpha1.CloudAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs"},
		Spec: v1alpha1.CloudAccountSpec{
			Provider:      "aws",
			AccountID:     "123456789012",
			CredentialRef: corev1.LocalObjectReference{Name: "aws-prod-credentials"},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod-credentials", Namespace: "costs"},
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(account).
		WithObjects(account, secret).
		Build()

	reconciler := &CloudAccountReconciler{Client: client}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "costs", Name: "aws-prod"},
	}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var actual v1alpha1.CloudAccount
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: "aws-prod"}, &actual); err != nil {
		t.Fatal(err)
	}
	condition := apiMeta.FindStatusCondition(actual.Status.Conditions, "CredentialsReady")
	if condition == nil {
		t.Fatal("CredentialsReady condition was not set")
	}
	if condition.Status != metav1.ConditionTrue || condition.Reason != "SecretFound" {
		t.Fatalf("CredentialsReady = (%s, %s), want (True, SecretFound)", condition.Status, condition.Reason)
	}
}
