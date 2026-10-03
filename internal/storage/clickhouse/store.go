package clickhouse

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghdwlsgur/louder/internal/normalize"
	"github.com/ghdwlsgur/louder/internal/storage"
)

var (
	ErrStorageUnavailable   = errors.New("clickhouse storage unavailable")
	ErrInvalidTimeRange     = errors.New("invalid clickhouse query time range")
	ErrInvalidCollectionRun = errors.New("invalid collection run")
)

type Inserter interface {
	InsertBatch(context.Context, []normalize.CostRecord, time.Time) error
	Close() error
}

type AccountScope = storage.AccountScope
type Reader = storage.CostReader

type Store struct {
	inserter  Inserter
	reader    storage.CostReader
	runWriter collectionRunInserter
	runReader storage.CollectionRunReader
	now       func() time.Time
}

type collectionRunInserter interface {
	InsertCollectionRun(context.Context, storage.CollectionRun) error
}

type collectionRunReader interface {
	ReadCollectionRuns(context.Context, []AccountScope, time.Time, time.Time) ([]storage.CollectionRun, error)
}

var _ storage.CostReader = (*Store)(nil)

func New(inserter Inserter, now func() time.Time) *Store {
	reader, _ := inserter.(Reader)
	runWriter, _ := inserter.(collectionRunInserter)
	runReader, _ := inserter.(collectionRunReader)
	return &Store{inserter: inserter, reader: reader, runWriter: runWriter, runReader: runReader, now: now}
}

func (s *Store) WriteCollectionRun(ctx context.Context, run storage.CollectionRun) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if run.Provider == "" || run.BillingAccountID == "" || run.WindowStart.IsZero() || run.WindowEnd.IsZero() || !run.WindowStart.Before(run.WindowEnd) || run.StartedAt.IsZero() || run.CompletedAt.IsZero() || run.CompletedAt.Before(run.StartedAt) {
		return ErrInvalidCollectionRun
	}
	if s.runWriter == nil || s.now == nil {
		return ErrStorageUnavailable
	}
	run.DataIngestedAt = s.now().UTC()
	if err := s.runWriter.InsertCollectionRun(ctx, run); err != nil {
		return ErrStorageUnavailable
	}
	return nil
}

func (s *Store) ReadCollectionRuns(ctx context.Context, accounts []AccountScope, start, end time.Time) ([]storage.CollectionRun, error) {
	if len(accounts) == 0 {
		return []storage.CollectionRun{}, nil
	}
	if start.IsZero() || end.IsZero() || !start.Before(end) {
		return nil, ErrInvalidTimeRange
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.runReader == nil {
		return nil, ErrStorageUnavailable
	}
	runs, err := s.runReader.ReadCollectionRuns(ctx, accounts, start.UTC(), end.UTC())
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	return runs, nil
}

func (s *Store) WriteCosts(ctx context.Context, records []normalize.CostRecord) error {
	if len(records) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.inserter == nil || s.now == nil {
		return ErrStorageUnavailable
	}
	if err := s.inserter.InsertBatch(ctx, records, s.now().UTC()); err != nil {
		return fmt.Errorf("%w", ErrStorageUnavailable)
	}
	return nil
}

func (s *Store) ReadCosts(ctx context.Context, accounts []AccountScope, start, end time.Time) ([]normalize.CostRecord, error) {
	if len(accounts) == 0 {
		return []normalize.CostRecord{}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if start.IsZero() || end.IsZero() || !start.Before(end) {
		return nil, ErrInvalidTimeRange
	}
	if s.reader == nil {
		return nil, ErrStorageUnavailable
	}
	records, err := s.reader.ReadCosts(ctx, accounts, start.UTC(), end.UTC())
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	return records, nil
}

func (s *Store) Close() error {
	if s.inserter == nil {
		return nil
	}
	if err := s.inserter.Close(); err != nil {
		return ErrStorageUnavailable
	}
	return nil
}
