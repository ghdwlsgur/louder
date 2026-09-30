package provider

import (
	"context"
	"time"
)

type CollectRequest struct {
	AccountID    string
	StartTime    time.Time
	EndTime      time.Time
	CollectionID string
}

type RawCostRecord struct {
	Provider       string
	SourceRecordID string
	BillingScope   string
	Amount         string
	Currency       string
	UsageStart     time.Time
	UsageEnd       time.Time
}

type ProviderMetadata struct {
	Name string
}

type Provider interface {
	ValidateCredentials(context.Context) error
	CollectCosts(context.Context, CollectRequest) ([]RawCostRecord, error)
	Metadata(context.Context) ProviderMetadata
}
