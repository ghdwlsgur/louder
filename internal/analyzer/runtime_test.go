package analyzer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/notifier"
	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/storage"
	corev1 "k8s.io/api/core/v1"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type runtimeFreshnessReader struct {
	*analyzerTestReader
	runs []storage.CollectionRun
}

func (r *runtimeFreshnessReader) ReadCollectionRuns(context.Context, []storage.AccountScope, time.Time, time.Time) ([]storage.CollectionRun, error) {
	return r.runs, nil
}

func TestRunBudgetPolicyLoadsNamespaceResourcesAndSends(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	budget := &v1alpha1.BudgetPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly", Namespace: "costs"},
		Spec:       v1alpha1.BudgetPolicySpec{Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}, Thresholds: []int32{80}},
	}
	account := &v1alpha1.CloudAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs"},
		Spec:       v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "sre"}},
	}
	policy := &v1alpha1.NotificationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "central-teams", Namespace: "costs"},
		Spec:       v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"BudgetThreshold"}, CredentialRef: corev1.LocalObjectReference{Name: "teams-hook"}},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "teams-hook", Namespace: "costs"},
		Data:       map[string][]byte{"TEAMS_WEBHOOK_URL": []byte("https://teams.example.test/hook")},
	}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget, account, policy, secret).Build()
	reader := &analyzerTestReader{records: []normalize.CostRecord{{Provider: "aws", BillingAccountID: "123", Amount: "80", Currency: "USD", UsageStart: now.Add(-time.Hour)}}}
	delivery := &notifier.FakeNotifier{}
	var gotEndpoint string
	factory := func(endpoint string) (notifier.Notifier, error) {
		gotEndpoint = endpoint
		return delivery, nil
	}

	intents, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "sre-monthly", factory, now)
	if err != nil {
		t.Fatalf("RunBudgetPolicy() error = %v", err)
	}
	if len(intents) != 1 || len(delivery.Notifications) != 1 {
		t.Fatalf("intents = %#v, notifications = %#v; want one of each", intents, delivery.Notifications)
	}
	if gotEndpoint != string(secret.Data["TEAMS_WEBHOOK_URL"]) {
		t.Errorf("notifier endpoint = %q, want Secret-provided endpoint", gotEndpoint)
	}
}

func TestRunBudgetPolicyExcludesStaleCostDataAndUpdatesAccountReadiness(t *testing.T) {
	now := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "monthly", Namespace: "costs"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}, Thresholds: []int32{80},
	}}
	staleAccount := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "stale", Namespace: "costs"}, Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "stale-id", Metadata: map[string]string{"team": "sre"}}}
	freshAccount := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "fresh", Namespace: "costs"}, Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "fresh-id", Metadata: map[string]string{"team": "sre"}}}
	policy := &v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "teams", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"BudgetThreshold"}, CredentialRef: corev1.LocalObjectReference{Name: "hook"}}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "hook", Namespace: "costs"}, Data: map[string][]byte{"TEAMS_WEBHOOK_URL": []byte("https://teams.example.test/hook")}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget, staleAccount, freshAccount).WithObjects(budget, staleAccount, freshAccount, policy, secret).Build()
	base := storage.CollectionRun{WindowStart: now.AddDate(0, 0, -8), WindowEnd: now, StartedAt: now.Add(-time.Hour), CompletedAt: now, DataIngestedAt: now, RecordCount: 8}
	staleRun := base
	staleRun.Provider, staleRun.BillingAccountID, staleRun.LatestUsageEnd = "aws", "stale-id", now.Add(-48*time.Hour)
	freshRun := base
	freshRun.Provider, freshRun.BillingAccountID, freshRun.LatestUsageEnd = "aws", "fresh-id", now
	reader := &runtimeFreshnessReader{analyzerTestReader: &analyzerTestReader{records: []normalize.CostRecord{{Provider: "aws", BillingAccountID: "fresh-id", Amount: "80", Currency: "USD", UsageStart: now.Add(-time.Hour)}}}, runs: []storage.CollectionRun{staleRun, freshRun}}
	factory := func(string) (notifier.Notifier, error) { return &notifier.FakeNotifier{}, nil }
	intents, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "monthly", factory, now)
	if err != nil {
		t.Fatalf("RunBudgetPolicy() error = %v", err)
	}
	if len(intents) != 1 || len(reader.accounts) != 1 || reader.accounts[0].BillingAccountID != "fresh-id" {
		t.Fatalf("intents = %#v, budget scopes = %#v; want one threshold based only on fresh account", intents, reader.accounts)
	}
	var gotStale v1alpha1.CloudAccount
	if err := kube.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: "stale"}, &gotStale); err != nil {
		t.Fatal(err)
	}
	condition := apiMeta.FindStatusCondition(gotStale.Status.Conditions, "DataFresh")
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "ProviderDataBeyondExpectedDelay" {
		t.Fatalf("DataFresh condition = %#v, want stale status", condition)
	}
}

