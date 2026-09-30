package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileReportsMissingCredentialSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
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
			AccountID:     "synthetic-account-id",
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

func TestReconcileCreatesCollectorCronJob(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		batchv1.AddToScheme,
		v1alpha1.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}

	account := &v1alpha1.CloudAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs", UID: types.UID("account-uid")},
		Spec: v1alpha1.CloudAccountSpec{
			Provider:      "aws",
			AccountID:     "synthetic-account-id",
			CredentialRef: corev1.LocalObjectReference{Name: "aws-prod-credentials"},
			Collection:    v1alpha1.CollectionSpec{Enabled: true, Schedule: "0 */6 * * *"},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod-credentials", Namespace: "costs"},
		Data:       map[string][]byte{"access-key": []byte("synthetic-secret")},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(account).WithObjects(account, secret).Build()
	r := &CloudAccountReconciler{Client: c, CollectorImage: "louder:local", CollectorFixtureMode: true, Scheme: scheme}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "costs", Name: "aws-prod"}}
	if _, err := r.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var actual batchv1.CronJob
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: "aws-prod-collector"}, &actual); err != nil {
		t.Fatalf("expected Collector CronJob: %v", err)
	}
	if actual.Spec.Schedule != account.Spec.Collection.Schedule {
		t.Errorf("schedule = %q, want %q", actual.Spec.Schedule, account.Spec.Collection.Schedule)
	}
	if actual.Spec.JobTemplate.Labels["finops.sre.local/cloud-account"] != accountLabelValue(account.Name) {
		t.Errorf("Job template labels = %#v, want CloudAccount selector", actual.Spec.JobTemplate.Labels)
	}
	if actual.Spec.JobTemplate.Annotations[cloudAccountAnnotation] != account.Name {
		t.Errorf("Job template annotations = %#v, want CloudAccount mapping", actual.Spec.JobTemplate.Annotations)
	}
	if len(actual.OwnerReferences) != 1 || actual.OwnerReferences[0].UID != account.UID {
		t.Errorf("owner references = %#v, want CloudAccount UID %q", actual.OwnerReferences, account.UID)
	}
	container := actual.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
	if actual.Spec.JobTemplate.Spec.Template.Spec.AutomountServiceAccountToken == nil || *actual.Spec.JobTemplate.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Error("Collector pod should not receive a Kubernetes API token")
	}
	if container.Image != "louder:local" {
		t.Errorf("collector image = %q, want louder:local", container.Image)
	}
	if len(container.EnvFrom) != 1 || container.EnvFrom[0].SecretRef == nil || container.EnvFrom[0].SecretRef.Name != secret.Name {
		t.Errorf("credential Secret reference = %#v, want reference to %q", container.EnvFrom, secret.Name)
	}
	if len(container.Env) != 0 {
		t.Errorf("literal environment variables = %#v, want credentials referenced through Secret only", container.Env)
	}
	if len(container.Args) == 0 {
		t.Fatal("Collector arguments are empty")
	}
	firstResourceVersion := actual.ResourceVersion
	if _, err := r.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: actual.Name}, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.ResourceVersion != firstResourceVersion {
		t.Errorf("idempotent reconcile changed CronJob resourceVersion from %q to %q", firstResourceVersion, actual.ResourceVersion)
	}
}

func TestReconcileRemovesCollectorCronJobWhenCollectionDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{corev1.AddToScheme, batchv1.AddToScheme, v1alpha1.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	account := &v1alpha1.CloudAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs", UID: types.UID("account-uid")},
		Spec: v1alpha1.CloudAccountSpec{
			Provider:      "aws",
			AccountID:     "synthetic-account-id",
			CredentialRef: corev1.LocalObjectReference{Name: "aws-prod-credentials"},
			Collection:    v1alpha1.CollectionSpec{Enabled: false, Schedule: "0 */6 * * *"},
		},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "aws-prod-credentials", Namespace: "costs"}}
	cronJob := collectorCronJob(account, "louder:local", true)
	if err := controllerutil.SetControllerReference(account, cronJob, scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(account).WithObjects(account, secret, cronJob).Build()
	r := &CloudAccountReconciler{Client: c, Scheme: scheme}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "costs", Name: "aws-prod"}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var actual batchv1.CronJob
	err := c.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: "aws-prod-collector"}, &actual)
	if err == nil {
		t.Fatal("Collector CronJob still exists while collection is disabled")
	}
}

