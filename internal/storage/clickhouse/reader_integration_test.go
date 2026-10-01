package clickhouse

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/internal/provider"
)

func TestNativeReaderReturnsNormalizedFinalRecord(t *testing.T) {
	if os.Getenv("CLICKHOUSE_INTEGRATION") != "1" {
		t.Skip("ClickHouse integration environment is not enabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := OpenFromEnv(ctx)
	if err != nil {
		t.Fatalf("OpenFromEnv() error = %v", err)
	}
	defer store.Close()

	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	records, err := store.ReadCosts(ctx, []AccountScope{{Provider: "aws", BillingAccountID: "synthetic-kind-account"}}, start, end)
	if err != nil {
		t.Fatalf("ReadCosts() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ReadCosts() returned %d records, want one logical row", len(records))
	}
	record := records[0]
	if record.Provider != "aws" || record.BillingAccountID != "synthetic-kind-account" || record.SourceRecordID != "fixture-aws-001" || record.CostBasis != provider.CostBasisNet || record.Amount != "12.34" || record.Currency != "USD" || !record.UsageStart.Equal(start) || !record.UsageEnd.Equal(end) {
		t.Errorf("ReadCosts() record = %#v, want normalized AWS fixture values", record)
	}
}
