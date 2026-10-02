package analyzer

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/notifier"
	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/storage/clickhouse"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestStoredBudgetProducesFakeNotifierIntents(t *testing.T) {
	if os.Getenv("CLICKHOUSE_INTEGRATION") != "1" {
		t.Skip("ClickHouse integration environment is not enabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := clickhouse.OpenFromEnv(ctx)
	if err != nil {
		t.Fatalf("OpenFromEnv() error = %v", err)
	}
	defer store.Close()

	policy := v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector:   map[string]string{"team": "sre"},
		Amount:     v1alpha1.BudgetAmount{Value: 10, Currency: "USD"},
		Thresholds: []int32{100, 80},
	}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{
		Provider: "aws", AccountID: "synthetic-kind-account", Metadata: map[string]string{"team": "sre"},
	}}}
	fake := &notifier.FakeNotifier{}
	now := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)

	intents, err := EvaluateAndNotifyBudget(ctx, store, fake, policy, accounts, now)
	if err != nil {
		t.Fatalf("EvaluateAndNotifyBudget() error = %v", err)
	}
	if len(intents) != 2 || len(fake.Notifications) != 2 {
		t.Fatalf("intents = %#v, notifications = %#v; want 80%% and 100%% threshold notifications", intents, fake.Notifications)
	}
	want := []notifier.Notification{
		{Type: "BudgetThreshold", Severity: "Warning", Title: "Monthly budget threshold reached", Summary: "Budget policy sre-monthly reached 80% of its monthly limit.", Details: map[string]string{"Budget": "10", "Currency": "USD", "Spend": "12.34", "Threshold": "80%"}},
		{Type: "BudgetThreshold", Severity: "Warning", Title: "Monthly budget threshold reached", Summary: "Budget policy sre-monthly reached 100% of its monthly limit.", Details: map[string]string{"Budget": "10", "Currency": "USD", "Spend": "12.34", "Threshold": "100%"}},
	}
	if !reflect.DeepEqual(fake.Notifications, want) {
		t.Errorf("notifications = %#v, want %#v", fake.Notifications, want)
	}
}

func TestStoredMonthlyNCPBudgetProducesFakeNotifierIntents(t *testing.T) {
	if os.Getenv("CLICKHOUSE_INTEGRATION") != "1" {
		t.Skip("ClickHouse integration environment is not enabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := clickhouse.OpenFromEnv(ctx)
	if err != nil {
		t.Fatalf("OpenFromEnv() error = %v", err)
	}
	defer store.Close()

	policy := v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "ncp-monthly"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector:   map[string]string{"team": "sre"},
		Amount:     v1alpha1.BudgetAmount{Value: 10000, Currency: "KRW"},
		Thresholds: []int32{100, 80},
	}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{
		Provider: "ncp", AccountID: "2760000", Metadata: map[string]string{"team": "sre"},
	}}}
	fake := &notifier.FakeNotifier{}
	now := time.Date(2026, time.October, 2, 0, 0, 0, 0, time.UTC)

	intents, err := EvaluateAndNotifyBudget(ctx, store, fake, policy, accounts, now)
	if err != nil {
		t.Fatalf("EvaluateAndNotifyBudget() error = %v", err)
	}
	if len(intents) != 2 || len(fake.Notifications) != 2 {
		t.Fatalf("intents = %#v, notifications = %#v; want 80%% and 100%% threshold notifications", intents, fake.Notifications)
	}
	want := []notifier.Notification{
		{Type: "BudgetThreshold", Severity: "Warning", Title: "Monthly budget threshold reached", Summary: "Budget policy ncp-monthly reached 80% of its monthly limit.", Details: map[string]string{"Budget": "10000", "Currency": "KRW", "Spend": "12345.67", "Threshold": "80%"}},
		{Type: "BudgetThreshold", Severity: "Warning", Title: "Monthly budget threshold reached", Summary: "Budget policy ncp-monthly reached 100% of its monthly limit.", Details: map[string]string{"Budget": "10000", "Currency": "KRW", "Spend": "12345.67", "Threshold": "100%"}},
	}
	if !reflect.DeepEqual(fake.Notifications, want) {
		t.Errorf("notifications = %#v, want %#v", fake.Notifications, want)
	}
}

