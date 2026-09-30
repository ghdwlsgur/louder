package normalize

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/ghdwlsgur/louder/internal/provider"
)

var ErrInvalidCostRecord = errors.New("invalid raw cost record")

var decimalAmount = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)$`)

type CostRecord struct {
	Provider         string             `json:"provider"`
	BillingAccountID string             `json:"billingAccountId"`
	SourceRecordID   string             `json:"sourceRecordId"`
	CostBasis        provider.CostBasis `json:"costBasis"`
	Amount           string             `json:"amount"`
	Currency         string             `json:"currency"`
	UsageStart       time.Time          `json:"usageStart"`
	UsageEnd         time.Time          `json:"usageEnd"`
}

func Normalize(raw provider.RawCostRecord) (CostRecord, error) {
	if strings.TrimSpace(raw.Provider) == "" || strings.TrimSpace(raw.BillingScope) == "" || strings.TrimSpace(raw.SourceRecordID) == "" || strings.TrimSpace(string(raw.CostBasis)) == "" {
		return CostRecord{}, ErrInvalidCostRecord
	}
	if !decimalAmount.MatchString(raw.Amount) {
		return CostRecord{}, ErrInvalidCostRecord
	}
	if !validCurrency(raw.Currency) {
		return CostRecord{}, ErrInvalidCostRecord
	}
	if raw.UsageStart.IsZero() || raw.UsageEnd.IsZero() || !raw.UsageStart.Before(raw.UsageEnd) {
		return CostRecord{}, ErrInvalidCostRecord
	}
	return CostRecord{
		Provider:         raw.Provider,
		BillingAccountID: raw.BillingScope,
		SourceRecordID:   raw.SourceRecordID,
		CostBasis:        raw.CostBasis,
		Amount:           raw.Amount,
		Currency:         raw.Currency,
		UsageStart:       raw.UsageStart.UTC(),
		UsageEnd:         raw.UsageEnd.UTC(),
	}, nil
}

func validCurrency(currency string) bool {
	if len(currency) != 3 {
		return false
	}
	for _, letter := range currency {
		if letter < 'A' || letter > 'Z' {
			return false
		}
	}
	return true
}
