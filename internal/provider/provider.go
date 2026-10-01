package provider

import (
	"context"
	"time"
)

type CollectRequest struct {
	AccountID      string
	ProviderConfig map[string]string
	StartTime      time.Time
	EndTime        time.Time
	CollectionID   string
}

type CostBasis string

const CostBasisUnblended CostBasis = "unblended_cost"
const CostBasisNet CostBasis = "net_cost"
const CostBasisActualPreTax CostBasis = "actual_pre_tax_cost"
const CostBasisOCI CostBasis = "oci_cost"
const CostBasisIBMBilled CostBasis = "ibm_billed_cost"
const CostBasisAlibabaPretax CostBasis = "alibaba_pretax_cost"

type RawCostRecord struct {
	Provider       string    `json:"provider"`
	SourceRecordID string    `json:"sourceRecordId"`
	BillingScope   string    `json:"billingScope"`
	CostBasis      CostBasis `json:"costBasis,omitempty"`
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