func TestStoredDailyAnomalyProducesFakeNotifierIntents(t *testing.T) {
	if os.Getenv("CLICKHOUSE_INTEGRATION") != "1" {
		t.Skip("ClickHouse integration environment is not enabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := clickhouse.OpenFromEnv(ctx)
	if err != nil {
		t.Fatalf("OpenFromEnv() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, time.October, 9, 0, 2, 0, 0, time.UTC)
	collected := metav1.NewTime(now)
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{
		Provider: "aws", AccountID: "daily-anomaly-kind-account", Metadata: map[string]string{"team": "sre"},
		Collection: v1alpha1.CollectionSpec{Enabled: true},
	}, Status: v1alpha1.CloudAccountStatus{LastSuccessfulCollectionTime: &collected}}}
	budget := v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "daily-anomaly"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: 1000, Currency: "USD"},
		DailyAnomaly: &v1alpha1.DailyAnomalySpec{AbsoluteIncreaseThreshold: 10},
	}}
	windowStart := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	records := make([]normalize.CostRecord, 0, 8)
	for offset := 0; offset < 8; offset++ {
		start := windowStart.AddDate(0, 0, offset)
		amount := "10"
		if offset == 7 {
			amount = "30"
		}
		records = append(records, normalize.CostRecord{
			Provider: "aws", BillingAccountID: "daily-anomaly-kind-account",
			SourceRecordID: fmt.Sprintf("kind-daily-anomaly-aws-%s", start.Format("2006-01-02")),
			CostBasis:      provider.CostBasisNet, Amount: amount, Currency: "USD",
			UsageStart: start, UsageEnd: start.AddDate(0, 0, 1),
		})
	}
	if err := store.WriteCosts(ctx, records); err != nil {
		t.Fatalf("WriteCosts() error = %v", err)
	}

	intents, err := EvaluateStoredDailyCostAnomalies(ctx, store, budget, accounts, now)
	if err != nil {
		t.Fatalf("EvaluateStoredDailyCostAnomalies() error = %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("EvaluateStoredDailyCostAnomalies() = %#v, want one alert from the stored target day and seven-day baseline", intents)
	}

	fake := &notifier.FakeNotifier{}
	policy := v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "daily-teams"}, Spec: v1alpha1.NotificationPolicySpec{
		Type: "teams", Events: []string{"CostAnomaly"}, Selector: map[string]string{"team": "sre"},
		CredentialRef: corev1.LocalObjectReference{Name: "teams-hook"},
	}}
	resolve := func(context.Context, v1alpha1.NotificationPolicy) (notifier.Notifier, error) { return fake, nil }
	if err := NotifyDailyAnomalyIntents(ctx, budget, intents, accounts, []v1alpha1.NotificationPolicy{policy}, resolve); err != nil {
		t.Fatalf("NotifyDailyAnomalyIntents() error = %v", err)
	}
	want := []notifier.Notification{{
		Type: "CostAnomaly", Severity: "Warning", Title: "Daily cost increase detected",
		Summary: "aws cost on 2026-10-08 was 20 USD above the seven-day average.",
		Details: map[string]string{"Policy": "daily-anomaly", "Provider": "aws", "BillingAccountID": "daily-anomaly-kind-account", "Date": "2026-10-08", "Today": "30", "BaselineAverage": "10", "Increase": "20", "Currency": "USD"},
	}}
	if !reflect.DeepEqual(fake.Notifications, want) {
		t.Errorf("notifications = %#v, want %#v", fake.Notifications, want)
	}
}

func TestStoredDailyAnomalyRuntimeDeduplicates(t *testing.T) {
	if os.Getenv("CLICKHOUSE_INTEGRATION") != "1" {
		t.Skip("ClickHouse integration environment is not enabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := clickhouse.OpenFromEnv(ctx)
	if err != nil {
		t.Fatalf("OpenFromEnv() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, time.October, 9, 0, 2, 0, 0, time.UTC)
	collected := metav1.NewTime(now)
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "daily-anomaly-runtime", Namespace: "cloud-cost"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: 1000, Currency: "USD"},
		DailyAnomaly: &v1alpha1.DailyAnomalySpec{AbsoluteIncreaseThreshold: 10},
	}}
	account := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "aws-daily-runtime", Namespace: "cloud-cost"}, Spec: v1alpha1.CloudAccountSpec{
		Provider: "aws", AccountID: "daily-anomaly-runtime-kind-account", Metadata: map[string]string{"team": "sre"}, Collection: v1alpha1.CollectionSpec{Enabled: true},
	}, Status: v1alpha1.CloudAccountStatus{LastSuccessfulCollectionTime: &collected}}
	policy := &v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "daily-runtime-teams", Namespace: "cloud-cost"}, Spec: v1alpha1.NotificationPolicySpec{
		Type: "teams", Events: []string{"CostAnomaly"}, Selector: map[string]string{"team": "sre"}, CredentialRef: corev1.LocalObjectReference{Name: "daily-runtime-hook"},
	}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "daily-runtime-hook", Namespace: "cloud-cost"}, Data: map[string][]byte{"TEAMS_WEBHOOK_URL": []byte("https://teams.example.test/hook")}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget, account, policy, secret).Build()
	windowStart := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	records := make([]normalize.CostRecord, 0, 8)
	for offset := 0; offset < 8; offset++ {
		start := windowStart.AddDate(0, 0, offset)
		amount := "10"
		if offset == 7 {
			amount = "30"
		}
		records = append(records, normalize.CostRecord{
			Provider: "aws", BillingAccountID: "daily-anomaly-runtime-kind-account",
			SourceRecordID: fmt.Sprintf("kind-daily-anomaly-runtime-aws-%s", start.Format("2006-01-02")),
			CostBasis:      provider.CostBasisNet, Amount: amount, Currency: "USD",
			UsageStart: start, UsageEnd: start.AddDate(0, 0, 1),
		})
	}
	if err := store.WriteCosts(ctx, records); err != nil {
		t.Fatalf("WriteCosts() error = %v", err)
	}
	delivery := &notifier.FakeNotifier{}
	factory := func(string) (notifier.Notifier, error) { return delivery, nil }

	for range 2 {
		if _, err := RunBudgetPolicy(ctx, kube, store, "cloud-cost", "daily-anomaly-runtime", factory, now); err != nil {
			t.Fatalf("RunBudgetPolicy() error = %v", err)
		}
	}
	if len(delivery.Notifications) != 1 {
		t.Fatalf("notifications = %#v, want one daily anomaly from ClickHouse after two runtime passes", delivery.Notifications)
	}
	if got := delivery.Notifications[0].Details["Date"]; got != "2026-10-08" {
		t.Errorf("notification date = %q, want 2026-10-08", got)
	}
	var stored v1alpha1.BudgetPolicy
	if err := kube.Get(ctx, types.NamespacedName{Namespace: "cloud-cost", Name: "daily-anomaly-runtime"}, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.LastNotifiedAnomalyDate != "2026-10-08" {
		t.Errorf("LastNotifiedAnomalyDate = %q, want 2026-10-08", stored.Status.LastNotifiedAnomalyDate)
	}
}
