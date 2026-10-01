package analyzer

import (
	"reflect"
	"testing"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
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