func TestRunBudgetPolicyDoesNotUseAccountsFromOtherNamespaces(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly", Namespace: "costs"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}, Thresholds: []int32{80},
	}}
	account := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "other-sre", Namespace: "other"}, Spec: v1alpha1.CloudAccountSpec{
		Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "sre"},
	}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(budget, account).Build()
	reader := &analyzerTestReader{records: []normalize.CostRecord{{Provider: "aws", BillingAccountID: "123", Amount: "100", Currency: "USD", UsageStart: now.Add(-time.Hour)}}}
	factory := func(string) (notifier.Notifier, error) { return &notifier.FakeNotifier{}, nil }

	intents, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "sre-monthly", factory, now)
	if err != nil {
		t.Fatalf("RunBudgetPolicy() error = %v", err)
	}
	if len(intents) != 0 || reader.calls != 0 {
		t.Fatalf("intents = %#v, reader calls = %d; want namespace-isolated no-op", intents, reader.calls)
	}
}

func TestRunBudgetPolicySanitizesMissingWebhookSecretKey(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly", Namespace: "costs"}, Spec: v1alpha1.BudgetPolicySpec{
		Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}, Thresholds: []int32{80},
	}}
	account := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs"}, Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123"}}
	policy := &v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "central", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{
		Type: "teams", Events: []string{"BudgetThreshold"}, CredentialRef: corev1.LocalObjectReference{Name: "teams-hook"},
	}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "teams-hook", Namespace: "costs"}, Data: map[string][]byte{"unrelated": []byte("secret-marker")}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(budget, account, policy, secret).Build()
	reader := &analyzerTestReader{records: []normalize.CostRecord{{Provider: "aws", BillingAccountID: "123", Amount: "80", Currency: "USD", UsageStart: now.Add(-time.Hour)}}}
	factory := func(string) (notifier.Notifier, error) { return &notifier.FakeNotifier{}, nil }

	_, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "sre-monthly", factory, now)
	if !errors.Is(err, ErrNotificationPolicyResolution) {
		t.Fatalf("RunBudgetPolicy() error = %v, want ErrNotificationPolicyResolution", err)
	}
	if strings.Contains(err.Error(), "secret-marker") {
		t.Fatalf("RunBudgetPolicy() leaked Secret content: %v", err)
	}
}

func TestRunBudgetPolicyReportsMissingBudgetPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).Build()

	_, err := RunBudgetPolicy(context.Background(), kube, &analyzerTestReader{}, "costs", "missing", nil, time.Now())
	if !errors.Is(err, ErrBudgetPolicyNotFound) {
		t.Fatalf("RunBudgetPolicy() error = %v, want ErrBudgetPolicyNotFound", err)
	}
}

