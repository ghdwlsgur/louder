package analyzer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/storage"
)

var (
	ErrInvalidDailyAnomalyThreshold = errors.New("invalid daily anomaly threshold")
	ErrDailyAnomalyCurrencyMismatch = errors.New("daily anomaly cost currency does not match policy currency")
	ErrDailyAnomalySourceStale      = errors.New("daily anomaly source has no successful collection covering the analysis date")
)

type DailyCostAnomalyIntent struct {
	PolicyName       string
	Provider         string
	BillingAccountID string
	Date             time.Time
	Today            string
	BaselineAverage  string
	Increase         string
	Currency         string
}

func EvaluateDailyCostAnomalies(policyName string, selector map[string]string, accounts []v1alpha1.CloudAccount, records []normalize.CostRecord, targetDay time.Time, threshold v1alpha1.BudgetAmount) ([]DailyCostAnomalyIntent, error) {
	if threshold.Value <= 0 || !validCurrency(threshold.Currency) {
		return nil, ErrInvalidDailyAnomalyThreshold
	}
	day := targetDay.UTC()
	if day.Hour() != 0 || day.Minute() != 0 || day.Second() != 0 || day.Nanosecond() != 0 {
		return nil, ErrInvalidDailyAnomalyThreshold
	}
	selected := make(map[accountKey]struct{})
	for _, account := range accounts {
		if !provider.ProvidesDailyCostRecords(account.Spec.Provider) || account.Spec.AccountID == "" || !matchesSelector(account.Spec.Metadata, selector) {
			continue
		}
		selected[accountKey{provider: account.Spec.Provider, accountID: account.Spec.AccountID}] = struct{}{}
	}
	if len(selected) == 0 {
		return []DailyCostAnomalyIntent{}, nil
	}

	windowStart := day.AddDate(0, 0, -7)
	windowEnd := day.AddDate(0, 0, 1)
	totals := make(map[accountKey]map[time.Time]*big.Rat)
	precisions := make(map[accountKey]int)
	costBases := make(map[accountKey]provider.CostBasis)
	for _, record := range records {
		key := accountKey{provider: record.Provider, accountID: record.BillingAccountID}
		if _, ok := selected[key]; !ok {
			continue
		}
		start := record.UsageStart.UTC()
		if start.Before(windowStart) || !start.Before(windowEnd) {
			continue
		}
		end := record.UsageEnd.UTC()
		if isMonthlyInterval(start, end) {
			continue
		}
		if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 || start.Nanosecond() != 0 || !end.Equal(start.AddDate(0, 0, 1)) {
			return nil, normalize.ErrInvalidCostRecord
		}
		if record.Currency != threshold.Currency {
			return nil, fmt.Errorf("%w: got %q, want %q", ErrDailyAnomalyCurrencyMismatch, record.Currency, threshold.Currency)
		}
		if costBasis, ok := costBases[key]; ok && costBasis != record.CostBasis {
			return nil, fmt.Errorf("%w: %q and %q", ErrMixedCostBasis, costBasis, record.CostBasis)
		}
		costBases[key] = record.CostBasis
		amount, ok := new(big.Rat).SetString(record.Amount)
		if !ok {
			return nil, normalize.ErrInvalidCostRecord
		}
		daily, exists := totals[key]
		if !exists {
			daily = make(map[time.Time]*big.Rat, 8)
			totals[key] = daily
		}
		date := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		if daily[date] == nil {
			daily[date] = new(big.Rat)
		}
		daily[date].Add(daily[date], amount)
		if digits := decimalPlaces(record.Amount); digits > precisions[key] {
			precisions[key] = digits
		}
	}

	intents := make([]DailyCostAnomalyIntent, 0)
	for key, daily := range totals {
		today := daily[day]
		if today == nil {
			today = new(big.Rat)
		}
		baselineTotal := new(big.Rat)
		for offset := -7; offset < 0; offset++ {
			if value := daily[day.AddDate(0, 0, offset)]; value != nil {
				baselineTotal.Add(baselineTotal, value)
			}
		}
		baselineAverage := new(big.Rat).Quo(baselineTotal, big.NewRat(7, 1))
		increase := new(big.Rat).Sub(new(big.Rat).Set(today), baselineAverage)
		if !exceedsAnomalyThreshold(today, baselineAverage, increase, threshold.Value) {
			continue
		}
		precision := precisions[key]
		intents = append(intents, DailyCostAnomalyIntent{
			PolicyName: policyName, Provider: key.provider, BillingAccountID: key.accountID, Date: day,
			Today: today.FloatString(precision), BaselineAverage: baselineAverage.FloatString(precision),
			Increase: increase.FloatString(precision), Currency: threshold.Currency,
		})
	}
	sort.Slice(intents, func(i, j int) bool {
		if intents[i].Provider != intents[j].Provider {
			return intents[i].Provider < intents[j].Provider
		}
		return intents[i].BillingAccountID < intents[j].BillingAccountID
	})
	return intents, nil
}

func EvaluateStoredDailyCostAnomalies(ctx context.Context, reader storage.CostReader, policy v1alpha1.BudgetPolicy, accounts []v1alpha1.CloudAccount, now time.Time) ([]DailyCostAnomalyIntent, error) {
	if policy.Spec.DailyAnomaly == nil {
		return []DailyCostAnomalyIntent{}, nil
	}
	if err := validateBudgetPolicy(policy); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	utcNow := now.UTC()
	targetDay := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day()-1, 0, 0, 0, 0, time.UTC)
	windowStart := targetDay.AddDate(0, 0, -7)
	windowEnd := targetDay.AddDate(0, 0, 1)
	selectedAccounts := make([]v1alpha1.CloudAccount, 0, len(accounts))
	scopes := make([]storage.AccountScope, 0, len(accounts))
	for _, account := range accounts {
		if !account.Spec.Collection.Enabled || !provider.ProvidesDailyCostRecords(account.Spec.Provider) || account.Spec.AccountID == "" || !matchesSelector(account.Spec.Metadata, policy.Spec.Selector) {
			continue
		}
		if account.Status.LastSuccessfulCollectionTime == nil || account.Status.LastSuccessfulCollectionTime.Time.Before(windowEnd) {
			return nil, ErrDailyAnomalySourceStale
		}
		selectedAccounts = append(selectedAccounts, account)
		scopes = append(scopes, storage.AccountScope{Provider: account.Spec.Provider, BillingAccountID: account.Spec.AccountID})
	}
	if len(scopes) == 0 {
		return []DailyCostAnomalyIntent{}, nil
	}
	if reader == nil {
		return nil, fmt.Errorf("cost reader is required")
	}
	records, err := reader.ReadCosts(ctx, scopes, windowStart, windowEnd)
	if err != nil {
		return nil, err
	}
	return EvaluateDailyCostAnomalies(policy.Name, policy.Spec.Selector, selectedAccounts, records, targetDay, v1alpha1.BudgetAmount{
		Value: policy.Spec.DailyAnomaly.AbsoluteIncreaseThreshold, Currency: policy.Spec.Amount.Currency,
	})
}

func isMonthlyInterval(start, end time.Time) bool {
	return start.Day() == 1 && provider.IsUTCMidnight(start) && end.Equal(start.AddDate(0, 1, 0))
}

func exceedsAnomalyThreshold(today, baseline, increase *big.Rat, absoluteThreshold int64) bool {
	relativeLimit := new(big.Rat).Mul(baseline, big.NewRat(3, 2))
	return today.Cmp(relativeLimit) > 0 && increase.Cmp(big.NewRat(absoluteThreshold, 1)) > 0
}
