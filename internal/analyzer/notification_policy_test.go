package analyzer

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/notifier"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSelectNotificationPoliciesMatchesSubscribedEventAndAccountMetadata(t *testing.T) {
	policies := []v1alpha1.NotificationPolicy{{
		ObjectMeta: metav1.ObjectMeta{Name: "sre-teams"},
		Spec: v1alpha1.NotificationPolicySpec{
			Type: "teams", Events: []string{"BudgetThreshold"}, Selector: map[string]string{"team": "sre"},
		},
	}}
	accounts := []v1alpha1.CloudAccount{{
		Spec: v1alpha1.CloudAccountSpec{Metadata: map[string]string{"team": "sre"}},
	}}

	got := SelectNotificationPolicies("BudgetThreshold", policies, accounts)
	if !reflect.DeepEqual(got, policies) {
		t.Fatalf("SelectNotificationPolicies() = %#v, want %#v", got, policies)
	}
}

func TestSelectNotificationPoliciesSkipsUnsubscribedEvent(t *testing.T) {
	policies := []v1alpha1.NotificationPolicy{{
		ObjectMeta: metav1.ObjectMeta{Name: "sre-teams"},
		Spec:       v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"DailySummary"}},
	}}
	accounts := []v1alpha1.CloudAccount{{}}

	got := SelectNotificationPolicies("BudgetThreshold", policies, accounts)
	if len(got) != 0 {
		t.Fatalf("SelectNotificationPolicies() = %#v, want no policy", got)
	}
}

func TestSelectNotificationPoliciesSkipsUnmatchedAccountSelector(t *testing.T) {
	policies := []v1alpha1.NotificationPolicy{{
		ObjectMeta: metav1.ObjectMeta{Name: "sre-teams"},
		Spec: v1alpha1.NotificationPolicySpec{
			Type: "teams", Events: []string{"BudgetThreshold"}, Selector: map[string]string{"team": "sre"},
		},
	}}
	accounts := []v1alpha1.CloudAccount{{
		Spec: v1alpha1.CloudAccountSpec{Metadata: map[string]string{"team": "engineering"}},
	}}

	got := SelectNotificationPolicies("BudgetThreshold", policies, accounts)
	if len(got) != 0 {
		t.Fatalf("SelectNotificationPolicies() = %#v, want no policy", got)
	}
}

func TestSelectNotificationPoliciesEmptySelectorMatchesAnyAccount(t *testing.T) {
	policies := []v1alpha1.NotificationPolicy{{
		ObjectMeta: metav1.ObjectMeta{Name: "central-teams"},
		Spec:       v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"BudgetThreshold"}},
	}}
	accounts := []v1alpha1.CloudAccount{{}}

	got := SelectNotificationPolicies("BudgetThreshold", policies, accounts)
	if !reflect.DeepEqual(got, policies) {
		t.Fatalf("SelectNotificationPolicies() = %#v, want %#v", got, policies)
	}
}

func TestSelectNotificationPoliciesRequiresRelevantAccount(t *testing.T) {
	policies := []v1alpha1.NotificationPolicy{{
		ObjectMeta: metav1.ObjectMeta{Name: "central-teams"},
		Spec:       v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"BudgetThreshold"}},
	}}

	got := SelectNotificationPolicies("BudgetThreshold", policies, nil)
	if len(got) != 0 {
		t.Fatalf("SelectNotificationPolicies() = %#v, want no policy", got)
	}
}

func TestSelectNotificationPoliciesReturnsPoliciesByName(t *testing.T) {
	policies := []v1alpha1.NotificationPolicy{
		{ObjectMeta: metav1.ObjectMeta{Name: "zulu"}, Spec: v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"BudgetThreshold"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "alpha"}, Spec: v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"BudgetThreshold"}}},
	}
	accounts := []v1alpha1.CloudAccount{{}}

	got := SelectNotificationPolicies("BudgetThreshold", policies, accounts)
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zulu" {
		t.Fatalf("selected policy order = %#v, want [alpha zulu]", got)
	}
}

func TestEvaluateAndNotifyBudgetPoliciesSendsToMatchingDestinations(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	budget := v1alpha1.BudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: "sre-monthly"}, Spec: v1alpha1.BudgetPolicySpec{
		Selector: map[string]string{"team": "sre"}, Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}, Thresholds: []int32{80},
	}}
	accounts := []v1alpha1.CloudAccount{
		{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "sre", Metadata: map[string]string{"team": "sre"}}},
		{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "engineering", Metadata: map[string]string{"team": "engineering"}}},
	}
	reader := &analyzerTestReader{records: []normalize.CostRecord{{Provider: "aws", BillingAccountID: "sre", Amount: "80", Currency: "USD", UsageStart: now.Add(-time.Hour)}}}
	policies := []v1alpha1.NotificationPolicy{
		{ObjectMeta: metav1.ObjectMeta{Name: "sre-destination"}, Spec: v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"BudgetThreshold"}, Selector: map[string]string{"team": "sre"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "engineering-destination"}, Spec: v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"BudgetThreshold"}, Selector: map[string]string{"team": "engineering"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "global-destination"}, Spec: v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"BudgetThreshold"}}},
	}
	deliveries := map[string]*notifier.FakeNotifier{"sre-destination": {}, "engineering-destination": {}, "global-destination": {}}
	var resolvedNames []string
	resolve := func(_ context.Context, policy v1alpha1.NotificationPolicy) (notifier.Notifier, error) {
		resolvedNames = append(resolvedNames, policy.Name)
		return deliveries[policy.Name], nil
	}

	intents, err := EvaluateAndNotifyBudgetPolicies(context.Background(), reader, budget, accounts, policies, resolve, now)
	if err != nil {
		t.Fatalf("EvaluateAndNotifyBudgetPolicies() error = %v", err)
	}
	if len(intents) != 1 || len(deliveries["sre-destination"].Notifications) != 1 || len(deliveries["global-destination"].Notifications) != 1 || len(deliveries["engineering-destination"].Notifications) != 0 {
		t.Fatalf("intents = %#v, SRE deliveries = %#v, global deliveries = %#v, engineering deliveries = %#v; want SRE and global deliveries only", intents, deliveries["sre-destination"].Notifications, deliveries["global-destination"].Notifications, deliveries["engineering-destination"].Notifications)
	}
	if !reflect.DeepEqual(resolvedNames, []string{"global-destination", "sre-destination"}) {
		t.Fatalf("resolved policies = %#v, want deterministic name order", resolvedNames)
	}
}