func TestRunBudgetPolicySuppressesThresholdsAfterSuccessfulDeliveryThisMonth(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly", Namespace: "costs"}, Spec: v1alpha1.BudgetPolicySpec{
		Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}, Thresholds: []int32{80, 100},
	}}
	account := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs"}, Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123"}}
	policy := &v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "central", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{
		Type: "teams", Events: []string{"BudgetThreshold"}, CredentialRef: corev1.LocalObjectReference{Name: "teams-hook"},
	}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "teams-hook", Namespace: "costs"}, Data: map[string][]byte{"TEAMS_WEBHOOK_URL": []byte("https://teams.example.test/hook")}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget, account, policy, secret).Build()
	reader := &analyzerTestReader{records: []normalize.CostRecord{{Provider: "aws", BillingAccountID: "123", Amount: "100", Currency: "USD", UsageStart: now.Add(-time.Hour)}}}
	delivery := &notifier.FakeNotifier{}
	factory := func(string) (notifier.Notifier, error) { return delivery, nil }

	for range 2 {
		if _, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "sre-monthly", factory, now); err != nil {
			t.Fatalf("RunBudgetPolicy() error = %v", err)
		}
	}
	if len(delivery.Notifications) != 2 {
		t.Fatalf("notifications = %d, want each of two thresholds sent once within the month", len(delivery.Notifications))
	}
	nextMonth := now.AddDate(0, 1, 0)
	reader.records[0].UsageStart = nextMonth.Add(-time.Hour)
	if _, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "sre-monthly", factory, nextMonth); err != nil {
		t.Fatalf("RunBudgetPolicy() in the next month error = %v", err)
	}
	if len(delivery.Notifications) != 4 {
		t.Fatalf("notifications after month rollover = %d, want both thresholds sent again", len(delivery.Notifications))
	}
	var stored v1alpha1.BudgetPolicy
	if err := kube.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: "sre-monthly"}, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.LastNotifiedMonth != "2026-11" || len(stored.Status.NotifiedThresholds) != 2 || stored.Status.NotifiedThresholds[0] != 80 || stored.Status.NotifiedThresholds[1] != 100 {
		t.Errorf("notification receipt = (%q, %#v), want (2026-11, [80 100])", stored.Status.LastNotifiedMonth, stored.Status.NotifiedThresholds)
	}
}

func TestRunBudgetPolicyDoesNotRecordThresholdWhenDeliveryFails(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly", Namespace: "costs"}, Spec: v1alpha1.BudgetPolicySpec{
		Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}, Thresholds: []int32{80},
	}}
	account := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs"}, Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "123"}}
	policy := &v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "central", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{
		Type: "teams", Events: []string{"BudgetThreshold"}, CredentialRef: corev1.LocalObjectReference{Name: "teams-hook"},
	}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "teams-hook", Namespace: "costs"}, Data: map[string][]byte{"TEAMS_WEBHOOK_URL": []byte("https://teams.example.test/hook")}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget, account, policy, secret).Build()
	reader := &analyzerTestReader{records: []normalize.CostRecord{{Provider: "aws", BillingAccountID: "123", Amount: "80", Currency: "USD", UsageStart: now.Add(-time.Hour)}}}
	deliveryErr := errors.New("delivery failed")
	delivery := &failingAnalyzerNotifier{failAt: 1, err: deliveryErr}
	factory := func(string) (notifier.Notifier, error) { return delivery, nil }

	_, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "sre-monthly", factory, now)
	if !errors.Is(err, deliveryErr) {
		t.Fatalf("RunBudgetPolicy() error = %v, want delivery error", err)
	}
	var stored v1alpha1.BudgetPolicy
	if err := kube.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: "sre-monthly"}, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.LastNotifiedMonth != "" || len(stored.Status.NotifiedThresholds) != 0 {
		t.Errorf("status = %#v, want no delivery receipt after failed send", stored.Status)
	}
}

