package clickhouse

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/internal/normalize"
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

func TestNativeReaderReturnsMonthlyNCPInvoicePeriod(t *testing.T) {
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

	monthStart := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)
	record := normalize.CostRecord{
		Provider: "ncp", BillingAccountID: "synthetic-ncp-account", SourceRecordID: "ncp-monthly-synthetic-ncp-account-2026-10-KRW",
		CostBasis: provider.CostBasisNCPMonthly, Amount: "12345.67", Currency: "KRW", UsageStart: monthStart, UsageEnd: monthEnd,
	}
	if err := store.WriteCosts(ctx, []normalize.CostRecord{record}); err != nil {
		t.Fatalf("WriteCosts() error = %v", err)
	}
	readEnd := time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)
	records, err := store.ReadCosts(ctx, []AccountScope{{Provider: "ncp", BillingAccountID: "synthetic-ncp-account"}}, monthStart, readEnd)
	if err != nil {
		t.Fatalf("ReadCosts() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ReadCosts() returned %d records, want one NCP monthly row", len(records))
	}
	if records[0] != record {
		t.Errorf("ReadCosts() record = %#v, want %#v", records[0], record)
	}
}
