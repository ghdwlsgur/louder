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
	ErrStorageUnavailable = errors.New("clickhouse storage unavailable")
	ErrInvalidTimeRange   = errors.New("invalid clickhouse query time range")
)

type Inserter interface {
	InsertBatch(context.Context, []normalize.CostRecord, time.Time) error
	Close() error
}

type AccountScope = storage.AccountScope
type Reader = storage.CostReader

type Store struct {
	inserter Inserter
	reader   storage.CostReader
	now      func() time.Time
}

var _ storage.CostReader = (*Store)(nil)

func New(inserter Inserter, now func() time.Time) *Store {
	reader, _ := inserter.(Reader)
	return &Store{inserter: inserter, reader: reader, now: now}
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
