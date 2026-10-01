package storage

import (
	"context"
	"time"

	"github.com/ghdwlsgur/louder/internal/normalize"
)

type AccountScope struct {
	Provider         string
	BillingAccountID string
}

type CostReader interface {
	ReadCosts(context.Context, []AccountScope, time.Time, time.Time) ([]normalize.CostRecord, error)
}
