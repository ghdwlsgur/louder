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

func TestEvaluateDailyCostAnomaliesReportsRelativeAndAbsoluteIncrease(t *testing.T) {
	targetDay := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	got, err := EvaluateDailyCostAnomalies("sre-monthly", anomalySelector(), anomalyAccounts(), anomalyRecords(targetDay, "25.00"), targetDay, v1alpha1.BudgetAmount{Value: 5, Currency: "USD"})
	if err != nil {
		t.Fatalf("EvaluateDailyCostAnomalies() error = %v", err)
	}
	want := []DailyCostAnomalyIntent{{
		PolicyName: "sre-monthly", Provider: "aws", BillingAccountID: "123", Date: targetDay,
		Today: "25.00", BaselineAverage: "10.00", Increase: "15.00", Currency: "USD",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EvaluateDailyCostAnomalies() = %#v, want %#v", got, want)
	}
}

func TestEvaluateDailyCostAnomaliesRequiresAbsoluteIncrease(t *testing.T) {
	targetDay := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	got, err := EvaluateDailyCostAnomalies("sre-monthly", anomalySelector(), anomalyAccounts(), anomalyRecords(targetDay, "16.00"), targetDay, v1alpha1.BudgetAmount{Value: 7, Currency: "USD"})
	if err != nil {
		t.Fatalf("EvaluateDailyCostAnomalies() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("EvaluateDailyCostAnomalies() = %#v, want no intent when absolute increase is below threshold", got)
	}
}

func TestEvaluateDailyCostAnomaliesRequiresRelativeIncrease(t *testing.T) {
	targetDay := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	got, err := EvaluateDailyCostAnomalies("sre-monthly", anomalySelector(), anomalyAccounts(), anomalyRecords(targetDay, "14.00"), targetDay, v1alpha1.BudgetAmount{Value: 3, Currency: "USD"})
	if err != nil {
		t.Fatalf("EvaluateDailyCostAnomalies() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("EvaluateDailyCostAnomalies() = %#v, want no intent when relative increase is below 1.5x", got)
	}
}

func TestEvaluateDailyCostAnomaliesKeepsAccountScopesIndependent(t *testing.T) {
	targetDay := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	accounts := append(anomalyAccounts(), v1alpha1.CloudAccount{Spec: v1alpha1.CloudAccountSpec{Provider: "oci", AccountID: "tenancy", Metadata: map[string]string{"team": "sre"}}})
	records := anomalyRecords(targetDay, "25.00")
	for offset := -7; offset <= 0; offset++ {
		day := targetDay.AddDate(0, 0, offset)
		records = append(records, normalize.CostRecord{
			Provider: "oci", BillingAccountID: "tenancy", SourceRecordID: "oci-" + day.Format("2006-01-02"),
			CostBasis: provider.CostBasisOCI, Amount: "100.00", Currency: "USD",
			UsageStart: day, UsageEnd: day.AddDate(0, 0, 1),
		})
	}

	got, err := EvaluateDailyCostAnomalies("sre-monthly", anomalySelector(), accounts, records, targetDay, v1alpha1.BudgetAmount{Value: 5, Currency: "USD"})
	if err != nil {
		t.Fatalf("EvaluateDailyCostAnomalies() error = %v", err)
	}
	if len(got) != 1 || got[0].Provider != "aws" {
		t.Fatalf("EvaluateDailyCostAnomalies() = %#v, want only the independently anomalous AWS scope", got)
	}
}

func TestEvaluateDailyCostAnomaliesRejectsCostBasisChangeWithinScope(t *testing.T) {
	targetDay := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	records := anomalyRecords(targetDay, "25.00")
	records[0].CostBasis = provider.CostBasisOCI
	_, err := EvaluateDailyCostAnomalies("sre-monthly", anomalySelector(), anomalyAccounts(), records, targetDay, v1alpha1.BudgetAmount{Value: 5, Currency: "USD"})
	if !errors.Is(err, ErrMixedCostBasis) {
		t.Fatalf("EvaluateDailyCostAnomalies() error = %v, want mixed cost basis", err)
	}
}

func TestEvaluateDailyCostAnomaliesRejectsCurrencyMismatch(t *testing.T) {
	targetDay := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	records := anomalyRecords(targetDay, "25.00")
	records[0].Currency = "KRW"
	_, err := EvaluateDailyCostAnomalies("sre-monthly", anomalySelector(), anomalyAccounts(), records, targetDay, v1alpha1.BudgetAmount{Value: 5, Currency: "USD"})
	if !errors.Is(err, ErrDailyAnomalyCurrencyMismatch) {
		t.Fatalf("EvaluateDailyCostAnomalies() error = %v, want currency mismatch", err)
	}
}

func TestEvaluateStoredDailyCostAnomaliesReadsTargetAndSevenBaselineDays(t *testing.T) {
	now := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)
	targetDay := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	collectionTime := metav1.NewTime(time.Date(2026, time.October, 2, 0, 5, 0, 0, time.UTC))
	reader := &analyzerTestReader{records: anomalyRecords(targetDay, "25.00")}
	policy := v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: 1000, Currency: "USD"},
		DailyAnomaly: &v1alpha1.DailyAnomalySpec{AbsoluteIncreaseThreshold: 5},
	}}
	account := anomalyAccounts()[0]
	account.Spec.Collection.Enabled = true
	account.Status.LastSuccessfulCollectionTime = &collectionTime

	got, err := EvaluateStoredDailyCostAnomalies(context.Background(), reader, policy, []v1alpha1.CloudAccount{account}, now)
	if err != nil {
		t.Fatalf("EvaluateStoredDailyCostAnomalies() error = %v", err)
	}
	if len(got) != 1 || !got[0].Date.Equal(targetDay) {
		t.Fatalf("EvaluateStoredDailyCostAnomalies() = %#v, want one anomaly for %s", got, targetDay)
	}
	wantStart := targetDay.AddDate(0, 0, -7)
	wantEnd := targetDay.AddDate(0, 0, 1)
	if reader.calls != 1 || !reader.start.Equal(wantStart) || !reader.end.Equal(wantEnd) {
		t.Errorf("ReadCosts() calls/range = %d [%s, %s), want 1 [%s, %s)", reader.calls, reader.start, reader.end, wantStart, wantEnd)
	}
	if reader.accounts[0] != (storage.AccountScope{Provider: "aws", BillingAccountID: "123"}) {
		t.Errorf("ReadCosts() scopes = %#v, want the selected account only", reader.accounts)
	}
}

