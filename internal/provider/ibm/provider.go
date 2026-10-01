package ibmprovider

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/shopspring/decimal"
)

var accountIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type focusRow struct {
	start, end           time.Time
	billedCost, currency string
}

type focusRequest struct {
	accountID  string
	start, end time.Time
}

type focusSource interface {
	collect(context.Context, focusRequest, func(focusRow) error) error
}

type Provider struct{ source focusSource }

func New() *Provider { return &Provider{source: newRESTSource(nil, "", "")} }

func newWithFocusSource(source focusSource) *Provider { return &Provider{source: source} }

func (*Provider) Metadata(context.Context) provider.ProviderMetadata {
	return provider.ProviderMetadata{Name: "ibm"}
}

func (*Provider) ValidateCredentials(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	if strings.TrimSpace(os.Getenv("IBM_CLOUD_API_KEY")) == "" {
		return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
	}
	return nil
}

func (p *Provider) CollectCosts(ctx context.Context, request provider.CollectRequest) ([]provider.RawCostRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	if p.source == nil {
		return nil, fmt.Errorf("IBM Usage Reports client is required")
	}
	if !accountIDPattern.MatchString(request.AccountID) {
		return nil, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	start, end, validWindow := provider.NormalizeDailyWindow(request.StartTime, request.EndTime)
	if !validWindow {
		return nil, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	totals := make(map[string]decimal.Decimal)
	err := p.source.collect(ctx, focusRequest{accountID: request.AccountID, start: start, end: end}, func(row focusRow) error {
		day, periodEnd := row.start.UTC(), row.end.UTC()
		if !periodEnd.After(start) || !day.Before(end) {
			return nil
		}
		if !provider.IsUTCMidnight(day) || !periodEnd.Equal(day.AddDate(0, 0, 1)) || row.currency == "" {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		amount, parseErr := decimal.NewFromString(row.billedCost)
		if parseErr != nil {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		key := day.Format("2006-01-02") + ":" + row.currency
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
			Provider: "ibm", SourceRecordID: "ibm-focus-" + request.AccountID + "-" + parts[0] + "-" + parts[1],
			BillingScope: request.AccountID, CostBasis: provider.CostBasisIBMBilled, Amount: amount.String(), Currency: parts[1],
			UsageStart: day, UsageEnd: day.AddDate(0, 0, 1),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].SourceRecordID < records[j].SourceRecordID })
	return records, nil
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

const (
	defaultIAMEndpoint     = "https://iam.cloud.ibm.com"
	defaultBillingEndpoint = "https://billing.cloud.ibm.com"
)

type restSource struct {
	client          *http.Client
	iamEndpoint     string
	billingEndpoint string
}

func newRESTSource(client *http.Client, iamEndpoint, billingEndpoint string) *restSource {
	return &restSource{client: client, iamEndpoint: iamEndpoint, billingEndpoint: billingEndpoint}
}

func (s *restSource) collect(ctx context.Context, request focusRequest, consume func(focusRow) error) error {
	apiKey := os.Getenv("IBM_CLOUD_API_KEY")
	if strings.TrimSpace(apiKey) == "" {
		return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
	}
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	iamEndpoint := s.iamEndpoint
	if iamEndpoint == "" {
		iamEndpoint = defaultIAMEndpoint
	}
	billingEndpoint := s.billingEndpoint
	if billingEndpoint == "" {
		billingEndpoint = defaultBillingEndpoint
	}
	iamBase, err := parseHTTPSBase(iamEndpoint)
	if err != nil {
		return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	billingBase, err := parseHTTPSBase(billingEndpoint)
	if err != nil {
		return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	token, err := requestIAMToken(ctx, client, iamBase, apiKey)
	if err != nil {
		return err
	}
	lastDay := request.end.Add(-time.Nanosecond)
	month := time.Date(request.start.Year(), request.start.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastMonth := time.Date(lastDay.Year(), lastDay.Month(), 1, 0, 0, 0, 0, time.UTC)
	for ; !month.After(lastMonth); month = month.AddDate(0, 1, 0) {
		if err := requestFOCUSMonth(ctx, client, billingBase, token, request.accountID, month, consume); err != nil {
			return err
		}
	}
	return nil
}

func requestIAMToken(ctx context.Context, client *http.Client, base *url.URL, apiKey string) (string, error) {
	form := url.Values{
		"grant_type": {"urn:ibm:params:oauth:grant-type:apikey"},
		"apikey":     {apiKey},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String()+"/identity/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return "", classifyError(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", classifyHTTPStatus(response.StatusCode, true)
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil || result.AccessToken == "" {
		return "", &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	return result.AccessToken, nil
}

func requestFOCUSMonth(ctx context.Context, client *http.Client, base *url.URL, token, accountID string, month time.Time, consume func(focusRow) error) error {
	endpoint := *base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v4/accounts/" + accountID + "/focus/" + month.Format("2006-01")
	query := endpoint.Query()
	query.Set("format", "csv")
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "text/csv")
	request.Header.Set("x-focus-version", "1.2")
	response, err := client.Do(request)
	if err != nil {
		return classifyError(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return classifyHTTPStatus(response.StatusCode, false)
	}
	return decodeFOCUSCSV(response.Body, consume)
}

func decodeFOCUSCSV(reader io.Reader, consume func(focusRow) error) error {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	header, err := csvReader.Read()
	if err != nil {
		return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}
	columns := make(map[string]int, len(header))
	for index, name := range header {
		columns[strings.TrimSpace(name)] = index
	}
	for _, required := range []string{"BilledCost", "BillingCurrency", "ChargePeriodStart", "ChargePeriodEnd"} {
		if _, exists := columns[required]; !exists {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
	}
	for {
		record, err := csvReader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil || len(record) != len(header) {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		start, startErr := time.Parse(time.RFC3339Nano, record[columns["ChargePeriodStart"]])
		end, endErr := time.Parse(time.RFC3339Nano, record[columns["ChargePeriodEnd"]])
		if startErr != nil || endErr != nil {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		if err := consume(focusRow{start: start.UTC(), end: end.UTC(), billedCost: record[columns["BilledCost"]], currency: record[columns["BillingCurrency"]]}); err != nil {
			return err
		}
	}
}

func parseHTTPSBase(endpoint string) (*url.URL, error) {
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("invalid IBM API endpoint")
	}
	return base, nil
}

func classifyHTTPStatus(status int, authenticating bool) error {
	class := provider.ErrorInvalidResponse
	switch status {
	case http.StatusUnauthorized:
		class = provider.ErrorAuthenticationFailed
	case http.StatusForbidden:
		class = provider.ErrorPermissionDenied
	case http.StatusTooManyRequests:
		class = provider.ErrorRateLimited
	default:
		if status >= 500 {
			class = provider.ErrorProviderUnavailable
		} else if authenticating && status == http.StatusBadRequest {
			class = provider.ErrorAuthenticationFailed
		}
	}
	return &provider.ProviderError{Class: class}
}
