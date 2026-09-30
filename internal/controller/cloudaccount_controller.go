package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type CloudAccountReconciler struct {
	client.Client
	APIReader                  client.Reader
	CollectorImage             string
	CollectorFixtureMode       bool
	CollectorStorageSecretName string
	Scheme                     *runtime.Scheme
}

const cloudAccountAnnotation = "finops.sre.local/cloud-account"

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
		deleteErr := r.deleteCollectorCronJob(ctx, &account)
		changed := apiMeta.SetStatusCondition(&account.Status.Conditions, metav1.Condition{
			Type:               "CredentialsReady",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: account.Generation,
			Reason:             "SecretNotFound",
			Message:            "The referenced credential Secret does not exist in the CloudAccount namespace.",
		})
		collectorReason := "SecretNotFound"
		collectorMessage := "Collector CronJob is not available because credentials are missing."
		if deleteErr != nil {
			collectorReason = "CronJobReconcileFailed"
			collectorMessage = "Unable to reconcile the managed Collector CronJob."
		}
		changed = setCollectorReadyCondition(&account, metav1.ConditionFalse, collectorReason, collectorMessage) || changed
		if changed {
			if updateErr := r.Status().Update(ctx, &account); updateErr != nil {
				return reconcile.Result{}, updateErr
			}
		}
		return reconcile.Result{}, deleteErr
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
	var collectorErr error
	if account.Spec.Collection.Enabled {
		collectorErr = r.reconcileCollectorCronJob(ctx, &account)
		if collectorErr == nil {
			changed = setCollectorReadyCondition(&account, metav1.ConditionTrue, "CronJobReady", "The managed Collector CronJob is reconciled.") || changed
		}
	} else {
		collectorErr = r.deleteCollectorCronJob(ctx, &account)
		if collectorErr == nil {
			changed = setCollectorReadyCondition(&account, metav1.ConditionFalse, "CollectionDisabled", "Collection is disabled for this CloudAccount.") || changed
		}
	}
	if collectorErr != nil {
		changed = setCollectorReadyCondition(&account, metav1.ConditionFalse, "CronJobReconcileFailed", "Unable to reconcile the managed Collector CronJob.") || changed
	}
	if changed {
		if err := r.Status().Update(ctx, &account); err != nil {
			return reconcile.Result{}, err
		}
	}
	if collectorErr != nil {
		return reconcile.Result{}, collectorErr
	}
	if err := r.reconcileCollectionStatus(ctx, &account); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func setCollectorReadyCondition(account *v1alpha1.CloudAccount, status metav1.ConditionStatus, reason, message string) bool {
	return apiMeta.SetStatusCondition(&account.Status.Conditions, metav1.Condition{
		Type:               "CollectorReady",
		Status:             status,
		ObservedGeneration: account.Generation,
		Reason:             reason,
		Message:            message,
	})
}

