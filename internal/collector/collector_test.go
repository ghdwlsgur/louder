package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/provider"
)

func TestPreviousSevenCompleteUTCDays(t *testing.T) {
	now := time.Date(2026, time.October, 1, 3, 15, 0, 0, time.FixedZone("UTC-7", -7*60*60))
	start, end := PreviousSevenCompleteUTCDays(now)
	wantStart := time.Date(2026, time.September, 24, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("PreviousSevenCompleteUTCDays() = (%s, %s), want (%s, %s)", start, end, wantStart, wantEnd)
	}
}

func TestRunWithRegistryCollectsPreviousSevenCompleteUTCDays(t *testing.T) {
	fake := &collectorTestProvider{records: []provider.RawCostRecord{{
		Provider:       "aws",
		SourceRecordID: "aws-cost-explorer-123456789012-2026-09-30",
		BillingScope:   "123456789012",
		CostBasis:      provider.CostBasisUnblended,
		Amount:         "12.34",
		Currency:       "USD",
		UsageStart:     time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC),
		UsageEnd:       time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
	}}}
	registry := provider.NewRegistry()
	if err := registry.Register("aws", func() (provider.Provider, error) { return fake, nil }); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 1, 3, 15, 0, 0, time.FixedZone("UTC-7", -7*60*60))
	var output bytes.Buffer
	if err := RunWithRegistry(context.Background(), registry, "aws", "123456789012", now, &output); err != nil {
		t.Fatalf("RunWithRegistry() error = %v", err)
	}
	wantStart := time.Date(2026, time.September, 24, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	if fake.request.AccountID != "123456789012" || !fake.request.StartTime.Equal(wantStart) || !fake.request.EndTime.Equal(wantEnd) {
		t.Errorf("CollectRequest = %#v, want account and previous seven complete UTC days", fake.request)
	}
	if !fake.validated || !fake.collected {
		t.Errorf("provider calls: validated=%t collected=%t, want both true", fake.validated, fake.collected)
	}
	if output.Len() == 0 || output.Bytes()[output.Len()-1] != '\n' {
		t.Errorf("output = %q, want JSON Lines record", output.String())
	}
}

func TestRunEmitsFixtureRecordsAsJSONLines(t *testing.T) {
	var output bytes.Buffer
	if err := Run(context.Background(), "aws", "synthetic-account", "embedded:aws", &output); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var record provider.RawCostRecord
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("output is not a JSON record: %v", err)
	}
	if record.Provider != "aws" || record.BillingScope != "synthetic-account" || record.SourceRecordID == "" || record.CostBasis != provider.CostBasisUnblended {
		t.Errorf("record = %#v, want AWS fixture record scoped to synthetic-account", record)
	}
	if output.Bytes()[output.Len()-1] != '\n' {
		t.Error("JSON Lines output must end with a newline")
	}
}

func TestRunWithRegistryAndStorageCollectsPreviousSevenCompleteUTCDays(t *testing.T) {
	fake := &collectorTestProvider{records: []provider.RawCostRecord{{
		Provider:       "aws",
		SourceRecordID: "aws-cost-explorer-123456789012-2026-09-30",
		BillingScope:   "123456789012",
		CostBasis:      provider.CostBasisUnblended,
		Amount:         "12.34",
		Currency:       "USD",
		UsageStart:     time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC),
		UsageEnd:       time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
	}}}
	registry := provider.NewRegistry()
	if err := registry.Register("aws", func() (provider.Provider, error) { return fake, nil }); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 1, 3, 15, 0, 0, time.FixedZone("UTC-7", -7*60*60))
	if err := RunWithRegistryAndStorage(context.Background(), registry, "aws", "123456789012", now, &bytes.Buffer{}, &collectorTestWriter{}); err != nil {
		t.Fatalf("RunWithRegistryAndStorage() error = %v", err)
	}
	wantStart := time.Date(2026, time.September, 24, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	if !fake.request.StartTime.Equal(wantStart) || !fake.request.EndTime.Equal(wantEnd) {
		t.Errorf("CollectRequest window = (%s, %s), want seven complete UTC days (%s, %s)", fake.request.StartTime, fake.request.EndTime, wantStart, wantEnd)
	}
}

