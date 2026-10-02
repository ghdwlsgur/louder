package analyzer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/provider"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEvaluateBudgetForecastProjectsMonthToDateDailySpend(t *testing.T) {
	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	policy, accounts := forecastPolicyAndAccounts(250)
	var records []normalize.CostRecord
	for day := 1; day <= 16; day++ {
		start := time.Date(2026, time.October, day, 0, 0, 0, 0, time.UTC)
		records = append(records, normalize.CostRecord{
			Provider: "aws", BillingAccountID: "123", CostBasis: provider.CostBasisNet,
			Amount: "10", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1),
		})
	}

	intent, err := EvaluateBudgetForecast(policy, accounts, records, now)
	if err != nil {
		t.Fatalf("EvaluateBudgetForecast() error = %v", err)
	}
	if intent == nil {
		t.Fatal("EvaluateBudgetForecast() = nil, want a forecast above the configured budget")
	}
	if intent.Month != "2026-10" || intent.Spent != "160.00" || intent.ProjectedSpend != "310.00" || intent.Budget != 250 || intent.Currency != "USD" {
		t.Errorf("forecast intent = %#v, want month 2026-10, spend 160.00, projected 310.00, budget 250 USD", intent)
	}
}

func TestEvaluateBudgetForecastDoesNotAlertAtOrBelowBudget(t *testing.T) {
	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	var records []normalize.CostRecord
	for day := 1; day <= 16; day++ {
		start := time.Date(2026, time.October, day, 0, 0, 0, 0, time.UTC)
		records = append(records, normalize.CostRecord{
			Provider: "aws", BillingAccountID: "123", Amount: "10", Currency: "USD",
			UsageStart: start, UsageEnd: start.AddDate(0, 0, 1),
		})
	}

	for _, budget := range []int64{310, 311} {
		t.Run(fmt.Sprintf("budget-%d", budget), func(t *testing.T) {
			policy, accounts := forecastPolicyAndAccounts(budget)
			intent, err := EvaluateBudgetForecast(policy, accounts, records, now)
			if err != nil {
				t.Fatalf("EvaluateBudgetForecast() error = %v", err)
			}
			if intent != nil {
				t.Errorf("EvaluateBudgetForecast() = %#v, want no alert at or below budget %d", intent, budget)
			}
		})
	}
}

func TestEvaluateBudgetForecastUsesLeapYearMonthLength(t *testing.T) {
	now := time.Date(2028, time.February, 14, 12, 0, 0, 0, time.UTC)
	policy, accounts := forecastPolicyAndAccounts(280)
	var records []normalize.CostRecord
	for day := 1; day <= 14; day++ {
		start := time.Date(2028, time.February, day, 0, 0, 0, 0, time.UTC)
		records = append(records, normalize.CostRecord{
			Provider: "aws", BillingAccountID: "123", Amount: "10", Currency: "USD",
			UsageStart: start, UsageEnd: start.AddDate(0, 0, 1),
		})
	}

	intent, err := EvaluateBudgetForecast(policy, accounts, records, now)
	if err != nil {
		t.Fatalf("EvaluateBudgetForecast() error = %v", err)
	}
	if intent == nil || intent.ProjectedSpend != "290.00" {
		t.Fatalf("forecast intent = %#v, want leap-February projection 290.00", intent)
	}
}

func TestEvaluateBudgetForecastExcludesNCPMonthlyInvoices(t *testing.T) {
	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	policy, accounts := forecastPolicyAndAccounts(100)
	policy.Spec.Amount.Currency = "KRW"
	accounts[0].Spec.Provider = "ncp"
	accounts[0].Spec.AccountID = "2760000"
	monthStart := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	records := []normalize.CostRecord{{
		Provider: "ncp", BillingAccountID: "2760000", CostBasis: provider.CostBasisNCPMonthly,
		Amount: "10000", Currency: "KRW", UsageStart: monthStart, UsageEnd: monthStart.AddDate(0, 1, 0),
	}}

	intent, err := EvaluateBudgetForecast(policy, accounts, records, now)
	if err != nil {
		t.Fatalf("EvaluateBudgetForecast() error = %v", err)
	}
	if intent != nil {
		t.Errorf("EvaluateBudgetForecast() = %#v, want no daily forecast for monthly NCP invoice records", intent)
	}
}

func TestEvaluateStoredBudgetForecastReadsCurrentUTCMonth(t *testing.T) {
	now := time.Date(2026, time.October, 16, 3, 0, 0, 0, time.FixedZone("UTC-7", -7*60*60))
	policy, accounts := forecastPolicyAndAccounts(250)
	reader := &analyzerTestReader{records: []normalize.CostRecord{{
		Provider: "aws", BillingAccountID: "123", Amount: "160", Currency: "USD",
		UsageStart: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
	}}}

	intent, err := EvaluateStoredBudgetForecast(context.Background(), reader, policy, accounts, now)
	if err != nil {
		t.Fatalf("EvaluateStoredBudgetForecast() error = %v", err)
	}
	monthStart := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	if reader.calls != 1 || !reader.start.Equal(monthStart) || !reader.end.Equal(now.UTC()) {
		t.Errorf("ReadCosts() = %d calls [%s, %s), want UTC [%s, %s)", reader.calls, reader.start, reader.end, monthStart, now.UTC())
	}
	if intent == nil || intent.ProjectedSpend != "310.00" {
		t.Errorf("forecast intent = %#v, want projected spend 310.00", intent)
	}
}

func TestEvaluateStoredBudgetForecastSkipsReadWhenDisabled(t *testing.T) {
	policy, accounts := forecastPolicyAndAccounts(250)
	policy.Spec.Forecast.Enabled = false
	reader := &analyzerTestReader{}
	intent, err := EvaluateStoredBudgetForecast(context.Background(), reader, policy, accounts, time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC))
	if err != nil || intent != nil || reader.calls != 0 {
		t.Fatalf("intent = %#v, error = %v, reads = %d; want disabled no-op", intent, err, reader.calls)
	}
}

func TestEvaluateBudgetForecastReturnsReaderError(t *testing.T) {
	policy, accounts := forecastPolicyAndAccounts(250)
	wantErr := errors.New("ClickHouse unavailable")
	reader := &analyzerTestReader{err: wantErr}
	_, err := EvaluateStoredBudgetForecast(context.Background(), reader, policy, accounts, time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC))
	if !errors.Is(err, wantErr) {
		t.Fatalf("EvaluateStoredBudgetForecast() error = %v, want reader error", err)
	}
}

func forecastPolicyAndAccounts(budget int64) (v1alpha1.BudgetPolicy, []v1alpha1.CloudAccount) {
	return v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "forecast"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: budget, Currency: "USD"},
		Forecast: &v1alpha1.BudgetForecastSpec{Enabled: true},
	}}, []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{
		Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "sre"},
	}}}
}