func TestRunBudgetPolicySendsAndDeduplicatesDailyAnomaly(t *testing.T) {
	now := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly", Namespace: "costs"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: 1000, Currency: "USD"},
		DailyAnomaly: &v1alpha1.DailyAnomalySpec{AbsoluteIncreaseThreshold: 5},
	}}
	collected := metav1.NewTime(time.Date(2026, time.October, 2, 0, 5, 0, 0, time.UTC))
	account := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs"}, Spec: v1alpha1.CloudAccountSpec{
		Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "sre"}, Collection: v1alpha1.CollectionSpec{Enabled: true},
	}, Status: v1alpha1.CloudAccountStatus{LastSuccessfulCollectionTime: &collected}}
	policy := &v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "daily-teams", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{
		Type: "teams", Events: []string{"CostAnomaly"}, Selector: map[string]string{"team": "sre"}, CredentialRef: corev1.LocalObjectReference{Name: "teams-hook"},
	}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "teams-hook", Namespace: "costs"}, Data: map[string][]byte{"TEAMS_WEBHOOK_URL": []byte("https://teams.example.test/hook")}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget, account, policy, secret).Build()
	targetDay := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	delivery := &notifier.FakeNotifier{}
	factory := func(string) (notifier.Notifier, error) { return delivery, nil }

	for range 2 {
		if _, err := RunBudgetPolicy(context.Background(), kube, &analyzerTestReader{records: anomalyRecords(targetDay, "25.00")}, "costs", "sre-monthly", factory, now); err != nil {
			t.Fatalf("RunBudgetPolicy() error = %v", err)
		}
	}
	if len(delivery.Notifications) != 1 {
		t.Fatalf("notifications = %#v, want one daily anomaly notification after two runs", delivery.Notifications)
	}
	notification := delivery.Notifications[0]
	if notification.Type != "CostAnomaly" || notification.Details["Date"] != "2026-10-01" || notification.Details["Provider"] != "aws" {
		t.Errorf("daily notification = %#v, want CostAnomaly for aws on 2026-10-01", notification)
	}
	var stored v1alpha1.BudgetPolicy
	if err := kube.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: "sre-monthly"}, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.LastNotifiedAnomalyDate != "2026-10-01" {
		t.Errorf("LastNotifiedAnomalyDate = %q, want 2026-10-01", stored.Status.LastNotifiedAnomalyDate)
	}
}

func TestRunBudgetPolicyContinuesDailyAnomalyWhenBudgetHasNoSubscriber(t *testing.T) {
	now := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)
	targetDay := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "daily", Namespace: "costs"}, Spec: v1alpha1.BudgetPolicySpec{
		Amount: v1alpha1.BudgetAmount{Value: 1, Currency: "USD"}, Thresholds: []int32{80},
		DailyAnomaly: &v1alpha1.DailyAnomalySpec{AbsoluteIncreaseThreshold: 5},
	}}
	collected := metav1.NewTime(time.Date(2026, time.October, 2, 0, 5, 0, 0, time.UTC))
	account := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "costs"}, Spec: v1alpha1.CloudAccountSpec{
		Provider: "aws", AccountID: "123", Collection: v1alpha1.CollectionSpec{Enabled: true},
	}, Status: v1alpha1.CloudAccountStatus{LastSuccessfulCollectionTime: &collected}}
	policy := &v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "anomalies", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{
		Type: "teams", Events: []string{"CostAnomaly"}, CredentialRef: corev1.LocalObjectReference{Name: "hook"},
	}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "hook", Namespace: "costs"}, Data: map[string][]byte{"TEAMS_WEBHOOK_URL": []byte("https://teams.example.test/hook")}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget, account, policy, secret).Build()
	delivery := &notifier.FakeNotifier{}
	factory := func(string) (notifier.Notifier, error) { return delivery, nil }

	_, err := RunBudgetPolicy(context.Background(), kube, &analyzerTestReader{records: anomalyRecords(targetDay, "25.00")}, "costs", "daily", factory, now)
	if !errors.Is(err, ErrNotifierRequired) {
		t.Fatalf("RunBudgetPolicy() error = %v, want ErrNotifierRequired for BudgetThreshold", err)
	}
	if len(delivery.Notifications) != 1 || delivery.Notifications[0].Type != "CostAnomaly" {
		t.Fatalf("notifications = %#v, want one CostAnomaly despite missing BudgetThreshold subscriber", delivery.Notifications)
	}
}

