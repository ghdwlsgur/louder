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

func TestRequestsForCredentialSecretOnlyEnqueuesReferencingAccounts(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	account := func(name, namespace, credentialSecret string) *v1alpha1.CloudAccount {
		return &v1alpha1.CloudAccount{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: v1alpha1.CloudAccountSpec{
				CredentialRef: corev1.LocalObjectReference{Name: credentialSecret},
			},
		}
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			account("aws-one", "costs", "shared-credentials"),
			account("aws-two", "costs", "shared-credentials"),
			account("gcp-other", "costs", "other-credentials"),
			account("aws-another-namespace", "other-costs", "shared-credentials"),
		).
		Build()

	reconciler := &CloudAccountReconciler{Client: client}
	requests := reconciler.requestsForCredentialSecret(context.Background(), &metav1.PartialObjectMetadata{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: "shared-credentials", Namespace: "costs"},
	})
	if len(requests) != 2 {
		t.Fatalf("got %d reconcile requests, want 2", len(requests))
	}
	want := map[types.NamespacedName]bool{
		{Namespace: "costs", Name: "aws-one"}: true,
		{Namespace: "costs", Name: "aws-two"}: true,
	}
	for _, request := range requests {
		if !want[request.NamespacedName] {
			t.Errorf("unexpected reconcile request: %s", request.NamespacedName)
		}
		delete(want, request.NamespacedName)
	}
	if len(want) != 0 {
		t.Errorf("missing reconcile requests: %v", want)
	}
}
