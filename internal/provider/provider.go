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
	Provider       string    `json:"provider"`
	SourceRecordID string    `json:"sourceRecordId"`
	BillingScope   string    `json:"billingScope"`
	Amount         string    `json:"amount"`
	Currency       string    `json:"currency"`
	UsageStart     time.Time `json:"usageStart"`
	UsageEnd       time.Time `json:"usageEnd"`
}

type ProviderMetadata struct {
	Name string
}

type Provider interface {
	ValidateCredentials(context.Context) error
	CollectCosts(context.Context, CollectRequest) ([]RawCostRecord, error)
	Metadata(context.Context) ProviderMetadata
}
