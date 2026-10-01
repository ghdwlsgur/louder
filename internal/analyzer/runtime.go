package analyzer

import (
	"context"
	"errors"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/notifier"
	"github.com/ghdwlsgur/louder/internal/storage"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrBudgetPolicyNotFound    = errors.New("budget policy not found")
	ErrWebhookSecretKeyMissing = errors.New("Teams webhook Secret is missing TEAMS_WEBHOOK_URL")
)

type TeamsNotifierFactory func(string) (notifier.Notifier, error)

func RunBudgetPolicy(ctx context.Context, kube client.Reader, reader storage.CostReader, namespace, policyName string, newTeamsNotifier TeamsNotifierFactory, now time.Time) ([]BudgetThresholdIntent, error) {
	var budget v1alpha1.BudgetPolicy
	if err := kube.Get(ctx, types.NamespacedName{Namespace: namespace, Name: policyName}, &budget); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrBudgetPolicyNotFound
		}
		return nil, err
	}
	var accounts v1alpha1.CloudAccountList
	if err := kube.List(ctx, &accounts, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	var policies v1alpha1.NotificationPolicyList
	if err := kube.List(ctx, &policies, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	resolve := func(ctx context.Context, policy v1alpha1.NotificationPolicy) (notifier.Notifier, error) {
		if newTeamsNotifier == nil {
			return nil, ErrNotifierRequired
		}
		var secret corev1.Secret
		if err := kube.Get(ctx, types.NamespacedName{Namespace: policy.Namespace, Name: policy.Spec.CredentialRef.Name}, &secret); err != nil {
			return nil, err
		}
		endpoint := secret.Data["TEAMS_WEBHOOK_URL"]
		if len(endpoint) == 0 {
			return nil, ErrWebhookSecretKeyMissing
		}
		return newTeamsNotifier(string(endpoint))
	}
	return EvaluateAndNotifyBudgetPolicies(ctx, reader, budget, accounts.Items, policies.Items, resolve, now)
}
