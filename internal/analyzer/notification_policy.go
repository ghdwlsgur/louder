package analyzer

import (
	"sort"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
)

func SelectNotificationPolicies(eventType string, policies []v1alpha1.NotificationPolicy, accounts []v1alpha1.CloudAccount) []v1alpha1.NotificationPolicy {
	selected := make([]v1alpha1.NotificationPolicy, 0, len(policies))
	for _, policy := range policies {
		if policy.Spec.Type != "teams" || !subscribesToEvent(policy.Spec.Events, eventType) || !matchesAnyAccount(policy.Spec.Selector, accounts) {
			continue
		}
		selected = append(selected, policy)
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].Name < selected[j].Name
	})
	return selected
}

func subscribesToEvent(events []string, eventType string) bool {
	for _, event := range events {
		if event == eventType {
			return true
		}
	}
	return false
}

func matchesAnyAccount(selector map[string]string, accounts []v1alpha1.CloudAccount) bool {
	for _, account := range accounts {
		if matchesSelector(account.Spec.Metadata, selector) {
			return true
		}
	}
	return false
}
