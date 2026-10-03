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
const CostBasisNCPMonthly CostBasis = "ncp_monthly_invoice_cost"

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

func NormalizeDailyWindow(start, end time.Time) (time.Time, time.Time, bool) {
	start, end = start.UTC(), end.UTC()
	return start, end, start.Before(end) && IsUTCMidnight(start) && IsUTCMidnight(end)
}

func IsUTCMidnight(value time.Time) bool {
	value = value.UTC()
	return value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
}

func ProvidesDailyCostRecords(name string) bool {
	switch name {
	case "aws", "azure", "gcp", "oci", "ibm", "alibaba":
		return true
	default:
		return false
	}
}

func ExpectedDailyCostDataDelay(name string) (time.Duration, bool) {
	switch name {
	case "aws", "gcp", "alibaba":
		return 24 * time.Hour, true
	case "azure":
		return 72 * time.Hour, true
	default:
		return 0, false
	}
}
