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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