func TestEvaluateAndNotifyBudgetPoliciesDoesNotResolveBelowThreshold(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	budget := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}, Thresholds: []int32{80}}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "sre"}}}
	reader := &analyzerTestReader{records: []normalize.CostRecord{{Provider: "aws", BillingAccountID: "sre", Amount: "79", Currency: "USD", UsageStart: now.Add(-time.Hour)}}}
	resolved := false
	resolve := func(context.Context, v1alpha1.NotificationPolicy) (notifier.Notifier, error) {
		resolved = true
		return &notifier.FakeNotifier{}, nil
	}

	intents, err := EvaluateAndNotifyBudgetPolicies(context.Background(), reader, budget, accounts, nil, resolve, now)
	if err != nil || resolved || len(intents) != 0 {
		t.Fatalf("intents = %#v, error = %v, resolver called = %t; want no intents, no error, and no resolution", intents, err, resolved)
	}
}

func TestEvaluateAndNotifyBudgetPoliciesRequiresMatchingDestination(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	budget := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}, Thresholds: []int32{80}}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "sre"}}}
	reader := &analyzerTestReader{records: []normalize.CostRecord{{Provider: "aws", BillingAccountID: "sre", Amount: "80", Currency: "USD", UsageStart: now.Add(-time.Hour)}}}
	resolve := func(context.Context, v1alpha1.NotificationPolicy) (notifier.Notifier, error) {
		return &notifier.FakeNotifier{}, nil
	}

	intents, err := EvaluateAndNotifyBudgetPolicies(context.Background(), reader, budget, accounts, nil, resolve, now)
	if !errors.Is(err, ErrNotifierRequired) {
		t.Fatalf("EvaluateAndNotifyBudgetPolicies() error = %v, want ErrNotifierRequired", err)
	}
	if len(intents) != 1 {
		t.Fatalf("intents = %#v, want reached threshold returned alongside routing error", intents)
	}
}

func TestEvaluateAndNotifyBudgetPoliciesSanitizesResolverErrors(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	budget := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}, Thresholds: []int32{80}}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "sre"}}}
	reader := &analyzerTestReader{records: []normalize.CostRecord{{Provider: "aws", BillingAccountID: "sre", Amount: "80", Currency: "USD", UsageStart: now.Add(-time.Hour)}}}
	policies := []v1alpha1.NotificationPolicy{{ObjectMeta: metav1.ObjectMeta{Name: "central"}, Spec: v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"BudgetThreshold"}}}}
	resolve := func(context.Context, v1alpha1.NotificationPolicy) (notifier.Notifier, error) {
		return nil, errors.New("webhook-secret-do-not-show")
	}

	_, err := EvaluateAndNotifyBudgetPolicies(context.Background(), reader, budget, accounts, policies, resolve, now)
	if !errors.Is(err, ErrNotificationPolicyResolution) {
		t.Fatalf("EvaluateAndNotifyBudgetPolicies() error = %v, want ErrNotificationPolicyResolution", err)
	}
	if strings.Contains(err.Error(), "webhook-secret-do-not-show") {
		t.Fatalf("resolution error leaked its cause: %v", err)
	}
}

func TestEvaluateAndNotifyBudgetPoliciesStopsAfterDeliveryFailure(t *testing.T) {
	now := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	budget := v1alpha1.BudgetPolicy{Spec: v1alpha1.BudgetPolicySpec{Amount: v1alpha1.BudgetAmount{Value: 100, Currency: "USD"}, Thresholds: []int32{80, 100}}}
	accounts := []v1alpha1.CloudAccount{{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "sre"}}}
	reader := &analyzerTestReader{records: []normalize.CostRecord{{Provider: "aws", BillingAccountID: "sre", Amount: "100", Currency: "USD", UsageStart: now.Add(-time.Hour)}}}
	policies := []v1alpha1.NotificationPolicy{{ObjectMeta: metav1.ObjectMeta{Name: "central"}, Spec: v1alpha1.NotificationPolicySpec{Type: "teams", Events: []string{"BudgetThreshold"}}}}
	delivery := &failingAnalyzerNotifier{failAt: 2, err: errors.New("delivery failed")}
	resolve := func(context.Context, v1alpha1.NotificationPolicy) (notifier.Notifier, error) { return delivery, nil }

	intents, err := EvaluateAndNotifyBudgetPolicies(context.Background(), reader, budget, accounts, policies, resolve, now)
	if err == nil || !errors.Is(err, delivery.err) {
		t.Fatalf("EvaluateAndNotifyBudgetPolicies() error = %v, want delivery error", err)
	}
	if len(intents) != 2 || len(delivery.attempts) != 2 {
		t.Fatalf("intents = %d, delivery attempts = %d; want two intents and stop at second failed attempt", len(intents), len(delivery.attempts))
	}
}