func TestEvaluateStoredDailyCostAnomaliesRejectsStaleCollection(t *testing.T) {
	now := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)
	policy := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{
		Amount:       v1alpha1.BudgetAmount{Value: 1000, Currency: "USD"},
		DailyAnomaly: &v1alpha1.DailyAnomalySpec{AbsoluteIncreaseThreshold: 5},
	}}
	account := anomalyAccounts()[0]
	account.Spec.Collection.Enabled = true
	stale := metav1.NewTime(time.Date(2026, time.October, 1, 23, 59, 59, 0, time.UTC))
	account.Status.LastSuccessfulCollectionTime = &stale
	reader := &analyzerTestReader{}

	_, err := EvaluateStoredDailyCostAnomalies(context.Background(), reader, policy, []v1alpha1.CloudAccount{account}, now)
	if !errors.Is(err, ErrDailyAnomalySourceStale) {
		t.Fatalf("EvaluateStoredDailyCostAnomalies() error = %v, want stale-source error", err)
	}
	if reader.calls != 0 {
		t.Errorf("ReadCosts() calls = %d, want no read before collection freshness is established", reader.calls)
	}
}

func TestNotifyDailyAnomalyIntentsHonorsPerDestinationAccountSelector(t *testing.T) {
	date := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	budget := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{Selector: map[string]string{"environment": "prod"}}}
	accounts := []v1alpha1.CloudAccount{
		{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "111", Metadata: map[string]string{"environment": "prod", "team": "platform"}}},
		{Spec: v1alpha1.CloudAccountSpec{Provider: "gcp", AccountID: "222", Metadata: map[string]string{"environment": "prod", "team": "data"}}},
	}
	policies := []v1alpha1.NotificationPolicy{
		{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"CostAnomaly"}, Selector: map[string]string{"team": "platform"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"CostAnomaly"}, Selector: map[string]string{"team": "data"}}},
	}
	platformDelivery := &notifier.FakeNotifier{}
	dataDelivery := &notifier.FakeNotifier{}
	resolve := func(_ context.Context, policy v1alpha1.NotificationPolicy) (notifier.Notifier, error) {
		if policy.Name == "platform" {
			return platformDelivery, nil
		}
		return dataDelivery, nil
	}
	intents := []DailyCostAnomalyIntent{
		{Provider: "aws", BillingAccountID: "111", Date: date},
		{Provider: "gcp", BillingAccountID: "222", Date: date},
	}
	if err := NotifyDailyAnomalyIntents(context.Background(), budget, intents, accounts, policies, resolve); err != nil {
		t.Fatalf("NotifyDailyAnomalyIntents() error = %v", err)
	}
	if len(platformDelivery.Notifications) != 1 || platformDelivery.Notifications[0].Details["Provider"] != "aws" {
		t.Errorf("platform notifications = %#v, want only AWS account 111", platformDelivery.Notifications)
	}
	if len(dataDelivery.Notifications) != 1 || dataDelivery.Notifications[0].Details["Provider"] != "gcp" {
		t.Errorf("data notifications = %#v, want only GCP account 222", dataDelivery.Notifications)
	}
}

func anomalySelector() map[string]string {
	return map[string]string{"team": "sre"}
}

func anomalyAccounts() []v1alpha1.CloudAccount {
	return []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "sre"}}}}
}

func anomalyRecords(targetDay time.Time, targetAmount string) []normalize.CostRecord {
	records := make([]normalize.CostRecord, 0, 8)
	for offset := -7; offset <= 0; offset++ {
		day := targetDay.AddDate(0, 0, offset)
		amount := "10.00"
		if offset == 0 {
			amount = targetAmount
		}
		records = append(records, normalize.CostRecord{
			Provider: "aws", BillingAccountID: "123", SourceRecordID: "aws-" + day.Format("2006-01-02"),
			CostBasis: provider.CostBasisNet, Amount: amount, Currency: "USD",
			UsageStart: day, UsageEnd: day.AddDate(0, 0, 1),
		})
	}
	return records
}