func TestReconcileDoesNotAdoptUnownedCollectorCronJob(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{corev1.AddToScheme, batchv1.AddToScheme, v1alpha1.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	account := &v1alpha1.CloudAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs", UID: types.UID("account-uid")},
		Spec: v1alpha1.CloudAccountSpec{
			Provider:      "aws",
			AccountID:     "synthetic-account",
			CredentialRef: corev1.LocalObjectReference{Name: "aws-prod-credentials"},
			Collection:    v1alpha1.CollectionSpec{Enabled: true, Schedule: "0 */6 * * *"},
		},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "aws-prod-credentials", Namespace: "costs"}}
	unowned := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: "aws-prod-collector", Namespace: "costs"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(account).WithObjects(account, secret, unowned).Build()
	r := &CloudAccountReconciler{Client: c, CollectorImage: "louder:local", CollectorFixtureMode: true, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "costs", Name: "aws-prod"}})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an ownership conflict")
	}

	var actual batchv1.CronJob
	if getErr := c.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: unowned.Name}, &actual); getErr != nil {
		t.Fatalf("unowned CronJob was deleted: %v", getErr)
	}
	if len(actual.OwnerReferences) != 0 {
		t.Errorf("unowned CronJob gained owner references: %#v", actual.OwnerReferences)
	}
}

func TestReconcileRemovesCollectorCronJobWhenCredentialSecretDisappears(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{corev1.AddToScheme, batchv1.AddToScheme, v1alpha1.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	account := &v1alpha1.CloudAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs", UID: types.UID("account-uid")},
		Spec: v1alpha1.CloudAccountSpec{
			Provider:      "aws",
			AccountID:     "synthetic-account",
			CredentialRef: corev1.LocalObjectReference{Name: "aws-prod-credentials"},
			Collection:    v1alpha1.CollectionSpec{Enabled: true, Schedule: "0 */6 * * *"},
		},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "aws-prod-credentials", Namespace: "costs"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(account).WithObjects(account, secret).Build()
	r := &CloudAccountReconciler{Client: c, CollectorImage: "louder:local", CollectorFixtureMode: true, Scheme: scheme}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "costs", Name: "aws-prod"}}
	if _, err := r.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("initial Reconcile() error = %v", err)
	}
	if err := c.Delete(context.Background(), secret); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("Reconcile() after Secret deletion error = %v", err)
	}
	var actual batchv1.CronJob
	err := c.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: "aws-prod-collector"}, &actual)
	if err == nil {
		t.Fatal("Collector CronJob still exists after its credential Secret disappeared")
	}
}

func TestReconcileReportsCollectorJobSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{corev1.AddToScheme, batchv1.AddToScheme, v1alpha1.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	completedAt := metav1.NewTime(time.Date(2026, 9, 30, 1, 2, 3, 0, time.UTC))
	account := &v1alpha1.CloudAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs", UID: types.UID("account-uid")},
		Spec: v1alpha1.CloudAccountSpec{
			Provider:      "aws",
			AccountID:     "synthetic-account-id",
			CredentialRef: corev1.LocalObjectReference{Name: "aws-prod-credentials"},
			Collection:    v1alpha1.CollectionSpec{Enabled: true, Schedule: "0 */6 * * *"},
		},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: account.Spec.CredentialRef.Name, Namespace: account.Namespace}}
	cronJob := collectorCronJob(account, "louder:local", true)
	cronJob.UID = types.UID("cronjob-uid")
	if err := controllerutil.SetControllerReference(account, cronJob, scheme); err != nil {
		t.Fatal(err)
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aws-prod-collector-run", Namespace: account.Namespace,
			Labels:            map[string]string{"app.kubernetes.io/name": "louder-collector", "finops.sre.local/cloud-account": accountLabelValue(account.Name)},
			Annotations:       map[string]string{"finops.sre.local/cloud-account": account.Name},
			CreationTimestamp: completedAt,
			OwnerReferences:   []metav1.OwnerReference{{APIVersion: batchv1.SchemeGroupVersion.String(), Kind: "CronJob", Name: cronJob.Name, UID: cronJob.UID, Controller: ptr.To(true)}},
		},
		Status: batchv1.JobStatus{
			CompletionTime: &completedAt,
			Conditions:     []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue, LastTransitionTime: completedAt}},
		},
	}
	olderAttemptTime := metav1.NewTime(completedAt.Add(-24 * time.Hour))
	olderFailure := job.DeepCopy()
	olderFailure.Name = "aws-prod-collector-older-failure"
	olderFailure.CreationTimestamp = olderAttemptTime
	olderFailure.Status = batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: olderAttemptTime}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(account).WithObjects(account, secret, cronJob, job, olderFailure).Build()
	r := &CloudAccountReconciler{Client: c, CollectorImage: "louder:local", CollectorFixtureMode: true, Scheme: scheme}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: account.Namespace, Name: account.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var actual v1alpha1.CloudAccount
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: account.Namespace, Name: account.Name}, &actual); err != nil {
		t.Fatal(err)
	}
	condition := apiMeta.FindStatusCondition(actual.Status.Conditions, "CollectionReady")
	if condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != "CollectionSucceeded" {
		t.Fatalf("CollectionReady = %#v, want True/CollectionSucceeded", condition)
	}
	if actual.Status.LastCollectionTime == nil || !actual.Status.LastCollectionTime.Equal(&completedAt) {
		t.Errorf("lastCollectionTime = %v, want %v", actual.Status.LastCollectionTime, completedAt)
	}
	if actual.Status.LastSuccessfulCollectionTime == nil || !actual.Status.LastSuccessfulCollectionTime.Equal(&completedAt) {
		t.Errorf("lastSuccessfulCollectionTime = %v, want %v", actual.Status.LastSuccessfulCollectionTime, completedAt)
	}
}

func TestReconcileReportsCollectorJobFailureWithoutLeakingMessage(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{corev1.AddToScheme, batchv1.AddToScheme, v1alpha1.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	previousSuccess := metav1.NewTime(time.Date(2026, 9, 29, 1, 2, 3, 0, time.UTC))
	failureTime := metav1.NewTime(time.Date(2026, 9, 30, 1, 2, 3, 0, time.UTC))
	account := &v1alpha1.CloudAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs", UID: types.UID("account-uid")},
		Spec: v1alpha1.CloudAccountSpec{
			Provider:      "aws",
			AccountID:     "synthetic-account-id",
			CredentialRef: corev1.LocalObjectReference{Name: "aws-prod-credentials"},
			Collection:    v1alpha1.CollectionSpec{Enabled: true, Schedule: "0 */6 * * *"},
		},
		Status: v1alpha1.CloudAccountStatus{
			LastCollectionTime:           previousSuccess.DeepCopy(),
			LastSuccessfulCollectionTime: previousSuccess.DeepCopy(),
		},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: account.Spec.CredentialRef.Name, Namespace: account.Namespace}}
	cronJob := collectorCronJob(account, "louder:local", true)
	cronJob.UID = types.UID("cronjob-uid")
	if err := controllerutil.SetControllerReference(account, cronJob, scheme); err != nil {
		t.Fatal(err)
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aws-prod-collector-failed", Namespace: account.Namespace,
			Labels:            map[string]string{"app.kubernetes.io/name": "louder-collector", "finops.sre.local/cloud-account": accountLabelValue(account.Name)},
			Annotations:       map[string]string{cloudAccountAnnotation: account.Name},
			CreationTimestamp: failureTime,
			OwnerReferences:   []metav1.OwnerReference{{APIVersion: batchv1.SchemeGroupVersion.String(), Kind: "CronJob", Name: cronJob.Name, UID: cronJob.UID, Controller: ptr.To(true)}},
		},
		Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{
			Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
			Reason: "BackoffLimitExceeded", Message: "sensitive error text must stay out of status",
			LastTransitionTime: failureTime,
		}}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(account).WithObjects(account, secret, cronJob, job).Build()
	r := &CloudAccountReconciler{Client: c, CollectorImage: "louder:local", CollectorFixtureMode: true, Scheme: scheme}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: account.Namespace, Name: account.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var actual v1alpha1.CloudAccount
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: account.Namespace, Name: account.Name}, &actual); err != nil {
		t.Fatal(err)
	}
	condition := apiMeta.FindStatusCondition(actual.Status.Conditions, "CollectionReady")
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "CollectorJobFailed" {
		t.Fatalf("CollectionReady = %#v, want False/CollectorJobFailed", condition)
	}
	if strings.Contains(condition.Message, "sensitive error text") {
		t.Errorf("CollectionReady message exposed the Job message: %q", condition.Message)
	}
	if actual.Status.LastCollectionTime == nil || !actual.Status.LastCollectionTime.Equal(&failureTime) {
		t.Errorf("lastCollectionTime = %v, want %v", actual.Status.LastCollectionTime, failureTime)
	}
	if actual.Status.LastSuccessfulCollectionTime == nil || !actual.Status.LastSuccessfulCollectionTime.Equal(&previousSuccess) {
		t.Errorf("lastSuccessfulCollectionTime = %v, want preserved value %v", actual.Status.LastSuccessfulCollectionTime, previousSuccess)
	}
}

