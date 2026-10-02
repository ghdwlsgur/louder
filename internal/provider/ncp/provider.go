package ncpprovider

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

var accountIDPattern = regexp.MustCompile(`^[0-9]+$`)

type demandRow struct {
	memberNo           string
	demandMonth        string
	amountIncludingVAT string
	currency           string
}

type demandRequest struct {
	accountID            string
	startMonth, endMonth string
}

type demandSource interface {
	collect(context.Context, demandRequest, func(demandRow) error) error
}

type Provider struct{ source demandSource }

func New() *Provider { return &Provider{source: newRESTSource(nil, "", nil)} }

func newWithDemandSource(source demandSource) *Provider { return &Provider{source: source} }

func (*Provider) Metadata(context.Context) provider.ProviderMetadata {
	return provider.ProviderMetadata{Name: "ncp"}
}

func (*Provider) ValidateCredentials(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	if strings.TrimSpace(os.Getenv("NCP_ACCESS_KEY")) == "" || strings.TrimSpace(os.Getenv("NCP_SECRET_KEY")) == "" {
		return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
	}
	return nil
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
	return &provider.ProviderError{Class: provider.ErrorProviderUnavailable, Err: err}
}

func (p *Provider) CollectCosts(ctx context.Context, request provider.CollectRequest) ([]provider.RawCostRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	if p.source == nil {
		return nil, fmt.Errorf("NCP billing source is required")
	}
	if !accountIDPattern.MatchString(request.AccountID) {
		return nil, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	start, end, validWindow := provider.NormalizeDailyWindow(request.StartTime, request.EndTime)
	if !validWindow {
		return nil, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	lastDay := end.Add(-time.Nanosecond)
	firstMonth := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastMonth := time.Date(lastDay.Year(), lastDay.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthCount := (lastMonth.Year()-firstMonth.Year())*12 + int(lastMonth.Month()-firstMonth.Month()) + 1
	if monthCount > 3 {
		return nil, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	req := demandRequest{accountID: request.AccountID, startMonth: firstMonth.Format("200601"), endMonth: lastMonth.Format("200601")}
	totals := make(map[string]decimal.Decimal)
	err := p.source.collect(ctx, req, func(row demandRow) error {
		if row.memberNo != request.AccountID {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		month, err := time.Parse("200601", row.demandMonth)
		if err != nil || row.demandMonth < req.startMonth || row.demandMonth > req.endMonth || !validCurrency(row.currency) {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		amount, err := decimal.NewFromString(row.amountIncludingVAT)
		if err != nil {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		monthKey := month.Format("2006-01") + ":" + row.currency
		totals[monthKey] = totals[monthKey].Add(amount)
		return nil
	})
	if err != nil {
		return nil, classifyError(err)
	}
	records := make([]provider.RawCostRecord, 0, len(totals))
	for key, amount := range totals {
		parts := strings.SplitN(key, ":", 2)
		month, _ := time.Parse("2006-01", parts[0])
		records = append(records, provider.RawCostRecord{
			Provider: "ncp", SourceRecordID: "ncp-monthly-" + request.AccountID + "-" + parts[0] + "-" + parts[1],
			BillingScope: request.AccountID, CostBasis: provider.CostBasisNCPMonthly, Amount: amount.String(), Currency: parts[1],
			UsageStart: month, UsageEnd: month.AddDate(0, 1, 0),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].SourceRecordID < records[j].SourceRecordID })
	return records, nil
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
