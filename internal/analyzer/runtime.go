package analyzer

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	ErrBudgetPolicyNotFound           = errors.New("budget policy not found")
	ErrWebhookSecretKeyMissing        = errors.New("Teams webhook Secret is missing TEAMS_WEBHOOK_URL")
	ErrBudgetNotificationStatusUpdate = errors.New("budget notification status could not be updated")
)

type TeamsNotifierFactory func(string) (notifier.Notifier, error)

func RunBudgetPolicy(ctx context.Context, kube client.Client, reader storage.CostReader, namespace, policyName string, newTeamsNotifier TeamsNotifierFactory, now time.Time) ([]BudgetThresholdIntent, error) {
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
	intents, err := EvaluateStoredBudget(ctx, reader, budget, accounts.Items, now)
	if err != nil || len(intents) == 0 {
		return intents, err
	}
	month := now.UTC().Format("2006-01")
	intents = pendingBudgetThresholds(intents, budget.Status, month)
	if len(intents) == 0 {
		return intents, nil
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
	if err := NotifyBudgetIntents(ctx, budget, intents, accounts.Items, policies.Items, resolve); err != nil {
		return intents, err
	}
	notified := append([]int32(nil), intentsToPercentages(intents)...)
	if budget.Status.LastNotifiedMonth == month {
		notified = append(notified, budget.Status.NotifiedThresholds...)
	}
	sort.Slice(notified, func(i, j int) bool { return notified[i] < notified[j] })
	budget.Status.LastNotifiedMonth = month
	budget.Status.NotifiedThresholds = compactThresholds(notified)
	if err := kube.Status().Update(ctx, &budget); err != nil {
		return intents, fmt.Errorf("%w", ErrBudgetNotificationStatusUpdate)
	}
	return intents, nil
}

func pendingBudgetThresholds(intents []BudgetThresholdIntent, status v1alpha1.BudgetPolicyStatus, month string) []BudgetThresholdIntent {
	if status.LastNotifiedMonth != month {
		return intents
	}
	delivered := make(map[int32]struct{}, len(status.NotifiedThresholds))
	for _, threshold := range status.NotifiedThresholds {
		delivered[threshold] = struct{}{}
	}
	pending := make([]BudgetThresholdIntent, 0, len(intents))
	for _, intent := range intents {
		if _, exists := delivered[intent.ThresholdPercent]; !exists {
			pending = append(pending, intent)
		}
	}
	return pending
}

func intentsToPercentages(intents []BudgetThresholdIntent) []int32 {
	thresholds := make([]int32, len(intents))
	for i, intent := range intents {
		thresholds[i] = intent.ThresholdPercent
	}
	return thresholds
}

func compactThresholds(sorted []int32) []int32 {
	if len(sorted) == 0 {
		return sorted
	}
	compacted := sorted[:1]
	for _, threshold := range sorted[1:] {
		if threshold != compacted[len(compacted)-1] {
			compacted = append(compacted, threshold)
		}
	}
	return compacted
}
