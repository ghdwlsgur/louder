package clickhouse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/storage"
)

func TestWriteCostsWritesOneBatchWithUTCIngestionTime(t *testing.T) {
	insertedAt := time.Date(2026, time.October, 1, 3, 0, 0, 0, time.FixedZone("UTC+9", 9*60*60))
	inserter := &fakeInserter{}
	store := New(inserter, func() time.Time { return insertedAt })
	want := []normalize.CostRecord{{
		Provider:         "aws",
		BillingAccountID: "123456789012",
		SourceRecordID:   "aws-cost-explorer-123456789012-2026-09-30",
		CostBasis:        "net_cost",
		Amount:           "12.3400",
		Currency:         "USD",
		UsageStart:       time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC),
		UsageEnd:         time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
	}}

	if err := store.WriteCosts(context.Background(), want); err != nil {
		t.Fatalf("WriteCosts() error = %v", err)
	}
	if len(inserter.batches) != 1 {
		t.Fatalf("insert batches = %d, want 1", len(inserter.batches))
	}
	if len(inserter.batches[0]) != 1 || inserter.batches[0][0] != want[0] {
		t.Errorf("inserted records = %#v, want %#v", inserter.batches[0], want)
	}
	if !inserter.insertedAt.Equal(insertedAt.UTC()) {
		t.Errorf("ingested_at = %s, want %s", inserter.insertedAt, insertedAt.UTC())
	}
}

func TestWriteCollectionRunPersistsExactWindowAndSourcePeriod(t *testing.T) {
	writer := &fakeCollectionRunInserter{}
	ingestedAt := time.Date(2026, time.October, 1, 3, 17, 1, 0, time.UTC)
	store := New(writer, func() time.Time { return ingestedAt })
	run := storage.CollectionRun{
		Provider: "aws", BillingAccountID: "123456789012",
		WindowStart: time.Date(2026, time.September, 23, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
		StartedAt:   time.Date(2026, time.October, 1, 3, 15, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, time.October, 1, 3, 17, 0, 0, time.UTC),
		RecordCount: 8, LatestUsageEnd: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.WriteCollectionRun(context.Background(), run); err != nil {
		t.Fatalf("WriteCollectionRun() error = %v", err)
	}
	want := run
	want.DataIngestedAt = ingestedAt
	if writer.run != want {
		t.Errorf("stored collection run = %#v, want %#v", writer.run, want)
	}
}

func TestReadCollectionRunsScopesAccountAndUTCAnalysisWindow(t *testing.T) {
	start := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.FixedZone("UTC+9", 9*60*60))
	end := start.AddDate(0, 0, 1)
	want := []storage.CollectionRun{{Provider: "aws", BillingAccountID: "123", WindowStart: start.UTC(), WindowEnd: end.UTC()}}
	reader := &fakeCollectionRunInserter{readRuns: want}
	store := New(reader, time.Now)
	accounts := []AccountScope{{Provider: "aws", BillingAccountID: "123"}}

	got, err := store.ReadCollectionRuns(context.Background(), accounts, start, end)
	if err != nil {
		t.Fatalf("ReadCollectionRuns() error = %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("ReadCollectionRuns() = %#v, want %#v", got, want)
	}
	if reader.readRunCalls != 1 || !equalScopes(reader.readRunAccounts, accounts) || !reader.readRunStart.Equal(start.UTC()) || !reader.readRunEnd.Equal(end.UTC()) {
		t.Errorf("run reader call = %d scopes=%#v range=[%s,%s), want scoped UTC interval", reader.readRunCalls, reader.readRunAccounts, reader.readRunStart, reader.readRunEnd)
	}
}

func TestWriteCostsSanitizesStorageErrors(t *testing.T) {
	inserter := &fakeInserter{err: errors.New("database password leaked by upstream")}
	err := New(inserter, time.Now).WriteCosts(context.Background(), []normalize.CostRecord{{Provider: "aws"}})
	if !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("WriteCosts() error = %v, want ErrStorageUnavailable", err)
	}
	if err.Error() == "" || strings.Contains(err.Error(), "database password") {
		t.Errorf("WriteCosts() error = %q, want a sanitized message", err)
	}
}

func TestReadCostsReturnsEmptyForNoAccountsWithoutQuerying(t *testing.T) {
	inserter := &fakeInserter{}
	store := New(inserter, time.Now)
	got, err := store.ReadCosts(context.Background(), nil, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("ReadCosts() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ReadCosts() = %#v, want empty result", got)
	}
	if inserter.readCalls != 0 {
		t.Errorf("reader calls = %d, want none for empty account scope", inserter.readCalls)
	}
}

func TestReadCostsPassesUTCWindowAndAccountScopes(t *testing.T) {
	start := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.FixedZone("UTC+9", 9*60*60))
	end := start.AddDate(0, 1, 0)
	want := []normalize.CostRecord{{Provider: "aws", BillingAccountID: "123", SourceRecordID: "record-1", Amount: "12.3400", Currency: "USD"}}
	inserter := &fakeInserter{readRecords: want}
	store := New(inserter, time.Now)
	accounts := []AccountScope{{Provider: "aws", BillingAccountID: "123"}, {Provider: "gcp", BillingAccountID: "456"}}

	got, err := store.ReadCosts(context.Background(), accounts, start, end)
	if err != nil {
		t.Fatalf("ReadCosts() error = %v", err)
	}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("ReadCosts() = %#v, want %#v", got, want)
	}
	if inserter.readCalls != 1 || !equalScopes(inserter.readAccounts, accounts) {
		t.Errorf("reader call = %d with %#v, want one call with %#v", inserter.readCalls, inserter.readAccounts, accounts)
	}
	if inserter.readStart.Location() != time.UTC || inserter.readEnd.Location() != time.UTC || !inserter.readStart.Equal(start.UTC()) || !inserter.readEnd.Equal(end.UTC()) {
		t.Errorf("reader window = [%s, %s), want UTC [%s, %s)", inserter.readStart, inserter.readEnd, start.UTC(), end.UTC())
	}
}

