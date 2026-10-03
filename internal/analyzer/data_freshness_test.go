package analyzer

import (
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/api/v1alpha1"
	"github.com/ghdwlsgur/louder/internal/storage"
)

func TestAssessDataFreshnessUsesBillingPeriodInsteadOfJobCompletion(t *testing.T) {
	now := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)
	run := storage.CollectionRun{
		Provider: "aws", BillingAccountID: "123", WindowStart: now.AddDate(0, 0, -8), WindowEnd: now.Truncate(24 * time.Hour),
		StartedAt: now.Add(-time.Minute), CompletedAt: now, DataIngestedAt: now,
		RecordCount: 8, LatestUsageEnd: now.Truncate(24 * time.Hour),
	}
	assessment := assessDataFreshness("aws", []storage.CollectionRun{run}, now)
	if assessment.state != dataFreshnessFresh || assessment.reason != "ProviderDataWithinExpectedDelay" {
		t.Fatalf("assessment = %#v, want Fresh from recent billing usage coverage", assessment)
	}
	if assessment.run == nil || assessment.run.CompletedAt != now {
		t.Fatalf("assessment run = %#v, want the successful run metadata", assessment.run)
	}
}

func TestExcludeStaleAccountsKeepsFreshAndUnknownAccounts(t *testing.T) {
	now := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
	base := storage.CollectionRun{WindowStart: now.AddDate(0, 0, -8), WindowEnd: now, StartedAt: now.Add(-time.Hour), CompletedAt: now, DataIngestedAt: now, RecordCount: 8}
	stale := base
	stale.Provider, stale.BillingAccountID, stale.LatestUsageEnd = "aws", "stale", now.Add(-48*time.Hour)
	fresh := base
	fresh.Provider, fresh.BillingAccountID, fresh.LatestUsageEnd = "aws", "fresh", now
	accounts := []v1alpha1.CloudAccount{
		{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "stale"}},
		{Spec: v1alpha1.CloudAccountSpec{Provider: "aws", AccountID: "fresh"}},
		{Spec: v1alpha1.CloudAccountSpec{Provider: "oci", AccountID: "unknown"}},
	}
	got := excludeStaleAccounts(accounts, []storage.CollectionRun{stale, fresh}, now)
	if len(got) != 2 || got[0].Spec.AccountID != "fresh" || got[1].Spec.AccountID != "unknown" {
		t.Fatalf("accounts = %#v, want fresh and unknown accounts", got)
	}
}

func TestAssessDataFreshnessMarksRecentJobWithOldUsageDataStale(t *testing.T) {
	now := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
	run := storage.CollectionRun{
		Provider: "aws", BillingAccountID: "123", WindowStart: now.AddDate(0, 0, -8), WindowEnd: now.Truncate(24 * time.Hour),
		StartedAt: now.Add(-time.Minute), CompletedAt: now, DataIngestedAt: now,
		RecordCount: 8, LatestUsageEnd: now.Add(-48 * time.Hour),
	}
	assessment := assessDataFreshness("aws", []storage.CollectionRun{run}, now)
	if assessment.state != dataFreshnessStale || assessment.reason != "ProviderDataBeyondExpectedDelay" {
		t.Fatalf("assessment = %#v, want Stale despite recent Job completion and ingestion", assessment)
	}
}

func TestAssessDataFreshnessReportsUnknownWithoutProviderBoundOrUsagePeriod(t *testing.T) {
	now := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
	validRun := storage.CollectionRun{
		Provider: "oci", BillingAccountID: "123", WindowStart: now.AddDate(0, 0, -8), WindowEnd: now.Truncate(24 * time.Hour),
		StartedAt: now.Add(-time.Minute), CompletedAt: now, DataIngestedAt: now,
		RecordCount: 8, LatestUsageEnd: now.Truncate(24 * time.Hour),
	}
	emptyRun := validRun
	emptyRun.Provider = "aws"
	emptyRun.RecordCount = 0
	emptyRun.LatestUsageEnd = time.Time{}
	for _, test := range []struct {
		name     string
		provider string
		runs     []storage.CollectionRun
		reason   string
	}{
		{"provider has no documented delay", "oci", []storage.CollectionRun{validRun}, "ProviderFreshnessUnbounded"},
		{"no usage period returned", "aws", []storage.CollectionRun{emptyRun}, "NoUsagePeriodObserved"},
		{"no collection run", "aws", nil, "NoSuccessfulCollectionMetadata"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assessment := assessDataFreshness(test.provider, test.runs, now)
			if assessment.state != dataFreshnessUnknown || assessment.reason != test.reason {
				t.Fatalf("assessment = %#v, want Unknown/%s", assessment, test.reason)
			}
		})
	}
}
