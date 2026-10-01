package analyzer

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/normalize"
)

var ErrInvalidBudgetPolicy = errors.New("invalid budget policy")

type BudgetThresholdIntent struct {
	PolicyName       string
	ThresholdPercent int32
	Spent            string
	Budget           int64
	Currency         string
}

func EvaluateBudget(policy v1alpha1.BudgetPolicy, accounts []v1alpha1.CloudAccount, records []normalize.CostRecord, now time.Time) ([]BudgetThresholdIntent, error) {
	if policy.Spec.Amount.Value <= 0 || !validCurrency(policy.Spec.Amount.Currency) {
		return nil, ErrInvalidBudgetPolicy
	}
	seenThresholds := make(map[int32]struct{}, len(policy.Spec.Thresholds))
	for _, threshold := range policy.Spec.Thresholds {
		if threshold <= 0 || threshold > 100 {
			return nil, ErrInvalidBudgetPolicy
		}
		if _, exists := seenThresholds[threshold]; exists {
			return nil, ErrInvalidBudgetPolicy
		}
		seenThresholds[threshold] = struct{}{}
	}
	selected := make(map[accountKey]struct{})
	for _, account := range accounts {
		if account.Spec.Provider == "" || account.Spec.AccountID == "" || !matchesSelector(account.Spec.Metadata, policy.Spec.Selector) {
			continue
		}
		selected[accountKey{provider: account.Spec.Provider, accountID: account.Spec.AccountID}] = struct{}{}
	}
	monthStart := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	total := new(big.Rat)
	precision := 0
	for _, record := range records {
		if _, ok := selected[accountKey{provider: record.Provider, accountID: record.BillingAccountID}]; !ok {
			continue
		}
		if record.UsageStart.Before(monthStart) || !record.UsageStart.Before(now.UTC()) {
			continue
		}
		if record.Currency != policy.Spec.Amount.Currency {
			return nil, fmt.Errorf("cost currency %q does not match budget currency %q", record.Currency, policy.Spec.Amount.Currency)
		}
		amount, ok := new(big.Rat).SetString(record.Amount)
		if !ok {
			return nil, normalize.ErrInvalidCostRecord
		}
		total.Add(total, amount)
		if digits := decimalPlaces(record.Amount); digits > precision {
			precision = digits
		}
	}
	spent := total.FloatString(precision)
	intents := make([]BudgetThresholdIntent, 0, len(policy.Spec.Thresholds))
	for _, threshold := range policy.Spec.Thresholds {
		if reachedThreshold(total, policy.Spec.Amount.Value, threshold) {
			intents = append(intents, BudgetThresholdIntent{PolicyName: policy.Name, ThresholdPercent: threshold, Spent: spent, Budget: policy.Spec.Amount.Value, Currency: policy.Spec.Amount.Currency})
		}
	}
	sort.Slice(intents, func(i, j int) bool { return intents[i].ThresholdPercent < intents[j].ThresholdPercent })
	return intents, nil
}

func validCurrency(currency string) bool {
	if len(currency) != 3 {
		return false
	}
	for _, character := range currency {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}

type accountKey struct {
	provider  string
	accountID string
}

func matchesSelector(metadata, selector map[string]string) bool {
	for key, value := range selector {
		if metadata[key] != value {
			return false
		}
	}
	return true
}

func reachedThreshold(spent *big.Rat, budget int64, threshold int32) bool {
	left := new(big.Rat).Mul(spent, big.NewRat(100, 1))
	right := new(big.Rat).Mul(big.NewRat(budget, 1), big.NewRat(int64(threshold), 1))
	return left.Cmp(right) >= 0
}

func decimalPlaces(value string) int {
	if point := strings.IndexByte(value, '.'); point >= 0 {
		return len(value) - point - 1
	}
	return 0
}
