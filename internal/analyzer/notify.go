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
		notification := notifier.Notification{
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
		if err := delivery.Send(ctx, notification); err != nil {
			return intents, err
		}
	}
	return intents, nil
}
