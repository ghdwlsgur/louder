package azureprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/shopspring/decimal"
)

const (
	azureManagementEndpoint = "https://management.azure.com"
	azureCostAPIVersion     = "2023-03-01"
	azureTokenScope         = "https://management.azure.com/.default"
)

var subscriptionIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type queryRequest struct {
	subscriptionID string
	start          time.Time
	end            time.Time
}

type dailyCost struct {
	date     string
	amount   string
	currency string
}

type costQuery interface {
	queryDailyCosts(context.Context, queryRequest) ([]dailyCost, error)
}

type Provider struct {
	query costQuery
}

func New() *Provider {
	return &Provider{query: azureRESTQuery{}}
}

func newWithQuery(query costQuery) *Provider {
	return &Provider{query: query}
}

func (*Provider) Metadata(context.Context) provider.ProviderMetadata {
	return provider.ProviderMetadata{Name: "azure"}
}

func (*Provider) ValidateCredentials(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	credential, err := newClientSecretCredential()
	if err != nil || credential == nil {
		return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
	}
	return nil
}

func (p *Provider) CollectCosts(ctx context.Context, request provider.CollectRequest) ([]provider.RawCostRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	if p.query == nil {
		return nil, fmt.Errorf("Azure Cost Management query client is required")
	}
	query, err := validateRequest(request)
	if err != nil {
		return nil, err
	}
	rows, err := p.query.queryDailyCosts(ctx, query)
	if err != nil {
		return nil, classifyError(err)
	}
	records := make([]provider.RawCostRecord, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		record, mapErr := mapDailyCost(request.AccountID, row)
		if mapErr != nil {
			return nil, mapErr
		}
		if record.UsageStart.Before(query.start) || !record.UsageStart.Before(query.end) {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		if _, duplicate := seen[record.SourceRecordID]; duplicate {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		seen[record.SourceRecordID] = struct{}{}
		records = append(records, record)
	}
	return records, nil
}

func validateRequest(request provider.CollectRequest) (queryRequest, error) {
	if !subscriptionIDPattern.MatchString(request.AccountID) {
		return queryRequest{}, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	start, end, validWindow := provider.NormalizeDailyWindow(request.StartTime, request.EndTime)
	if !validWindow {
		return queryRequest{}, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	return queryRequest{subscriptionID: request.AccountID, start: start, end: end}, nil
}

func mapDailyCost(subscriptionID string, row dailyCost) (provider.RawCostRecord, error) {
	start, err := time.Parse("2006-01-02", row.date)
	amount, amountErr := decimal.NewFromString(row.amount)
	if err != nil || amountErr != nil || row.currency == "" {
		return provider.RawCostRecord{}, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	return provider.RawCostRecord{
		Provider: "azure", SourceRecordID: "azure-cost-query-" + subscriptionID + "-" + row.date + "-" + row.currency,
		BillingScope: subscriptionID, CostBasis: provider.CostBasisActualPreTax, Amount: amount.String(), Currency: row.currency,
		UsageStart: start, UsageEnd: start.AddDate(0, 0, 1),
	}, nil
}

type tokenCredential interface {
	GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error)
}

type azureRESTQuery struct {
	endpoint   string
	httpClient *http.Client
	credential tokenCredential
}

func (q azureRESTQuery) queryDailyCosts(ctx context.Context, request queryRequest) ([]dailyCost, error) {
	credential := q.credential
	if credential == nil {
		var err error
		credential, err = newClientSecretCredential()
		if err != nil {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
		}
	}
	token, err := credential.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{azureTokenScope}})
	if err != nil {
		return nil, classifyError(err)
	}
	endpoint := q.endpoint
	if endpoint == "" {
		endpoint = azureManagementEndpoint
	}
	baseURL, err := url.Parse(endpoint)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	body, err := json.Marshal(buildQueryDefinition(request))
	if err != nil {
		return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse, Err: err}
	}
	queryURL := strings.TrimRight(endpoint, "/") + "/subscriptions/" + request.subscriptionID + "/providers/Microsoft.CostManagement/query?api-version=" + azureCostAPIVersion
	client := q.httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	var records []dailyCost
	seen := make(map[string]struct{})
	for page := 0; queryURL != ""; page++ {
		if page >= 100 {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		parsedURL, parseErr := url.Parse(queryURL)
		if parseErr != nil || parsedURL.Scheme != baseURL.Scheme || !strings.EqualFold(parsedURL.Host, baseURL.Host) || parsedURL.User != nil {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		if _, duplicate := seen[queryURL]; duplicate {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		seen[queryURL] = struct{}{}
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, queryURL, bytes.NewReader(body))
		if requestErr != nil {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse, Err: requestErr}
		}
		request.Header.Set("Authorization", "Bearer "+token.Token)
		request.Header.Set("Content-Type", "application/json")
		response, requestErr := client.Do(request)
		if requestErr != nil {
			return nil, classifyError(requestErr)
		}
		if response.StatusCode == http.StatusNoContent {
			_ = response.Body.Close()
			return records, nil
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			_ = response.Body.Close()
			return nil, classifyStatus(response.StatusCode)
		}
		var result queryResponse
		decodeErr := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&result)
		_ = response.Body.Close()
		if decodeErr != nil {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		pageRecords, rowErr := mapResponseRows(result.Properties.Columns, result.Properties.Rows)
		if rowErr != nil {
			return nil, rowErr
		}
		records = append(records, pageRecords...)
		queryURL = result.Properties.NextLink
	}
	return records, nil
}

type queryDefinition struct {
	Type       string          `json:"type"`
	Timeframe  string          `json:"timeframe"`
	TimePeriod queryTimePeriod `json:"timePeriod"`
	Dataset    queryDataset    `json:"dataset"`
}

type queryTimePeriod struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type queryDataset struct {
	Granularity string                      `json:"granularity"`
	Aggregation map[string]queryAggregation `json:"aggregation"`
}

type queryAggregation struct {
	Name     string `json:"name"`
	Function string `json:"function"`
}

type queryResponse struct {
	Properties struct {
		Columns  []queryColumn       `json:"columns"`
		Rows     [][]json.RawMessage `json:"rows"`
		NextLink string              `json:"nextLink"`
	} `json:"properties"`
}

type queryColumn struct {
	Name string `json:"name"`
}

func buildQueryDefinition(request queryRequest) queryDefinition {
	return queryDefinition{
		Type: "ActualCost", Timeframe: "Custom",
		TimePeriod: queryTimePeriod{From: request.start, To: request.end.Add(-time.Nanosecond)},
		Dataset:    queryDataset{Granularity: "Daily", Aggregation: map[string]queryAggregation{"totalCost": {Name: "PreTaxCost", Function: "Sum"}}},
	}
}

func mapResponseRows(columns []queryColumn, rows [][]json.RawMessage) ([]dailyCost, error) {
	indices := make(map[string]int, len(columns))
	for index, column := range columns {
		indices[column.Name] = index
	}
	amountIndex, amountOK := indices["PreTaxCost"]
	dateIndex, dateOK := indices["UsageDate"]
	currencyIndex, currencyOK := indices["Currency"]
	if !amountOK || !dateOK || !currencyOK {
		return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	results := make([]dailyCost, 0, len(rows))
	for _, row := range rows {
		if amountIndex >= len(row) || dateIndex >= len(row) || currencyIndex >= len(row) {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		var amountValue any
		var dateValue any
		var currency string
		amountDecoder := json.NewDecoder(bytes.NewReader(row[amountIndex]))
		amountDecoder.UseNumber()
		dateDecoder := json.NewDecoder(bytes.NewReader(row[dateIndex]))
		dateDecoder.UseNumber()
		if amountDecoder.Decode(&amountValue) != nil || dateDecoder.Decode(&dateValue) != nil || json.Unmarshal(row[currencyIndex], &currency) != nil {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		amountNumber, ok := amountValue.(json.Number)
		if !ok {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		amount, err := decimal.NewFromString(amountNumber.String())
		if err != nil || currency == "" {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		date, err := parseUsageDate(dateValue)
		if err != nil {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		results = append(results, dailyCost{date: date, amount: amount.String(), currency: currency})
	}
	return results, nil
}

func parseUsageDate(value any) (string, error) {
	var date time.Time
	switch typed := value.(type) {
	case json.Number:
		parsed, err := time.Parse("20060102", typed.String())
		if err != nil {
			return "", err
		}
		date = parsed
	case string:
		parsed, err := time.Parse("2006-01-02", typed)
		if err != nil {
			return "", err
		}
		date = parsed
	default:
		return "", fmt.Errorf("invalid Azure usage date")
	}
	return date.Format("2006-01-02"), nil
}

func newClientSecretCredential() (*azidentity.ClientSecretCredential, error) {
	tenantID, clientID, clientSecret := os.Getenv("AZURE_TENANT_ID"), os.Getenv("AZURE_CLIENT_ID"), os.Getenv("AZURE_CLIENT_SECRET")
	if !subscriptionIDPattern.MatchString(tenantID) || !subscriptionIDPattern.MatchString(clientID) || clientSecret == "" {
		return nil, fmt.Errorf("invalid Azure service principal credential shape")
	}
	return azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
}

func classifyError(err error) *provider.ProviderError {
	var providerError *provider.ProviderError
	if errors.As(err, &providerError) {
		return providerError
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	var responseError *azcore.ResponseError
	if errors.As(err, &responseError) {
		return classifyStatus(responseError.StatusCode)
	}
	return &provider.ProviderError{Class: provider.ErrorProviderUnavailable}
}

func classifyStatus(statusCode int) *provider.ProviderError {
	switch statusCode {
	case http.StatusUnauthorized:
		return &provider.ProviderError{Class: provider.ErrorAuthenticationFailed}
	case http.StatusForbidden:
		return &provider.ProviderError{Class: provider.ErrorPermissionDenied}
	case http.StatusTooManyRequests:
		return &provider.ProviderError{Class: provider.ErrorRateLimited}
	case http.StatusRequestTimeout:
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	case http.StatusBadRequest, http.StatusNotFound:
		return &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	default:
		if statusCode >= 500 {
			return &provider.ProviderError{Class: provider.ErrorProviderUnavailable}
		}
		return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
}