func (r *CloudAccountReconciler) reconcileCollectionStatus(ctx context.Context, account *v1alpha1.CloudAccount) error {
	var cronJob batchv1.CronJob
	key := types.NamespacedName{Namespace: account.Namespace, Name: collectorCronJobName(account.Name)}
	if err := r.Get(ctx, key, &cronJob); err != nil {
		return client.IgnoreNotFound(err)
	}
	cronJobOwner := metav1.GetControllerOf(&cronJob)
	if cronJobOwner == nil || cronJobOwner.UID != account.UID {
		return nil
	}

	var jobs batchv1.JobList
	jobReader := r.APIReader
	if jobReader == nil {
		jobReader = r.Client
	}
	if err := jobReader.List(ctx, &jobs,
		client.InNamespace(account.Namespace),
		client.MatchingLabels{"app.kubernetes.io/name": "louder-collector", "finops.sre.local/cloud-account": accountLabelValue(account.Name)},
	); err != nil {
		return err
	}
	ownedTerminalJobs := make([]*batchv1.Job, 0, len(jobs.Items))
	for i := range jobs.Items {
		job := &jobs.Items[i]
		owner := metav1.GetControllerOf(job)
		if owner == nil || owner.Kind != "CronJob" || owner.Name != cronJob.Name || owner.UID != cronJob.UID {
			continue
		}
		if terminalJobCondition(job) != nil {
			ownedTerminalJobs = append(ownedTerminalJobs, job)
		}
	}
	if len(ownedTerminalJobs) == 0 {
		return nil
	}
	sort.Slice(ownedTerminalJobs, func(i, j int) bool {
		if ownedTerminalJobs[i].CreationTimestamp.Equal(&ownedTerminalJobs[j].CreationTimestamp) {
			return ownedTerminalJobs[i].Name < ownedTerminalJobs[j].Name
		}
		return ownedTerminalJobs[i].CreationTimestamp.Before(&ownedTerminalJobs[j].CreationTimestamp)
	})

	latestAttempt := ownedTerminalJobs[len(ownedTerminalJobs)-1]
	latestAttemptTime := jobTerminalTime(latestAttempt)
	if account.Status.LastCollectionTime != nil && !latestAttemptTime.After(account.Status.LastCollectionTime.Time) {
		return nil
	}
	previousStatus := account.DeepCopy().Status
	condition := terminalJobCondition(latestAttempt)
	collectionCondition := metav1.Condition{
		Type:               "CollectionReady",
		ObservedGeneration: account.Generation,
	}
	if condition.Type == batchv1.JobComplete {
		collectionCondition.Status = metav1.ConditionTrue
		collectionCondition.Reason = "CollectionSucceeded"
		collectionCondition.Message = "The latest Collector Job completed successfully."
		account.Status.LastSuccessfulCollectionTime = latestAttemptTime.DeepCopy()
	} else {
		collectionCondition.Status = metav1.ConditionFalse
		collectionCondition.Reason = "CollectorJobFailed"
		collectionCondition.Message = "The latest Collector Job failed."
	}
	apiMeta.SetStatusCondition(&account.Status.Conditions, collectionCondition)
	account.Status.LastCollectionTime = latestAttemptTime.DeepCopy()
	if reflect.DeepEqual(previousStatus, account.Status) {
		return nil
	}
	return r.Status().Update(ctx, account)
}

func terminalJobCondition(job *batchv1.Job) *batchv1.JobCondition {
	for i := range job.Status.Conditions {
		condition := &job.Status.Conditions[i]
		if condition.Status == corev1.ConditionTrue && (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) {
			return condition
		}
	}
	return nil
}

func jobTerminalTime(job *batchv1.Job) metav1.Time {
	if job.Status.CompletionTime != nil {
		return *job.Status.CompletionTime
	}
	condition := terminalJobCondition(job)
	if condition != nil && !condition.LastTransitionTime.IsZero() {
		return condition.LastTransitionTime
	}
	return job.CreationTimestamp
}