func TestRunBudgetPolicyDeduplicatesDailyAnomaliesPerAccountDateAndDestination(t *testing.T) {
	now := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)
	targetDay := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "daily", Namespace: "costs"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: 1000, Currency: "USD"},
		DailyAnomaly: &v1alpha1.DailyAnomalySpec{AbsoluteIncreaseThreshold: 5},
	}}
	collected := metav1.NewTime(time.Date(2026, time.October, 2, 0, 5, 0, 0, time.UTC))
	aws := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs"}, Spec: v1alpha1.CloudAccountSpec{
		Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "sre"}, Collection: v1alpha1.CollectionSpec{Enabled: true},
	}, Status: v1alpha1.CloudAccountStatus{LastSuccessfulCollectionTime: &collected}}
	gcp := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "gcp-prod", Namespace: "costs"}, Spec: v1alpha1.CloudAccountSpec{
		Provider: "gcp", AccountID: "456", Metadata: map[string]string{"team": "sre"}, Collection: v1alpha1.CollectionSpec{Enabled: true},
	}, Status: v1alpha1.CloudAccountStatus{LastSuccessfulCollectionTime: &collected}}
	firstPolicy := &v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "first-teams", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{
		Type: "teams", Events: []string{"CostAnomaly"}, Selector: map[string]string{"team": "sre"}, CredentialRef: corev1.LocalObjectReference{Name: "first-hook"},
	}}
	firstSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "first-hook", Namespace: "costs"}, Data: map[string][]byte{"TEAMS_WEBHOOK_URL": []byte("https://first.example.test/hook")}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget, aws, gcp, firstPolicy, firstSecret).Build()
	firstDelivery := &notifier.FakeNotifier{}
	secondDelivery := &notifier.FakeNotifier{}
	factory := func(endpoint string) (notifier.Notifier, error) {
		if endpoint == "https://first.example.test/hook" {
			return firstDelivery, nil
		}
		return secondDelivery, nil
	}
	reader := &analyzerTestReader{records: anomalyRecords(targetDay, "25.00")}

	if _, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "daily", factory, now); err != nil {
		t.Fatalf("first RunBudgetPolicy() error = %v", err)
	}
	if len(firstDelivery.Notifications) != 1 {
		t.Fatalf("first destination notifications = %#v, want AWS anomaly", firstDelivery.Notifications)
	}
	secondPolicy := &v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "second-teams", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{
		Type: "teams", Events: []string{"CostAnomaly"}, Selector: map[string]string{"team": "sre"}, CredentialRef: corev1.LocalObjectReference{Name: "second-hook"},
	}}
	secondSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "second-hook", Namespace: "costs"}, Data: map[string][]byte{"TEAMS_WEBHOOK_URL": []byte("https://second.example.test/hook")}}
	if err := kube.Create(context.Background(), secondPolicy); err != nil {
		t.Fatal(err)
	}
	if err := kube.Create(context.Background(), secondSecret); err != nil {
		t.Fatal(err)
	}
	gcpRecords := anomalyRecords(targetDay, "30.00")
	for _, record := range gcpRecords {
		record.Provider = "gcp"
		record.BillingAccountID = "456"
		record.SourceRecordID = "gcp-" + record.SourceRecordID
		reader.records = append(reader.records, record)
	}
	if _, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "daily", factory, now); err != nil {
		t.Fatalf("second RunBudgetPolicy() error = %v", err)
	}
	if len(firstDelivery.Notifications) != 2 || firstDelivery.Notifications[1].Details["BillingAccountID"] != "456" {
		t.Errorf("first destination notifications = %#v, want AWS once and later GCP once", firstDelivery.Notifications)
	}
	if len(secondDelivery.Notifications) != 2 || secondDelivery.Notifications[0].Details["BillingAccountID"] != "123" || secondDelivery.Notifications[1].Details["BillingAccountID"] != "456" {
		t.Errorf("new destination notifications = %#v, want both account anomalies for the same date", secondDelivery.Notifications)
	}
}

