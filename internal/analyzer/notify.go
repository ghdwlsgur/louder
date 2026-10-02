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

var ErrNotifierRequired = errors.New("notifier is required for budget notification")
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

func NotifyBudgetForecastIntent(ctx context.Context, budget v1alpha1.BudgetPolicy, intent BudgetForecastIntent, accounts []v1alpha1.CloudAccount, policies []v1alpha1.NotificationPolicy, resolve NotificationResolver) error {
	if resolve == nil {
		return ErrNotifierRequired
	}
	selectedAccounts := make([]v1alpha1.CloudAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.Spec.Provider != "" && account.Spec.AccountID != "" && matchesSelector(account.Spec.Metadata, budget.Spec.Selector) {
			selectedAccounts = append(selectedAccounts, account)
		}
	}
	selected := SelectNotificationPolicies("BudgetForecast", policies, selectedAccounts)
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
	notification := budgetForecastNotification(intent)
	for _, delivery := range deliveries {
		if err := delivery.Send(ctx, notification); err != nil {
			return err
		}
	}
	return nil
}

func NotifyDailyAnomalyIntents(ctx context.Context, budget v1alpha1.BudgetPolicy, intents []DailyCostAnomalyIntent, accounts []v1alpha1.CloudAccount, policies []v1alpha1.NotificationPolicy, resolve NotificationResolver) error {
	_, err := NotifyDailyAnomalyIntentsWithReceipts(ctx, budget, intents, accounts, policies, resolve, nil)
	return err
}

func NotifyDailyAnomalyIntentsWithReceipts(ctx context.Context, budget v1alpha1.BudgetPolicy, intents []DailyCostAnomalyIntent, accounts []v1alpha1.CloudAccount, policies []v1alpha1.NotificationPolicy, resolve NotificationResolver, previous []v1alpha1.DailyAnomalyNotificationReceipt) ([]v1alpha1.DailyAnomalyNotificationReceipt, error) {
	if len(intents) == 0 {
		return nil, nil
	}
	if resolve == nil {
		return nil, ErrNotifierRequired
	}
	accountsByScope := make(map[accountKey]v1alpha1.CloudAccount, len(accounts))
	for _, account := range accounts {
		if account.Spec.Provider != "" && account.Spec.AccountID != "" && matchesSelector(account.Spec.Metadata, budget.Spec.Selector) {
			accountsByScope[accountKey{provider: account.Spec.Provider, accountID: account.Spec.AccountID}] = account
		}
	}
	type destination struct {
		policy   v1alpha1.NotificationPolicy
		delivery notifier.Notifier
		intents  []DailyCostAnomalyIntent
	}
	destinations := make([]destination, 0, len(policies))
	eligibleDestination := false
	for _, policy := range SelectNotificationPolicies("CostAnomaly", policies, accounts) {
		matched := make([]DailyCostAnomalyIntent, 0)
		for _, intent := range intents {
			account, exists := accountsByScope[accountKey{provider: intent.Provider, accountID: intent.BillingAccountID}]
			receipt := dailyAnomalyReceipt(intent, policy.Name)
			if exists && matchesSelector(account.Spec.Metadata, policy.Spec.Selector) {
				eligibleDestination = true
				if hasDailyAnomalyReceipt(previous, receipt) {
					continue
				}
				matched = append(matched, intent)
			}
		}
		if len(matched) == 0 {
			continue
		}
		delivery, err := resolve(ctx, policy)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrNotificationPolicyResolution, policy.Name)
		}
		if delivery == nil {
			return nil, ErrNotifierRequired
		}
		destinations = append(destinations, destination{policy: policy, delivery: delivery, intents: matched})
	}
	if len(destinations) == 0 {
		if eligibleDestination {
			return nil, nil
		}
		return nil, ErrNotifierRequired
	}
	delivered := make([]v1alpha1.DailyAnomalyNotificationReceipt, 0)
	for _, destination := range destinations {
		for _, intent := range destination.intents {
			if err := destination.delivery.Send(ctx, dailyAnomalyNotification(intent)); err != nil {
				return delivered, err
			}
			delivered = append(delivered, dailyAnomalyReceipt(intent, destination.policy.Name))
		}
	}
	return delivered, nil
}

func dailyAnomalyReceipt(intent DailyCostAnomalyIntent, policyName string) v1alpha1.DailyAnomalyNotificationReceipt {
	return v1alpha1.DailyAnomalyNotificationReceipt{
		Date: intent.Date.UTC().Format("2006-01-02"), Provider: intent.Provider,
		BillingAccountID: intent.BillingAccountID, NotificationPolicyName: policyName,
	}
}

func hasDailyAnomalyReceipt(receipts []v1alpha1.DailyAnomalyNotificationReceipt, target v1alpha1.DailyAnomalyNotificationReceipt) bool {
	for _, receipt := range receipts {
		if receipt == target {
			return true
		}
	}
	return false
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

func budgetForecastNotification(intent BudgetForecastIntent) notifier.Notification {
	return notifier.Notification{
		Type: "BudgetForecast", Severity: "Warning", Title: "Monthly budget may be exceeded",
		Summary: fmt.Sprintf("Budget policy %s projects %s %s for %s against a budget of %d %s.", intent.PolicyName, intent.ProjectedSpend, intent.Currency, intent.Month, intent.Budget, intent.Currency),
		Details: map[string]string{
			"Policy": intent.PolicyName, "Month": intent.Month, "Spent": intent.Spent,
			"ProjectedSpend": intent.ProjectedSpend, "Budget": strconv.FormatInt(intent.Budget, 10), "Currency": intent.Currency,
		},
	}
}

func dailyAnomalyNotification(intent DailyCostAnomalyIntent) notifier.Notification {
	return notifier.Notification{
		Type: "CostAnomaly", Severity: "Warning", Title: "Daily cost increase detected",
		Summary: fmt.Sprintf("%s cost on %s was %s %s above the seven-day average.", intent.Provider, intent.Date.Format("2006-01-02"), intent.Increase, intent.Currency),
		Details: map[string]string{
			"Policy": intent.PolicyName, "Provider": intent.Provider, "BillingAccountID": intent.BillingAccountID,
			"Date": intent.Date.Format("2006-01-02"), "Today": intent.Today,
			"BaselineAverage": intent.BaselineAverage, "Increase": intent.Increase, "Currency": intent.Currency,
		},
	}
}