func TestReadCostsRejectsInvalidTimeRangeBeforeQuery(t *testing.T) {
	inserter := &fakeInserter{}
	store := New(inserter, time.Now)
	instant := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	_, err := store.ReadCosts(context.Background(), []AccountScope{{Provider: "aws", BillingAccountID: "123"}}, instant, instant)
	if err == nil {
		t.Fatal("ReadCosts() error = nil, want invalid time range error")
	}
	if inserter.readCalls != 0 {
		t.Errorf("reader calls = %d, want no query for invalid range", inserter.readCalls)
	}
}

func TestReadCostsSanitizesReaderErrors(t *testing.T) {
	inserter := &fakeInserter{readErr: errors.New("query failed with password=secret")}
	store := New(inserter, time.Now)
	start := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	_, err := store.ReadCosts(context.Background(), []AccountScope{{Provider: "aws", BillingAccountID: "123"}}, start, start.AddDate(0, 1, 0))
	if !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("ReadCosts() error = %v, want ErrStorageUnavailable", err)
	}
	if strings.Contains(err.Error(), "password=secret") {
		t.Errorf("ReadCosts() exposed storage error: %v", err)
	}
}

type fakeInserter struct {
	batches      [][]normalize.CostRecord
	insertedAt   time.Time
	err          error
	readErr      error
	readCalls    int
	readAccounts []AccountScope
	readStart    time.Time
	readEnd      time.Time
	readRecords  []normalize.CostRecord
}

type fakeCollectionRunInserter struct {
	fakeInserter
	run             storage.CollectionRun
	readRuns        []storage.CollectionRun
	readRunCalls    int
	readRunAccounts []AccountScope
	readRunStart    time.Time
	readRunEnd      time.Time
}

func (f *fakeCollectionRunInserter) InsertCollectionRun(_ context.Context, run storage.CollectionRun) error {
	f.run = run
	return nil
}

func (f *fakeCollectionRunInserter) ReadCollectionRuns(_ context.Context, accounts []AccountScope, start, end time.Time) ([]storage.CollectionRun, error) {
	f.readRunCalls++
	f.readRunAccounts = append([]AccountScope(nil), accounts...)
	f.readRunStart = start
	f.readRunEnd = end
	return f.readRuns, nil
}

func (f *fakeInserter) InsertBatch(_ context.Context, records []normalize.CostRecord, insertedAt time.Time) error {
	f.batches = append(f.batches, records)
	f.insertedAt = insertedAt
	return f.err
}

func (f *fakeInserter) ReadCosts(_ context.Context, accounts []AccountScope, start, end time.Time) ([]normalize.CostRecord, error) {
	f.readCalls++
	f.readAccounts = append([]AccountScope(nil), accounts...)
	f.readStart = start
	f.readEnd = end
	return f.readRecords, f.readErr
}

func equalScopes(left, right []AccountScope) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (*fakeInserter) Close() error { return nil }
