package analyzer

import (
	"sort"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type dataFreshnessState string

const (
	dataFreshnessFresh   dataFreshnessState = "Fresh"
	dataFreshnessStale   dataFreshnessState = "Stale"
	dataFreshnessUnknown dataFreshnessState = "Unknown"
)

type dataFreshnessAssessment struct {
	state  dataFreshnessState
	reason string
	run    *storage.CollectionRun
}

func excludeStaleAccounts(accounts []v1alpha1.CloudAccount, runs []storage.CollectionRun, now time.Time) []v1alpha1.CloudAccount {
	byAccount := make(map[accountKey][]storage.CollectionRun, len(runs))
	for _, run := range runs {
		key := accountKey{provider: run.Provider, accountID: run.BillingAccountID}
		byAccount[key] = append(byAccount[key], run)
	}
	filtered := make([]v1alpha1.CloudAccount, 0, len(accounts))
	for _, account := range accounts {
		key := accountKey{provider: account.Spec.Provider, accountID: account.Spec.AccountID}
		if assessDataFreshness(account.Spec.Provider, byAccount[key], now).state != dataFreshnessStale {
			filtered = append(filtered, account)
		}
	}
	return filtered
}

func assessDataFreshness(providerName string, runs []storage.CollectionRun, now time.Time) dataFreshnessAssessment {
	if len(runs) == 0 {
		return dataFreshnessAssessment{state: dataFreshnessUnknown, reason: "NoSuccessfulCollectionMetadata"}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CompletedAt.After(runs[j].CompletedAt) })
	run := runs[0]
	assessment := dataFreshnessAssessment{state: dataFreshnessUnknown, run: &run}
	if run.Provider == "" || run.BillingAccountID == "" || run.WindowStart.IsZero() || run.WindowEnd.IsZero() ||
		run.StartedAt.IsZero() || run.CompletedAt.IsZero() || run.DataIngestedAt.IsZero() || run.WindowEnd.Before(run.WindowStart) ||
		run.CompletedAt.Before(run.StartedAt) {
		assessment.reason = "InvalidCollectionRunMetadata"
		return assessment
	}
	delay, bounded := expectedFreshnessDelay(providerName)
	if !bounded {
		assessment.reason = "ProviderFreshnessUnbounded"
		return assessment
	}
	if run.RecordCount == 0 || run.LatestUsageEnd.IsZero() {
		assessment.reason = "NoUsagePeriodObserved"
		return assessment
	}
	if now.UTC().Sub(run.LatestUsageEnd.UTC()) > delay {
		assessment.state = dataFreshnessStale
		assessment.reason = "ProviderDataBeyondExpectedDelay"
		return assessment
	}
	assessment.state = dataFreshnessFresh
	assessment.reason = "ProviderDataWithinExpectedDelay"
	return assessment
}

func freshnessConditionStatus(state dataFreshnessState) metav1.ConditionStatus {
	switch state {
	case dataFreshnessFresh:
		return metav1.ConditionTrue
	case dataFreshnessStale:
		return metav1.ConditionFalse
	default:
		return metav1.ConditionUnknown
	}
}

func expectedFreshnessDelay(providerName string) (time.Duration, bool) {
	return provider.ExpectedDailyCostDataDelay(providerName)
}
