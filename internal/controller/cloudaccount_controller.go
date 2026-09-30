package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"

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
	APIReader            client.Reader
	CollectorImage       string
	CollectorFixtureMode bool
	Scheme               *runtime.Scheme
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
		if err := r.deleteCollectorCronJob(ctx, &account); err != nil {
			return reconcile.Result{}, err
		}
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
		if err := r.Status().Update(ctx, &account); err != nil {
			return reconcile.Result{}, err
		}
	}
	if account.Spec.Collection.Enabled {
		if err := r.reconcileCollectorCronJob(ctx, &account); err != nil {
			return reconcile.Result{}, err
		}
	} else if err := r.deleteCollectorCronJob(ctx, &account); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *CloudAccountReconciler) reconcileCollectorCronJob(ctx context.Context, account *v1alpha1.CloudAccount) error {
	desired := collectorCronJob(account, r.CollectorImage, r.CollectorFixtureMode)
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

func collectorCronJob(account *v1alpha1.CloudAccount, image string, fixtureMode bool) *batchv1.CronJob {
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
			JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{
				BackoffLimit: &backoffLimit,
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              &corev1.PodSecurityContext{RunAsNonRoot: ptr.To(true), RunAsUser: ptr.To[int64](65532), RunAsGroup: ptr.To[int64](65532), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
					Containers: []corev1.Container{{
						Name: "collector", Image: image, ImagePullPolicy: corev1.PullIfNotPresent,
						Command: []string{"/louder-collector"}, Args: args,
						EnvFrom:         []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: account.Spec.CredentialRef.Name}}}},
						SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: ptr.To(false), ReadOnlyRootFilesystem: ptr.To(true), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
						Resources:       corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")}},
					}},
				}},
			}},
		},
	}
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
