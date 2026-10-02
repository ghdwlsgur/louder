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
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type BudgetPolicyReconciler struct {
	client.Client
	AnalyzerImage              string
	StorageSecretName          string
	AnalyzerServiceAccountName string
	Scheme                     *runtime.Scheme
}

func (r *BudgetPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var policy v1alpha1.BudgetPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if policy.Spec.Schedule == "" {
		return ctrl.Result{}, r.deleteAnalyzerCronJob(ctx, &policy)
	}
	if r.AnalyzerImage == "" || r.StorageSecretName == "" || r.AnalyzerServiceAccountName == "" {
		return ctrl.Result{}, fmt.Errorf("Analyzer image, storage Secret, and ServiceAccount are required for scheduled BudgetPolicy %s/%s", policy.Namespace, policy.Name)
	}
	return ctrl.Result{}, r.reconcileAnalyzerCronJob(ctx, &policy)
}

func (r *BudgetPolicyReconciler) reconcileAnalyzerCronJob(ctx context.Context, policy *v1alpha1.BudgetPolicy) error {
	desired := analyzerCronJob(policy, r.AnalyzerImage, r.StorageSecretName, r.AnalyzerServiceAccountName)
	if err := controllerutil.SetControllerReference(policy, desired, r.Scheme); err != nil {
		return err
	}
	var current batchv1.CronJob
	err := r.Get(ctx, types.NamespacedName{Namespace: policy.Namespace, Name: desired.Name}, &current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	owner := metav1.GetControllerOf(&current)
	if owner == nil || owner.UID != policy.UID {
		return fmt.Errorf("CronJob %s/%s is not controlled by BudgetPolicy %s/%s", current.Namespace, current.Name, policy.Namespace, policy.Name)
	}
	specChanged := !apiequality.Semantic.DeepDerivative(desired.Spec, current.Spec)
	if specChanged || !reflect.DeepEqual(current.Labels, desired.Labels) || !reflect.DeepEqual(current.OwnerReferences, desired.OwnerReferences) {
		if specChanged {
			current.Spec = desired.Spec
		}
		current.Labels = desired.Labels
		current.OwnerReferences = desired.OwnerReferences
		return r.Update(ctx, &current)
	}
	return nil
}

func (r *BudgetPolicyReconciler) deleteAnalyzerCronJob(ctx context.Context, policy *v1alpha1.BudgetPolicy) error {
	var cronJob batchv1.CronJob
	key := types.NamespacedName{Namespace: policy.Namespace, Name: analyzerCronJobName(policy.Name)}
	if err := r.Get(ctx, key, &cronJob); err != nil {
		return client.IgnoreNotFound(err)
	}
	owner := metav1.GetControllerOf(&cronJob)
	if owner == nil || owner.UID != policy.UID {
		return fmt.Errorf("CronJob %s/%s is not controlled by BudgetPolicy %s/%s", cronJob.Namespace, cronJob.Name, policy.Namespace, policy.Name)
	}
	return client.IgnoreNotFound(r.Delete(ctx, &cronJob))
}

func analyzerCronJob(policy *v1alpha1.BudgetPolicy, image, storageSecretName, serviceAccountName string) *batchv1.CronJob {
	labels := map[string]string{"app.kubernetes.io/name": "louder-analyzer", "finops.sre.local/budget-policy": budgetPolicyLabelValue(policy.Name)}
	backoffLimit := int32(1)
	historyLimit := int32(1)
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: analyzerCronJobName(policy.Name), Namespace: policy.Namespace, Labels: labels},
		Spec: batchv1.CronJobSpec{
			Schedule:                   policy.Spec.Schedule,
			TimeZone:                   ptr.To("Etc/UTC"),
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			SuccessfulJobsHistoryLimit: &historyLimit,
			FailedJobsHistoryLimit:     &historyLimit,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: batchv1.JobSpec{
					BackoffLimit: &backoffLimit,
					Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{
						ServiceAccountName:           serviceAccountName,
						AutomountServiceAccountToken: ptr.To(true),
						RestartPolicy:                corev1.RestartPolicyNever,
						SecurityContext:              &corev1.PodSecurityContext{RunAsNonRoot: ptr.To(true), RunAsUser: ptr.To[int64](65532), RunAsGroup: ptr.To[int64](65532), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
						Containers: []corev1.Container{{
							Name: "analyzer", Image: image, ImagePullPolicy: corev1.PullIfNotPresent,
							Command: []string{"/louder-analyzer"}, Args: []string{"--namespace=" + policy.Namespace, "--budget-policy=" + policy.Name},
							EnvFrom:         []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: storageSecretName}}}},
							SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: ptr.To(false), ReadOnlyRootFilesystem: ptr.To(true), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
							Resources:       corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")}},
						}},
					}},
				},
			},
		},
	}
}

func analyzerCronJobName(policyName string) string {
	const suffix = "-analyzer"
	if len(policyName)+len(suffix) <= 52 {
		return policyName + suffix
	}
	digest := sha256.Sum256([]byte(policyName))
	return policyName[:43] + "-" + hex.EncodeToString(digest[:4])
}

func budgetPolicyLabelValue(policyName string) string {
	if len(policyName) <= 63 {
		return policyName
	}
	digest := sha256.Sum256([]byte(policyName))
	return policyName[:54] + "-" + hex.EncodeToString(digest[:4])
}

func (r *BudgetPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.BudgetPolicy{}).
		Owns(&batchv1.CronJob{}).
		Complete(r)
}
