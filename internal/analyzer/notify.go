package analyzer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/notifier"
	"github.com/ghdwlsgur/louder/internal/storage"
)

var ErrNotifierRequired = errors.New("notifier is required for reached budget thresholds")
var ErrNotificationPolicyResolution = errors.New("notification policy could not be resolved")

type NotificationResolver func(context.Context, v1alpha1.NotificationPolicy) (notifier.Notifier, error)

func EvaluateAndNotifyBudget(ctx context.Context, reader storage.CostReader, delivery notifier.Notifier, policy v1alpha1.BudgetPolicy, accounts []v1alpha1.CloudAccount, now time.Time) ([]BudgetThresholdIntent, error) {
	intents, err := EvaluateStoredBudget(ctx, reader, policy, accounts, now)
	if err != nil {
		return nil, err
	}
	if len(intents) == 0 {
		return intents, nil
	}
	if delivery == nil {
		return nil, ErrNotifierRequired
	}
	for _, intent := range intents {
		if err := delivery.Send(ctx, budgetThresholdNotification(intent)); err != nil {
			return intents, err
		}
	}
	return intents, nil
}

func EvaluateAndNotifyBudgetPolicies(ctx context.Context, reader storage.CostReader, budget v1alpha1.BudgetPolicy, accounts []v1alpha1.CloudAccount, policies []v1alpha1.NotificationPolicy, resolve NotificationResolver, now time.Time) ([]BudgetThresholdIntent, error) {
	intents, err := EvaluateStoredBudget(ctx, reader, budget, accounts, now)
	if err != nil || len(intents) == 0 {
		return intents, err
	}
	if err := NotifyBudgetIntents(ctx, budget, intents, accounts, policies, resolve); err != nil {
		return intents, err
	}
	return intents, nil
}

func NotifyBudgetIntents(ctx context.Context, budget v1alpha1.BudgetPolicy, intents []BudgetThresholdIntent, accounts []v1alpha1.CloudAccount, policies []v1alpha1.NotificationPolicy, resolve NotificationResolver) error {
	if len(intents) == 0 {
		return nil
	}
	if resolve == nil {
		return ErrNotifierRequired
	}

	budgetAccounts := make([]v1alpha1.CloudAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.Spec.Provider != "" && account.Spec.AccountID != "" && matchesSelector(account.Spec.Metadata, budget.Spec.Selector) {
			budgetAccounts = append(budgetAccounts, account)
		}
	}
	selected := SelectNotificationPolicies("BudgetThreshold", policies, budgetAccounts)
	if len(selected) == 0 {
		return ErrNotifierRequired
	}

	deliveries := make([]notifier.Notifier, 0, len(selected))
	for _, policy := range selected {
		delivery, err := resolve(ctx, policy)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrNotificationPolicyResolution, policy.Name)
		}
		if delivery == nil {
			return ErrNotifierRequired
		}
		deliveries = append(deliveries, delivery)
	}
	for _, delivery := range deliveries {
		for _, intent := range intents {
			if err := delivery.Send(ctx, budgetThresholdNotification(intent)); err != nil {
				return err
			}
		}
	}
	return nil
}

func budgetThresholdNotification(intent BudgetThresholdIntent) notifier.Notification {
	return notifier.Notification{
		Type:     "BudgetThreshold",
		Severity: "Warning",
		Title:    "Monthly budget threshold reached",
		Summary:  fmt.Sprintf("Budget policy %s reached %d%% of its monthly limit.", intent.PolicyName, intent.ThresholdPercent),
		Details: map[string]string{
			"Budget":    strconv.FormatInt(intent.Budget, 10),
			"Currency":  intent.Currency,
			"Spend":     intent.Spent,
			"Threshold": strconv.Itoa(int(intent.ThresholdPercent)) + "%",
		},
	}
}
