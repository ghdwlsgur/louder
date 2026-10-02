package controller

import (
	"context"
	"testing"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestBudgetPolicyReconcileCreatesAnalyzerCronJob(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{corev1.AddToScheme, batchv1.AddToScheme, v1alpha1.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	budget := &v1alpha1.BudgetPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly", Namespace: "costs", UID: types.UID("budget-uid")},
		Spec:       v1alpha1.BudgetPolicySpec{Schedule: "0 * * * *", Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget).Build()
	reconciler := &BudgetPolicyReconciler{Client: kube, AnalyzerImage: "louder:local", StorageSecretName: "clickhouse", AnalyzerServiceAccountName: "louder-analyzer", Scheme: scheme}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: budget.Namespace, Name: budget.Name}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var actual batchv1.CronJob
	if err := kube.Get(context.Background(), types.NamespacedName{Namespace: budget.Namespace, Name: "sre-monthly-analyzer"}, &actual); err != nil {
		t.Fatalf("expected Analyzer CronJob: %v", err)
	}
	if actual.Spec.Schedule != budget.Spec.Schedule {
		t.Errorf("schedule = %q, want %q", actual.Spec.Schedule, budget.Spec.Schedule)
	}
	if actual.Spec.ConcurrencyPolicy != batchv1.ForbidConcurrent {
		t.Errorf("concurrency policy = %q, want Forbid", actual.Spec.ConcurrencyPolicy)
	}
	if actual.Spec.TimeZone == nil || *actual.Spec.TimeZone != "Etc/UTC" {
		t.Errorf("time zone = %v, want Etc/UTC", actual.Spec.TimeZone)
	}
	if len(actual.OwnerReferences) != 1 || actual.OwnerReferences[0].UID != budget.UID {
		t.Errorf("owner references = %#v, want BudgetPolicy UID %q", actual.OwnerReferences, budget.UID)
	}
	container := actual.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
	if container.Command[0] != "/louder-analyzer" {
		t.Errorf("command = %#v, want /louder-analyzer", container.Command)
	}
	if actual.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName != "louder-analyzer" {
		t.Errorf("service account = %q, want louder-analyzer", actual.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName)
	}
	container = actual.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
	if container.Image != "louder:local" || len(container.Args) != 2 || container.Args[0] != "--namespace=costs" || container.Args[1] != "--budget-policy=sre-monthly" {
		t.Errorf("Analyzer container = %#v, want image and policy-specific args", container)
	}
	if len(container.EnvFrom) != 1 || container.EnvFrom[0].SecretRef == nil || container.EnvFrom[0].SecretRef.Name != "clickhouse" {
		t.Errorf("storage environment = %#v, want ClickHouse Secret reference", container.EnvFrom)
	}
	if actual.Spec.JobTemplate.Spec.Template.Spec.AutomountServiceAccountToken == nil || !*actual.Spec.JobTemplate.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Error("Analyzer pod must mount its namespace-scoped Kubernetes API token")
	}
	actual.Spec.Suspend = ptr.To(false)
	actual.Spec.JobTemplate.Spec.Template.Spec.DNSPolicy = corev1.DNSClusterFirst
	if err := kube.Update(context.Background(), &actual); err != nil {
		t.Fatal(err)
	}
	resourceVersion := actual.ResourceVersion
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("Reconcile() with API-defaulted fields error = %v", err)
	}
	if err := kube.Get(context.Background(), types.NamespacedName{Namespace: budget.Namespace, Name: actual.Name}, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.ResourceVersion != resourceVersion {
		t.Errorf("reconcile changed resourceVersion from %q to %q for API-defaulted fields", resourceVersion, actual.ResourceVersion)
	}
	if actual.Spec.Suspend == nil || *actual.Spec.Suspend != false || actual.Spec.JobTemplate.Spec.Template.Spec.DNSPolicy != corev1.DNSClusterFirst {
		t.Errorf("API defaults were not preserved: suspend=%v dnsPolicy=%q", actual.Spec.Suspend, actual.Spec.JobTemplate.Spec.Template.Spec.DNSPolicy)
	}
}

func TestBudgetPolicyReconcileDeletesCronJobWhenScheduleIsRemoved(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{corev1.AddToScheme, batchv1.AddToScheme, v1alpha1.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	budget := &v1alpha1.BudgetPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly", Namespace: "costs", UID: types.UID("budget-uid")},
		Spec:       v1alpha1.BudgetPolicySpec{Schedule: "0 * * * *"},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget).Build()
	reconciler := &BudgetPolicyReconciler{Client: kube, AnalyzerImage: "louder:local", StorageSecretName: "clickhouse", AnalyzerServiceAccountName: "louder-analyzer", Scheme: scheme}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: budget.Namespace, Name: budget.Name}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("initial Reconcile() error = %v", err)
	}
	var actualBudget v1alpha1.BudgetPolicy
	if err := kube.Get(context.Background(), request.NamespacedName, &actualBudget); err != nil {
		t.Fatal(err)
	}
	actualBudget.Spec.Schedule = ""
	if err := kube.Update(context.Background(), &actualBudget); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("Reconcile() after removing schedule error = %v", err)
	}
	var cronJob batchv1.CronJob
	err := kube.Get(context.Background(), types.NamespacedName{Namespace: budget.Namespace, Name: analyzerCronJobName(budget.Name)}, &cronJob)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("Analyzer CronJob get error = %v, want NotFound", err)
	}
}

func TestBudgetPolicyReconcileUpdatesCronJobWhenScheduleChanges(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{corev1.AddToScheme, batchv1.AddToScheme, v1alpha1.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly", Namespace: "costs", UID: types.UID("budget-uid")}, Spec: v1alpha1.BudgetPolicySpec{Schedule: "0 * * * *"}}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget).Build()
	reconciler := &BudgetPolicyReconciler{Client: kube, AnalyzerImage: "louder:local", StorageSecretName: "clickhouse", AnalyzerServiceAccountName: "louder-analyzer", Scheme: scheme}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: budget.Namespace, Name: budget.Name}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("initial Reconcile() error = %v", err)
	}
	var actualBudget v1alpha1.BudgetPolicy
	if err := kube.Get(context.Background(), request.NamespacedName, &actualBudget); err != nil {
		t.Fatal(err)
	}
	actualBudget.Spec.Schedule = "30 * * * *"
	if err := kube.Update(context.Background(), &actualBudget); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("Reconcile() after schedule change error = %v", err)
	}
	var actual batchv1.CronJob
	if err := kube.Get(context.Background(), types.NamespacedName{Namespace: budget.Namespace, Name: analyzerCronJobName(budget.Name)}, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Spec.Schedule != "30 * * * *" {
		t.Errorf("schedule = %q, want updated schedule", actual.Spec.Schedule)
	}
}
