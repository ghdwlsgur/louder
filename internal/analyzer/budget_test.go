package analyzer

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/notifier"
	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/storage"
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

func TestEvaluateStoredBudgetReadsSelectedAccountsForCurrentUTCMonth(t *testing.T) {
	now := time.Date(2026, time.October, 1, 3, 0, 0, 0, time.FixedZone("UTC-7", -7*60*60))
	policy := v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector:   map[string]string{"team": "sre"},
		Amount:     v1alpha1.BudgetAmount{Value: 100, Currency: "USD"},
		Thresholds: []int32{80},
	}}
	accounts := []v1alpha1.CloudAccount{
		{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "sre"}}},
		{Spec: v1alpha1.CloudAccountSpec{Provider: "gcp", AccountID: "456", Metadata: map[string]string{"team": "engineering"}}},
	}
	reader := &analyzerTestReader{records: []normalize.CostRecord{{
		Provider: "aws", BillingAccountID: "123", SourceRecordID: "record-1", Amount: "80.00", Currency: "USD",
		UsageStart: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
	}}}

	intents, err := EvaluateStoredBudget(context.Background(), reader, policy, accounts, now)
	if err != nil {
		t.Fatalf("EvaluateStoredBudget() error = %v", err)
	}
	if reader.calls != 1 || len(reader.accounts) != 1 || reader.accounts[0] != (storage.AccountScope{Provider: "aws", BillingAccountID: "123"}) {
		t.Errorf("reader scopes = %#v in %d calls, want only matching AWS account", reader.accounts, reader.calls)
	}
	monthStart := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	if reader.start.Location() != time.UTC || reader.end.Location() != time.UTC || !reader.start.Equal(monthStart) || !reader.end.Equal(now.UTC()) {
		t.Errorf("reader range = [%s, %s), want UTC [%s, %s)", reader.start, reader.end, monthStart, now.UTC())
	}
	if len(intents) != 1 || intents[0].ThresholdPercent != 80 || intents[0].Spent != "80.00" {
		t.Errorf("intents = %#v, want 80%% threshold with exact spend", intents)
	}
}

func TestEvaluateStoredBudgetSkipsReaderWhenNoAccountsMatch(t *testing.T) {
	policy := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{
		Selector:   map[string]string{"team": "sre"},
		Amount:     v1alpha1.BudgetAmount{Value: 100, Currency: "USD"},
		Thresholds: []int32{80},
	}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "engineering"}}}}
	reader := &analyzerTestReader{}

	intents, err := EvaluateStoredBudget(context.Background(), reader, policy, accounts, time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EvaluateStoredBudget() error = %v", err)
	}
	if reader.calls != 0 || len(intents) != 0 {
		t.Errorf("reader calls = %d, intents = %#v; want no query and no intents", reader.calls, intents)
	}
}

func TestEvaluateStoredBudgetReturnsNoPartialIntentsOnReadError(t *testing.T) {
	policy := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{
		Amount:     v1alpha1.BudgetAmount{Value: 100, Currency: "USD"},
		Thresholds: []int32{80},
	}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123"}}}
	reader := &analyzerTestReader{err: errors.New("clickhouse unavailable")}

	intents, err := EvaluateStoredBudget(context.Background(), reader, policy, accounts, time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("EvaluateStoredBudget() error = nil, want read error")
	}
	if len(intents) != 0 {
		t.Errorf("intents = %#v, want no partial result", intents)
	}
}

