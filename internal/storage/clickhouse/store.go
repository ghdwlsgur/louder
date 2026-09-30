package clickhouse

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghdwlsgur/louder/internal/normalize"
)

var ErrStorageUnavailable = errors.New("clickhouse storage unavailable")

type Inserter interface {
	InsertBatch(context.Context, []normalize.CostRecord, time.Time) error
	Close() error
}

type Store struct {
	inserter Inserter
	now      func() time.Time
}

func New(inserter Inserter, now func() time.Time) *Store {
	return &Store{inserter: inserter, now: now}
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

func (s *Store) Close() error {
	if s.inserter == nil {
		return nil
	}
	if err := s.inserter.Close(); err != nil {
		return ErrStorageUnavailable
	}
	return nil
}
