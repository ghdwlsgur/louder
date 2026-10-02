package analyzer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/storage"
)

var ErrBudgetForecastCurrencyMismatch = errors.New("budget forecast cost currency does not match policy currency")

type BudgetForecastIntent struct {
	PolicyName     string
	Month          string
	Spent          string
	ProjectedSpend string
	Budget         int64
	Currency       string
}

func EvaluateBudgetForecast(policy v1alpha1.BudgetPolicy, accounts []v1alpha1.CloudAccount, records []normalize.CostRecord, now time.Time) (*BudgetForecastIntent, error) {
	if policy.Spec.Forecast == nil || !policy.Spec.Forecast.Enabled {
		return nil, nil
	}
	if err := validateBudgetPolicy(policy); err != nil {
		return nil, err
	}
	selected := make(map[accountKey]struct{})
	for _, account := range accounts {
		if account.Spec.Provider == "" || account.Spec.AccountID == "" || !matchesSelector(account.Spec.Metadata, policy.Spec.Selector) {
			continue
		}
		selected[accountKey{provider: account.Spec.Provider, accountID: account.Spec.AccountID}] = struct{}{}
	}
	if len(selected) == 0 {
		return nil, nil
	}

	utcNow := now.UTC()
	monthStart := time.Date(utcNow.Year(), utcNow.Month(), 1, 0, 0, 0, 0, time.UTC)
	totals := new(big.Rat)
	precision := 0
	var costBasis provider.CostBasis
	for _, record := range records {
		if _, ok := selected[accountKey{provider: record.Provider, accountID: record.BillingAccountID}]; !ok {
			continue
		}
		start := record.UsageStart.UTC()
		if start.Before(monthStart) || !start.Before(utcNow) {
			continue
		}
		if record.CostBasis == provider.CostBasisNCPMonthly || isMonthlyInterval(start, record.UsageEnd.UTC()) {
			continue
		}
		if record.Currency != policy.Spec.Amount.Currency {
			return nil, fmt.Errorf("%w: got %q, want %q", ErrBudgetForecastCurrencyMismatch, record.Currency, policy.Spec.Amount.Currency)
		}
		if costBasis != "" && costBasis != record.CostBasis {
			return nil, fmt.Errorf("%w: %q and %q", ErrMixedCostBasis, costBasis, record.CostBasis)
		}
		costBasis = record.CostBasis
		amount, ok := new(big.Rat).SetString(record.Amount)
		if !ok {
			return nil, normalize.ErrInvalidCostRecord
		}
		totals.Add(totals, amount)
		if digits := decimalPlaces(record.Amount); digits > precision {
			precision = digits
		}
	}
	if totals.Sign() <= 0 {
		return nil, nil
	}

	daysInMonth := time.Date(utcNow.Year(), utcNow.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	projected := new(big.Rat).Mul(totals, big.NewRat(int64(daysInMonth), int64(utcNow.Day())))
	if projected.Cmp(big.NewRat(policy.Spec.Amount.Value, 1)) <= 0 {
		return nil, nil
	}
	if precision < 2 {
		precision = 2
	}
	return &BudgetForecastIntent{
		PolicyName: policy.Name, Month: utcNow.Format("2006-01"),
		Spent: totals.FloatString(precision), ProjectedSpend: projected.FloatString(precision),
		Budget: policy.Spec.Amount.Value, Currency: policy.Spec.Amount.Currency,
	}, nil
}

func EvaluateStoredBudgetForecast(ctx context.Context, reader storage.CostReader, policy v1alpha1.BudgetPolicy, accounts []v1alpha1.CloudAccount, now time.Time) (*BudgetForecastIntent, error) {
	if policy.Spec.Forecast == nil || !policy.Spec.Forecast.Enabled {
		return nil, nil
	}
	if err := validateBudgetPolicy(policy); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	selectedAccounts := make([]v1alpha1.CloudAccount, 0, len(accounts))
	scopes := make([]storage.AccountScope, 0, len(accounts))
	for _, account := range accounts {
		if account.Spec.Provider == "" || account.Spec.AccountID == "" || !matchesSelector(account.Spec.Metadata, policy.Spec.Selector) {
			continue
		}
		selectedAccounts = append(selectedAccounts, account)
		scopes = append(scopes, storage.AccountScope{Provider: account.Spec.Provider, BillingAccountID: account.Spec.AccountID})
	}
	if len(scopes) == 0 {
		return nil, nil
	}
	if reader == nil {
		return nil, fmt.Errorf("cost reader is required")
	}
	utcNow := now.UTC()
	monthStart := time.Date(utcNow.Year(), utcNow.Month(), 1, 0, 0, 0, 0, time.UTC)
	if !monthStart.Before(utcNow) {
		return nil, nil
	}
	records, err := reader.ReadCosts(ctx, scopes, monthStart, utcNow)
	if err != nil {
		return nil, err
	}
	return EvaluateBudgetForecast(policy, selectedAccounts, records, utcNow)
}