func TestRunWithStoragePersistsNormalizedAWSFixture(t *testing.T) {
	storage := &collectorTestWriter{}
	var output bytes.Buffer
	if err := RunWithStorage(context.Background(), "aws", "synthetic-account", "embedded:aws", &output, storage); err != nil {
		t.Fatalf("RunWithStorage() error = %v", err)
	}
	if len(storage.records) != 1 {
		t.Fatalf("stored records = %#v, want one normalized record", storage.records)
	}
	want := normalize.CostRecord{
		Provider:         "aws",
		BillingAccountID: "synthetic-account",
		SourceRecordID:   "fixture-aws-001",
		CostBasis:        provider.CostBasisUnblended,
		Amount:           "12.34",
		Currency:         "USD",
		UsageStart:       time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		UsageEnd:         time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC),
	}
	if storage.records[0] != want {
		t.Errorf("stored record = %#v, want %#v", storage.records[0], want)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"sourceRecordId":"fixture-aws-001"`)) {
		t.Errorf("stdout = %q, want raw JSON Lines output", output.String())
	}
}

func TestRunWithStorageDoesNotEmitRawRecordsWhenStorageFails(t *testing.T) {
	storage := &collectorTestWriter{err: errors.New("storage unavailable")}
	var output bytes.Buffer
	err := RunWithStorage(context.Background(), "aws", "synthetic-account", "embedded:aws", &output, storage)
	if err == nil {
		t.Fatal("RunWithStorage() error = nil, want storage error")
	}
	if output.Len() != 0 {
		t.Errorf("stdout = %q, want no records when persistence fails", output.String())
	}
}

func TestRunRejectsFixtureForDifferentProvider(t *testing.T) {
	var output bytes.Buffer
	if err := Run(context.Background(), "aws", "synthetic-account", "embedded:gcp", &output); err == nil {
		t.Fatal("Run() error = nil, want provider/fixture mismatch error")
	}
	if output.Len() != 0 {
		t.Errorf("output = %q, want no partial output on fixture mismatch", output.String())
	}
}

func TestRunSupportsInitialProviderFixtures(t *testing.T) {
	for _, providerName := range []string{"aws", "gcp", "azure"} {
		t.Run(providerName, func(t *testing.T) {
			var output bytes.Buffer
			if err := Run(context.Background(), providerName, "synthetic-account", "embedded:"+providerName, &output); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if output.Len() == 0 {
				t.Fatal("Run() output is empty")
			}
		})
	}
}

func TestRunDoesNotFallBackToSyntheticFixture(t *testing.T) {
	var output bytes.Buffer
	if err := Run(context.Background(), "aws", "synthetic-account", "", &output); err == nil {
		t.Fatal("Run() error = nil without an explicit fixture")
	}
	if output.Len() != 0 {
		t.Errorf("output = %q, want no synthetic data without explicit fixture mode", output.String())
	}
}

func TestDecodeFixtureRejectsMalformedJSON(t *testing.T) {
	if _, err := decodeFixture([]byte(`[{"provider":`), "aws", "synthetic-account"); err == nil {
		t.Fatal("decodeFixture() error = nil for malformed JSON")
	}
}

type collectorTestProvider struct {
	validated   bool
	collected   bool
	validateErr error
	collectErr  error
	request     provider.CollectRequest
	records     []provider.RawCostRecord
}

type collectorTestWriter struct {
	records []normalize.CostRecord
	err     error
}

func (w *collectorTestWriter) WriteCosts(_ context.Context, records []normalize.CostRecord) error {
	w.records = append(w.records, records...)
	return w.err
}

func (p *collectorTestProvider) ValidateCredentials(context.Context) error {
	p.validated = true
	return p.validateErr
}

func (p *collectorTestProvider) CollectCosts(_ context.Context, request provider.CollectRequest) ([]provider.RawCostRecord, error) {
	p.collected = true
	p.request = request
	return p.records, p.collectErr
}

func (*collectorTestProvider) Metadata(context.Context) provider.ProviderMetadata {
	return provider.ProviderMetadata{Name: "aws"}
}