func TestReconcileIgnoresCollectorJobWithDifferentCronJobOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{corev1.AddToScheme, batchv1.AddToScheme, v1alpha1.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	completedAt := metav1.NewTime(time.Date(2026, 9, 30, 1, 2, 3, 0, time.UTC))
	account := &v1alpha1.CloudAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs", UID: types.UID("account-uid")},
		Spec: v1alpha1.CloudAccountSpec{
			Provider:      "aws",
			AccountID:     "synthetic-account-id",
			CredentialRef: corev1.LocalObjectReference{Name: "aws-prod-credentials"},
			Collection:    v1alpha1.CollectionSpec{Enabled: true, Schedule: "0 */6 * * *"},
		},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: account.Spec.CredentialRef.Name, Namespace: account.Namespace}}
	cronJob := collectorCronJob(account, "louder:local", true)
	cronJob.UID = types.UID("cronjob-uid")
	if err := controllerutil.SetControllerReference(account, cronJob, scheme); err != nil {
		t.Fatal(err)
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "forged-collector-job", Namespace: account.Namespace,
			Labels:            map[string]string{"app.kubernetes.io/name": "louder-collector", "finops.sre.local/cloud-account": accountLabelValue(account.Name)},
			CreationTimestamp: completedAt,
			OwnerReferences:   []metav1.OwnerReference{{APIVersion: batchv1.SchemeGroupVersion.String(), Kind: "CronJob", Name: cronJob.Name, UID: "different-cronjob-uid", Controller: ptr.To(true)}},
		},
		Status: batchv1.JobStatus{
			CompletionTime: &completedAt,
			Conditions:     []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue, LastTransitionTime: completedAt}},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(account).WithObjects(account, secret, cronJob, job).Build()
	r := &CloudAccountReconciler{Client: c, CollectorImage: "louder:local", CollectorFixtureMode: true, Scheme: scheme}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: account.Namespace, Name: account.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var actual v1alpha1.CloudAccount
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: account.Namespace, Name: account.Name}, &actual); err != nil {
		t.Fatal(err)
	}
	if condition := apiMeta.FindStatusCondition(actual.Status.Conditions, "CollectionReady"); condition != nil {
		t.Errorf("CollectionReady = %#v, want no condition for an unowned Job", condition)
	}
}

func TestRequestsForCollectorJobUsesAnnotationNamespace(t *testing.T) {
	reconciler := &CloudAccountReconciler{}
	requests := reconciler.requestsForCollectorJob(context.Background(), &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "Job"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "collector-run", Namespace: "costs",
			Annotations: map[string]string{cloudAccountAnnotation: "aws-prod"},
		},
	})
	if len(requests) != 1 || requests[0].NamespacedName != (types.NamespacedName{Namespace: "costs", Name: "aws-prod"}) {
		t.Fatalf("requests = %#v, want costs/aws-prod", requests)
	}
}

func TestReconcileReportsCredentialSecretReady(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
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
			AccountID:     "synthetic-account-id",
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