func TestEvaluateAndNotifyBudgetSendsReachedThreshold(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	policy := v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly"}, Spec: v1alpha1.BudgetPolicySpec{
		Amount:     v1alpha1.BudgetAmount{Value: 100, Currency: "USD"},
		Thresholds: []int32{80},
	}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123"}}}
	reader := &analyzerTestReader{records: []normalize.CostRecord{{
		Provider: "aws", BillingAccountID: "123", Amount: "80.00", Currency: "USD", UsageStart: now.Add(-time.Hour),
	}}}
	fake := &notifier.FakeNotifier{}

	intents, err := EvaluateAndNotifyBudget(context.Background(), reader, fake, policy, accounts, now)
	if err != nil {
		t.Fatalf("EvaluateAndNotifyBudget() error = %v", err)
	}
	if len(intents) != 1 || len(fake.Notifications) != 1 {
		t.Fatalf("intents = %#v, notifications = %#v; want one of each", intents, fake.Notifications)
	}
	want := notifier.Notification{
		Type: "BudgetThreshold", Severity: "Warning", Title: "Monthly budget threshold reached",
		Summary: "Budget policy sre-monthly reached 80% of its monthly limit.",
		Details: map[string]string{"Budget": "100", "Currency": "USD", "Spend": "80.00", "Threshold": "80%"},
	}
	if !reflect.DeepEqual(fake.Notifications[0], want) {
		t.Errorf("notification = %#v, want %#v", fake.Notifications[0], want)
	}
}

func TestEvaluateAndNotifyBudgetDoesNotNotifyBelowThreshold(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	policy := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{
		Amount:     v1alpha1.BudgetAmount{Value: 100, Currency: "USD"},
		Thresholds: []int32{80},
	}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123"}}}
	reader := &analyzerTestReader{records: []normalize.CostRecord{{
		Provider: "aws", BillingAccountID: "123", Amount: "79.99", Currency: "USD", UsageStart: now.Add(-time.Hour),
	}}}
	fake := &notifier.FakeNotifier{}

	intents, err := EvaluateAndNotifyBudget(context.Background(), reader, fake, policy, accounts, now)
	if err != nil {
		t.Fatalf("EvaluateAndNotifyBudget() error = %v", err)
	}
	if len(intents) != 0 || len(fake.Notifications) != 0 {
		t.Errorf("intents = %#v, notifications = %#v; want no threshold or notification", intents, fake.Notifications)
	}
}

func TestEvaluateAndNotifyBudgetStopsAfterNotifierFailure(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	policy := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{
		Amount:     v1alpha1.BudgetAmount{Value: 100, Currency: "USD"},
		Thresholds: []int32{50, 60, 70, 80},
	}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123"}}}
	reader := &analyzerTestReader{records: []normalize.CostRecord{{
		Provider: "aws", BillingAccountID: "123", Amount: "90.00", Currency: "USD", UsageStart: now.Add(-time.Hour),
	}}}
	delivery := &failingAnalyzerNotifier{failAt: 2, err: errors.New("delivery failed")}

	intents, err := EvaluateAndNotifyBudget(context.Background(), reader, delivery, policy, accounts, now)
	if err == nil {
		t.Fatal("EvaluateAndNotifyBudget() error = nil, want notifier error")
	}
	if len(intents) != 4 || len(delivery.attempts) != 2 {
		t.Errorf("intents = %d, notifier attempts = %d; want four intents and stop after second attempt", len(intents), len(delivery.attempts))
	}
}

type failingAnalyzerNotifier struct {
	attempts []notifier.Notification
	failAt   int
	err      error
}

func (n *failingAnalyzerNotifier) Send(_ context.Context, notification notifier.Notification) error {
	n.attempts = append(n.attempts, notification)
	if len(n.attempts) == n.failAt {
		return n.err
	}
	return nil
}

type analyzerTestReader struct {
	calls    int
	accounts []storage.AccountScope
	start    time.Time
	end      time.Time
	records  []normalize.CostRecord
	err      error
}

func (r *analyzerTestReader) ReadCosts(_ context.Context, accounts []storage.AccountScope, start, end time.Time) ([]normalize.CostRecord, error) {
	r.calls++
	r.accounts = append([]storage.AccountScope(nil), accounts...)
	r.start = start
	r.end = end
	return r.records, r.err
}
