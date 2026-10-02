package analyzer

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/notifier"
	"github.com/ghdwlsgur/louder/internal/storage/clickhouse"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
