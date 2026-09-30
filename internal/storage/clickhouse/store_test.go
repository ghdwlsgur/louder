package clickhouse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/internal/normalize"
)

func TestWriteCostsWritesOneBatchWithUTCIngestionTime(t *testing.T) {
	insertedAt := time.Date(2026, time.October, 1, 3, 0, 0, 0, time.FixedZone("UTC+9", 9*60*60))
	inserter := &fakeInserter{}
	store := New(inserter, func() time.Time { return insertedAt })
	want := []normalize.CostRecord{{
		Provider:         "aws",
		BillingAccountID: "123456789012",
		SourceRecordID:   "aws-cost-explorer-123456789012-2026-09-30",
		CostBasis:        "unblended_cost",
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

type fakeInserter struct {
	batches    [][]normalize.CostRecord
	insertedAt time.Time
	err        error
}

func (f *fakeInserter) InsertBatch(_ context.Context, records []normalize.CostRecord, insertedAt time.Time) error {
	f.batches = append(f.batches, records)
	f.insertedAt = insertedAt
	return f.err
}

func (*fakeInserter) Close() error { return nil }
