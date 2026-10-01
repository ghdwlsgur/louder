package normalize

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/internal/provider"
)

func TestNormalizePreservesAWSNetDailyCost(t *testing.T) {
	start := time.Date(2026, time.September, 30, 9, 0, 0, 0, time.FixedZone("UTC+9", 9*60*60))
	end := start.Add(24 * time.Hour)
	raw := provider.RawCostRecord{
		Provider:       "aws",
		SourceRecordID: "aws-cost-explorer-123456789012-2026-09-30",
		BillingScope:   "123456789012",
		CostBasis:      provider.CostBasisNet,
		Amount:         "12.3400",
		Currency:       "USD",
		UsageStart:     start,
		UsageEnd:       end,
	}

	record, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	want := CostRecord{
		Provider:         "aws",
		BillingAccountID: "123456789012",
		SourceRecordID:   raw.SourceRecordID,
		CostBasis:        provider.CostBasisNet,
		Amount:           "12.3400",
		Currency:         "USD",
		UsageStart:       time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC),
		UsageEnd:         time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
	}
	if record != want {
		t.Errorf("Normalize() = %#v, want %#v", record, want)
	}
	gotJSON, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	wantJSON, err := os.ReadFile("testdata/aws-net-daily.json")
	if err != nil {
		t.Fatalf("read normalized golden: %v", err)
	}
	var compactWant bytes.Buffer
	if err := json.Compact(&compactWant, wantJSON); err != nil {
		t.Fatalf("compact normalized golden: %v", err)
	}
	if !bytes.Equal(gotJSON, compactWant.Bytes()) {
		t.Errorf("normalized JSON = %s, want %s", gotJSON, compactWant.Bytes())
	}
}

func TestNormalizePreservesGCPBigQueryNetDailyCost(t *testing.T) {
	start := time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC)
	raw := provider.RawCostRecord{Provider: "gcp", SourceRecordID: "gcp-bigquery-ABC-2026-09-29-USD", BillingScope: "ABC", CostBasis: provider.CostBasisNet, Amount: "1.234567", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1)}
	got, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	want := CostRecord{Provider: "gcp", BillingAccountID: "ABC", SourceRecordID: raw.SourceRecordID, CostBasis: provider.CostBasisNet, Amount: "1.234567", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1)}
	if got != want {
		t.Errorf("Normalize() = %#v, want %#v", got, want)
	}
}

func TestNormalizeRejectsMissingCostBasis(t *testing.T) {
	raw := validRawRecord()
	raw.CostBasis = ""
	if record, err := Normalize(raw); !errors.Is(err, ErrInvalidCostRecord) {
		t.Fatalf("Normalize() = (%#v, nil), want missing-cost-basis error", record)
	}
}

func TestNormalizeRejectsMalformedAmount(t *testing.T) {
	raw := validRawRecord()
	raw.Amount = "twelve dollars"
	if record, err := Normalize(raw); !errors.Is(err, ErrInvalidCostRecord) {
		t.Fatalf("Normalize() = (%#v, nil), want malformed-amount error", record)
	}
}

func TestNormalizeRejectsInvalidCurrency(t *testing.T) {
	raw := validRawRecord()
	raw.Currency = "usd"
	if record, err := Normalize(raw); !errors.Is(err, ErrInvalidCostRecord) {
		t.Fatalf("Normalize() = (%#v, nil), want invalid-currency error", record)
	}
}

func TestNormalizeRejectsEmptyUsageInterval(t *testing.T) {
	raw := validRawRecord()
	raw.UsageEnd = raw.UsageStart
	if record, err := Normalize(raw); !errors.Is(err, ErrInvalidCostRecord) {
		t.Fatalf("Normalize() = (%#v, nil), want invalid-interval error", record)
	}
}

func validRawRecord() provider.RawCostRecord {
	return provider.RawCostRecord{
		Provider:       "aws",
		SourceRecordID: "aws-cost-explorer-123456789012-2026-09-30",
		BillingScope:   "123456789012",
		CostBasis:      provider.CostBasisUnblended,
		Amount:         "12.3400",
		Currency:       "USD",
		UsageStart:     time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC),
		UsageEnd:       time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
	}
}
