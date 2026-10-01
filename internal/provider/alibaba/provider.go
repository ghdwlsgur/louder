package alibabaprovider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/shopspring/decimal"
)

var accountIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)

type billRow struct {
	billingDate, amount, currency string
}

type billRequest struct {
	accountID  string
	start, end time.Time
}

type billSource interface {
	collect(context.Context, billRequest, func(billRow) error) error
}

type Provider struct{ source billSource }

func New() *Provider { return &Provider{source: newBSSSource(nil, nil, "")} }

func newWithBillSource(source billSource) *Provider { return &Provider{source: source} }

func (*Provider) Metadata(context.Context) provider.ProviderMetadata {
	return provider.ProviderMetadata{Name: "alibaba"}
}

func (*Provider) ValidateCredentials(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	if strings.TrimSpace(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")) == "" || strings.TrimSpace(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")) == "" {
		return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
	}
	return nil
}

func (p *Provider) CollectCosts(ctx context.Context, request provider.CollectRequest) ([]provider.RawCostRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	if p.source == nil {
		return nil, fmt.Errorf("Alibaba Cloud BSS client is required")
	}
	if !accountIDPattern.MatchString(request.AccountID) {
		return nil, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	start, end := request.StartTime.UTC(), request.EndTime.UTC()
	if !start.Before(end) || !isUTCMidnight(start) || !isUTCMidnight(end) {
		return nil, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	totals := make(map[string]decimal.Decimal)
	err := p.source.collect(ctx, billRequest{accountID: request.AccountID, start: start, end: end}, func(row billRow) error {
		day, parseErr := time.Parse("2006-01-02", row.billingDate)
		if parseErr != nil || !isUTCMidnight(day) || day.Before(start) || !day.Before(end) || row.currency == "" {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		amount, parseErr := decimal.NewFromString(row.amount)
		if parseErr != nil {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		key := row.billingDate + ":" + row.currency
		totals[key] = totals[key].Add(amount)
		return nil
	})
	if err != nil {
		return nil, classifyError(err)
	}
	records := make([]provider.RawCostRecord, 0, len(totals))
	for key, amount := range totals {
		parts := strings.SplitN(key, ":", 2)
		day, _ := time.Parse("2006-01-02", parts[0])
		records = append(records, provider.RawCostRecord{
			Provider: "alibaba", SourceRecordID: "alibaba-instance-" + request.AccountID + "-" + parts[0] + "-" + parts[1],
			BillingScope: request.AccountID, CostBasis: provider.CostBasisAlibabaPretax, Amount: amount.String(), Currency: parts[1],
			UsageStart: day, UsageEnd: day.AddDate(0, 0, 1),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].SourceRecordID < records[j].SourceRecordID })
	return records, nil
}

func isUTCMidnight(value time.Time) bool {
	return value.Location() == time.UTC && value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
}

func classifyError(err error) error {
	var existing *provider.ProviderError
	if errors.As(err, &existing) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	return &provider.ProviderError{Class: provider.ErrorProviderUnavailable}
}