func TestRunBudgetPolicySendsMonthlyBudgetForecast(t *testing.T) {
	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "forecast", Namespace: "costs"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: 250, Currency: "USD"},
		Forecast: &v1alpha1.BudgetForecastSpec{Enabled: true},
	}}
	account := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs"}, Spec: v1alpha1.CloudAccountSpec{
		Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "sre"},
	}}
	policy := &v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "forecast-teams", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{
		Type: "teams", Events: []string{"BudgetForecast"}, Selector: map[string]string{"team": "sre"}, CredentialRef: corev1.LocalObjectReference{Name: "teams-hook"},
	}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "teams-hook", Namespace: "costs"}, Data: map[string][]byte{"TEAMS_WEBHOOK_URL": []byte("https://teams.example.test/hook")}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget, account, policy, secret).Build()
	var records []normalize.CostRecord
	for day := 1; day <= 16; day++ {
		start := time.Date(2026, time.October, day, 0, 0, 0, 0, time.UTC)
		records = append(records, normalize.CostRecord{
			Provider: "aws", BillingAccountID: "123", CostBasis: provider.CostBasisNet,
			Amount: "10", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1),
		})
	}
	reader := &analyzerTestReader{records: records}
	delivery := &notifier.FakeNotifier{}
	factory := func(string) (notifier.Notifier, error) { return delivery, nil }

	for range 2 {
		if _, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "forecast", factory, now); err != nil {
			t.Fatalf("RunBudgetPolicy() error = %v", err)
		}
	}
	if len(delivery.Notifications) != 1 {
		t.Fatalf("notifications = %#v, want one forecast alert for a projected $310 month-end spend", delivery.Notifications)
	}
	notification := delivery.Notifications[0]
	if notification.Type != "BudgetForecast" || notification.Details["Month"] != "2026-10" || notification.Details["Spent"] != "160.00" || notification.Details["ProjectedSpend"] != "310.00" {
		t.Errorf("notification = %#v, want October forecast with spend 160.00 and projection 310.00", notification)
	}
	var stored v1alpha1.BudgetPolicy
	if err := kube.Get(context.Background(), types.NamespacedName{Namespace: "costs", Name: "forecast"}, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.LastNotifiedForecastMonth != "2026-10" {
		t.Errorf("LastNotifiedForecastMonth = %q, want 2026-10", stored.Status.LastNotifiedForecastMonth)
	}
}

func TestRunBudgetPolicyDoesNotRecordForecastAfterDeliveryFailure(t *testing.T) {
	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	budget := &v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "forecast", Namespace: "costs"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: 250, Currency: "USD"},
		Forecast: &v1alpha1.BudgetForecastSpec{Enabled: true},
	}}
	account := &v1alpha1.CloudAccount{ObjectMeta: metav1.ObjectMeta{Name: "aws-prod", Namespace: "costs"}, Spec: v1alpha1.CloudAccountSpec{
		Provider: "aws", AccountID: "123", Metadata: map[string]string{"team": "sre"},
	}}
	policy := &v1alpha1.NotificationPolicy{ObjectMeta: metav1.ObjectMeta{Name: "forecast-teams", Namespace: "costs"}, Spec: v1alpha1.NotificationPolicySpec{
		Type: "teams", Events: []string{"BudgetForecast"}, Selector: map[string]string{"team": "sre"}, CredentialRef: corev1.LocalObjectReference{Name: "teams-hook"},
	}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "teams-hook", Namespace: "costs"}, Data: map[string][]byte{"TEAMS_WEBHOOK_URL": []byte("https://teams.example.test/hook")}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(budget).WithObjects(budget, account, policy, secret).Build()
	var records []normalize.CostRecord
	for day := 1; day <= 16; day++ {
		start := time.Date(2026, time.October, day, 0, 0, 0, 0, time.UTC)
		records = append(records, normalize.CostRecord{Provider: "aws", BillingAccountID: "123", Amount: "10", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1)})
	}
	reader := &analyzerTestReader{records: records}
	deliveryErr := errors.New("forecast delivery failed")
	delivery := &failingAnalyzerNotifier{failAt: 1, err: deliveryErr}
	factory := func(string) (notifier.Notifier, error) { return delivery, nil }

	if _, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "forecast", factory, now); !errors.Is(err, deliveryErr) {
		t.Fatalf("RunBudgetPolicy() error = %v, want forecast delivery error", err)
	}
	var stored v1alpha1.BudgetPolicy
	key := types.NamespacedName{Namespace: "costs", Name: "forecast"}
	if err := kube.Get(context.Background(), key, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.LastNotifiedForecastMonth != "" {
		t.Fatalf("LastNotifiedForecastMonth = %q, want no receipt after failed delivery", stored.Status.LastNotifiedForecastMonth)
	}
	if _, err := RunBudgetPolicy(context.Background(), kube, reader, "costs", "forecast", factory, now); err != nil {
		t.Fatalf("RunBudgetPolicy() retry error = %v", err)
	}
	if len(delivery.attempts) != 2 {
		t.Fatalf("delivery attempts = %d, want failure followed by successful retry", len(delivery.attempts))
	}
	if err := kube.Get(context.Background(), key, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.LastNotifiedForecastMonth != "2026-10" {
		t.Errorf("LastNotifiedForecastMonth = %q after retry, want 2026-10", stored.Status.LastNotifiedForecastMonth)
	}
}