func (r *CloudAccountReconciler) reconcileCollectorCronJob(ctx context.Context, account *v1alpha1.CloudAccount) error {
	desired := collectorCronJob(account, r.CollectorImage, r.CollectorFixtureMode, r.CollectorStorageSecretName)
	r.Scheme.Default(desired)
	if err := controllerutil.SetControllerReference(account, desired, r.Scheme); err != nil {
		return err
	}
	var current batchv1.CronJob
	err := r.Get(ctx, types.NamespacedName{Namespace: account.Namespace, Name: desired.Name}, &current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	owner := metav1.GetControllerOf(&current)
	if owner == nil || owner.UID != account.UID {
		return fmt.Errorf("CronJob %s/%s is not controlled by CloudAccount %s/%s", current.Namespace, current.Name, account.Namespace, account.Name)
	}
	if !reflect.DeepEqual(current.Spec, desired.Spec) || !reflect.DeepEqual(current.Labels, desired.Labels) || !reflect.DeepEqual(current.OwnerReferences, desired.OwnerReferences) {
		current.Spec = desired.Spec
		current.Labels = desired.Labels
		current.OwnerReferences = desired.OwnerReferences
		return r.Update(ctx, &current)
	}
	return nil
}

func (r *CloudAccountReconciler) deleteCollectorCronJob(ctx context.Context, account *v1alpha1.CloudAccount) error {
	var cronJob batchv1.CronJob
	key := types.NamespacedName{Namespace: account.Namespace, Name: collectorCronJobName(account.Name)}
	if err := r.Get(ctx, key, &cronJob); err != nil {
		return client.IgnoreNotFound(err)
	}
	owner := metav1.GetControllerOf(&cronJob)
	if owner == nil || owner.UID != account.UID {
		return fmt.Errorf("CronJob %s/%s is not controlled by CloudAccount %s/%s", cronJob.Namespace, cronJob.Name, account.Namespace, account.Name)
	}
	return client.IgnoreNotFound(r.Delete(ctx, &cronJob))
}

func collectorCronJob(account *v1alpha1.CloudAccount, image string, fixtureMode bool, storageSecretName string) *batchv1.CronJob {
	labels := map[string]string{"app.kubernetes.io/name": "louder-collector", "finops.sre.local/cloud-account": accountLabelValue(account.Name)}
	backoffLimit := int32(2)
	args := []string{"--provider=" + account.Spec.Provider, "--account-id=" + account.Spec.AccountID}
	if fixtureMode {
		args = append(args, "--fixture=embedded:"+account.Spec.Provider)
	}
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: collectorCronJobName(account.Name), Namespace: account.Namespace, Labels: labels},
		Spec: batchv1.CronJobSpec{
			Schedule:          account.Spec.Collection.Schedule,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: map[string]string{cloudAccountAnnotation: account.Name},
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: &backoffLimit,
					Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{
						RestartPolicy:                corev1.RestartPolicyNever,
						AutomountServiceAccountToken: ptr.To(false),
						SecurityContext:              &corev1.PodSecurityContext{RunAsNonRoot: ptr.To(true), RunAsUser: ptr.To[int64](65532), RunAsGroup: ptr.To[int64](65532), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
						Containers: []corev1.Container{{
							Name: "collector", Image: image, ImagePullPolicy: corev1.PullIfNotPresent,
							Command: []string{"/louder-collector"}, Args: args,
							EnvFrom:         collectorSecretEnvFrom(account, fixtureMode, storageSecretName),
							SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: ptr.To(false), ReadOnlyRootFilesystem: ptr.To(true), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
							Resources:       corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")}},
						}},
					}},
				}},
		},
	}
}

func collectorSecretEnvFrom(account *v1alpha1.CloudAccount, fixtureMode bool, storageSecretName string) []corev1.EnvFromSource {
	secrets := []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: account.Spec.CredentialRef.Name}}}}
	if storageSecretName != "" {
		secrets = append(secrets, corev1.EnvFromSource{SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: storageSecretName},
			Optional:             ptr.To(fixtureMode),
		}})
	}
	return secrets
}

func collectorCronJobName(accountName string) string {
	const suffix = "-collector"
	if len(accountName)+len(suffix) <= 52 {
		return accountName + suffix
	}
	digest := sha256.Sum256([]byte(accountName))
	return accountName[:43] + "-" + hex.EncodeToString(digest[:4])
}

func accountLabelValue(accountName string) string {
	if len(accountName) <= 63 {
		return accountName
	}
	digest := sha256.Sum256([]byte(accountName))
	return accountName[:54] + "-" + hex.EncodeToString(digest[:4])
}

func (r *CloudAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.APIReader = mgr.GetAPIReader()
	r.Scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.CloudAccount{}).
		Owns(&batchv1.CronJob{}).
		WatchesMetadata(&batchv1.Job{}, handler.EnqueueRequestsFromMapFunc(r.requestsForCollectorJob)).
		WatchesMetadata(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.requestsForCredentialSecret)).
		Complete(r)
}

func (r *CloudAccountReconciler) requestsForCollectorJob(_ context.Context, obj client.Object) []reconcile.Request {
	job, ok := obj.(*metav1.PartialObjectMetadata)
	if !ok {
		return nil
	}
	accountName := job.GetAnnotations()[cloudAccountAnnotation]
	if accountName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: job.GetNamespace(), Name: accountName}}}
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
