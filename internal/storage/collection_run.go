package storage

import (
	"context"
	"time"
)

type CollectionRun struct {
	Provider         string
	BillingAccountID string
	WindowStart      time.Time
	WindowEnd        time.Time
	StartedAt        time.Time
	CompletedAt      time.Time
	DataIngestedAt   time.Time
	RecordCount      uint64
	LatestUsageEnd   time.Time
}

type CollectionRunWriter interface {
	WriteCollectionRun(context.Context, CollectionRun) error
}

type CollectionRunReader interface {
	ReadCollectionRuns(context.Context, []AccountScope, time.Time, time.Time) ([]CollectionRun, error)
}
