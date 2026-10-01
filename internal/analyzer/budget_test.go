package analyzer

import (
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/provider"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEvaluateBudgetReturnsReachedThreshold(t *testing.T) {
	now := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	policy := v1alpha1.BudgetPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly"},
		Spec: v1alpha1.BudgetPolicySpec{
			Selector:   map[string]string{"team": "sre"},
			Amount:     v1alpha1.BudgetAmount{Value: 100, Currency: "USD"},
			Thresholds: []int32{80},
		},
	}
	accounts := []v1alpha1.CloudAccount{{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod"},
		Spec:       v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "sre"}},
	}}
	records := []normalize.CostRecord{{
		Provider: "aws", BillingAccountID: "123", SourceRecordID: "aws-2026-10-01",
		CostBasis: provider.CostBasisUnblended, Amount: "80.00", Currency: "USD",
		UsageStart: now.Add(-time.Hour), UsageEnd: now.Add(23 * time.Hour),
	}}

	intents, err := EvaluateBudget(policy, accounts, records, now)
	if err != nil {
		t.Fatalf("EvaluateBudget() error = %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("intents = %#v, want one reached threshold", intents)
	}
	want := BudgetThresholdIntent{PolicyName: "sre-monthly", ThresholdPercent: 80, Spent: "80.00", Budget: 100, Currency: "USD"}
	if intents[0] != want {
		t.Errorf("intent = %#v, want %#v", intents[0], want)
	}
}

func TestEvaluateBudgetExcludesUnselectedAndOutOfMonthRecords(t *testing.T) {
	now := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	policy := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{
		Selector:   map[string]string{"team": "sre"},
		Amount:     v1alpha1.BudgetAmount{Value: 100, Currency: "USD"},
		Thresholds: []int32{80},
	}}
	accounts := []v1alpha1.CloudAccount{
		{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "selected", Metadata: map[string]string{"team": "sre"}}},
		{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "other-team", Metadata: map[string]string{"team": "engineering"}}},
	}
	records := []normalize.CostRecord{
		{Provider: "aws", BillingAccountID: "selected", Amount: "79", Currency: "USD", UsageStart: time.Date(2026, time.September, 30, 23, 59, 0, 0, time.UTC)},
		{Provider: "aws", BillingAccountID: "selected", Amount: "79", Currency: "USD", UsageStart: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60))},
		{Provider: "aws", BillingAccountID: "selected", Amount: "79", Currency: "USD", UsageStart: now},
		{Provider: "aws", BillingAccountID: "other-team", Amount: "100", Currency: "USD", UsageStart: now.Add(-time.Hour)},
		{Provider: "aws", BillingAccountID: "selected", Amount: "79.00", Currency: "USD", UsageStart: now.Add(-time.Hour)},
	}

	intents, err := EvaluateBudget(policy, accounts, records, now)
	if err != nil {
		t.Fatalf("EvaluateBudget() error = %v", err)
	}
	if len(intents) != 0 {
		t.Errorf("intents = %#v, want no threshold because only 79.00 current-month USD is selected", intents)
	}
}

func TestEvaluateBudgetRejectsInvalidThresholdWithoutPartialIntents(t *testing.T) {
	policy := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{
		Amount:     v1alpha1.BudgetAmount{Value: 100, Currency: "USD"},
		Thresholds: []int32{80, 101},
	}}
	intents, err := EvaluateBudget(policy, nil, nil, time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("EvaluateBudget() error = nil, want invalid threshold error")
	}
	if len(intents) != 0 {
		t.Errorf("intents = %#v, want no partial results", intents)
	}
}

func TestEvaluateBudgetRejectsSelectedAccountCurrencyMismatch(t *testing.T) {
	policy := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{
		Amount:     v1alpha1.BudgetAmount{Value: 100, Currency: "USD"},
		Thresholds: []int32{80},
	}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123"}}}
	records := []normalize.CostRecord{{
		Provider: "aws", BillingAccountID: "123", Amount: "80", Currency: "CAD",
		UsageStart: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
	}}

	intents, err := EvaluateBudget(policy, accounts, records, time.Date(2026, time.October, 2, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("EvaluateBudget() error = nil, want currency mismatch")
	}
	if len(intents) != 0 {
		t.Errorf("intents = %#v, want no partial results", intents)
	}
}

func TestEvaluateBudgetAggregatesExactAmountsAndSortsThresholds(t *testing.T) {
	now := time.Date(2026, time.October, 2, 0, 0, 0, 0, time.UTC)
	policy := v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector:   map[string]string{"team": "sre"},
		Amount:     v1alpha1.BudgetAmount{Value: 100, Currency: "USD"},
		Thresholds: []int32{100, 80, 90},
	}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "sre"}}}}
	start := now.Add(-time.Hour)
	records := []normalize.CostRecord{
		{Provider: "aws", BillingAccountID: "123", Amount: "99.99", Currency: "USD", UsageStart: start},
		{Provider: "aws", BillingAccountID: "123", Amount: "0.01", Currency: "USD", UsageStart: start},
	}

	intents, err := EvaluateBudget(policy, accounts, records, now)
	if err != nil {
		t.Fatalf("EvaluateBudget() error = %v", err)
	}
	wantThresholds := []int32{80, 90, 100}
	if len(intents) != len(wantThresholds) {
		t.Fatalf("intents = %#v, want thresholds %v", intents, wantThresholds)
	}
	for i, intent := range intents {
		if intent.ThresholdPercent != wantThresholds[i] || intent.Spent != "100.00" {
			t.Errorf("intents[%d] = %#v, want threshold %d and exact spend 100.00", i, intent, wantThresholds[i])
		}
	}
}
