package gcpprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"regexp"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/ghdwlsgur/louder/internal/provider"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const credentialsEnv = "GOOGLE_CREDENTIALS_JSON"

var projectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type queryRequest struct {
	projectID string
	datasetID string
	tableID   string
	accountID string
	start     time.Time
	end       time.Time
}

type dailyCost struct {
	Date     string `bigquery:"usage_date"`
	Micros   int64  `bigquery:"net_micros"`
	Currency string `bigquery:"currency"`
}

type dailyQuery interface {
	queryDailyNetCosts(context.Context, queryRequest) ([]dailyCost, error)
}

type Provider struct {
	query dailyQuery
}

func New() *Provider {
	return &Provider{query: bigQueryDailyQuery{}}
}

func newWithQuery(query dailyQuery) *Provider {
	return &Provider{query: query}
}

func (*Provider) Metadata(context.Context) provider.ProviderMetadata {
	return provider.ProviderMetadata{Name: "gcp"}
}

func (*Provider) ValidateCredentials(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	var credentials struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	data := os.Getenv(credentialsEnv)
	if data == "" || json.Unmarshal([]byte(data), &credentials) != nil || credentials.Type != "service_account" || credentials.ClientEmail == "" || credentials.PrivateKey == "" {
		return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
	}
	return nil
}

func (p *Provider) CollectCosts(ctx context.Context, request provider.CollectRequest) ([]provider.RawCostRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	if p.query == nil {
		return nil, fmt.Errorf("BigQuery query client is required")
	}
	query, err := validateRequest(request, request.ProviderConfig)
	if err != nil {
		return nil, err
	}
	rows, err := p.query.queryDailyNetCosts(ctx, query)
	if err != nil {
		return nil, classifyError(ctx, err)
	}
	records := make([]provider.RawCostRecord, 0, len(rows))
	for _, row := range rows {
		record, mapErr := mapDailyCost(request.AccountID, row)
		if mapErr != nil {
			return nil, mapErr
		}
		if record.UsageStart.Before(query.start) || !record.UsageStart.Before(query.end) {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		records = append(records, record)
	}
	return records, nil
}

func classifyError(ctx context.Context, err error) *provider.ProviderError {
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	var apiError *googleapi.Error
	if errors.As(err, &apiError) {
		switch apiError.Code {
		case 401:
			return &provider.ProviderError{Class: provider.ErrorAuthenticationFailed, Err: err}
		case 403:
			return &provider.ProviderError{Class: provider.ErrorPermissionDenied, Err: err}
		case 429:
			return &provider.ProviderError{Class: provider.ErrorRateLimited, Err: err}
		default:
			if apiError.Code >= 500 {
				return &provider.ProviderError{Class: provider.ErrorProviderUnavailable, Err: err}
			}
		}
	}
	return &provider.ProviderError{Class: provider.ErrorProviderUnavailable, Err: err}
}

func validateRequest(request provider.CollectRequest, config map[string]string) (queryRequest, error) {
	projectID, datasetID, tableID := config["projectId"], config["datasetId"], config["tableId"]
	if !projectIDPattern.MatchString(projectID) || !identifierPattern.MatchString(datasetID) || !identifierPattern.MatchString(tableID) {
		return queryRequest{}, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	start, end, validWindow := provider.NormalizeDailyWindow(request.StartTime, request.EndTime)
	if request.AccountID == "" || !validWindow {
		return queryRequest{}, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	return queryRequest{projectID: projectID, datasetID: datasetID, tableID: tableID, accountID: request.AccountID, start: start, end: end}, nil
}

func mapDailyCost(accountID string, row dailyCost) (provider.RawCostRecord, error) {
	start, err := time.Parse("2006-01-02", row.Date)
	if err != nil || row.Currency == "" {
		return provider.RawCostRecord{}, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	amount := new(big.Rat).SetFrac(big.NewInt(row.Micros), big.NewInt(1_000_000)).FloatString(6)
	return provider.RawCostRecord{
		Provider: "gcp", SourceRecordID: "gcp-bigquery-" + accountID + "-" + row.Date + "-" + row.Currency,
		BillingScope: accountID, CostBasis: provider.CostBasisNet, Amount: amount, Currency: row.Currency,
		UsageStart: start, UsageEnd: start.AddDate(0, 0, 1),
	}, nil
}

type bigQueryDailyQuery struct{}

func (bigQueryDailyQuery) queryDailyNetCosts(ctx context.Context, request queryRequest) ([]dailyCost, error) {
	credentials := os.Getenv(credentialsEnv)
	client, err := bigquery.NewClient(ctx, request.projectID, option.WithCredentialsJSON([]byte(credentials)))
	if err != nil {
		return nil, err
	}
	defer client.Close()
	sql, parameters := buildQuery(request)
	query := client.Query(sql)
	query.Parameters = parameters
	query.UseLegacySQL = false
	rowIterator, err := query.Read(ctx)
	if err != nil {
		return nil, err
	}
	var results []dailyCost
	for {
		var row dailyCost
		if err := rowIterator.Next(&row); err == iterator.Done {
			break
		} else if err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, nil
}

func buildQuery(request queryRequest) (string, []bigquery.QueryParameter) {
	qualifiedTable := fmt.Sprintf("`%s.%s.%s`", request.projectID, request.datasetID, request.tableID)
	sql := fmt.Sprintf(`SELECT FORMAT_DATE('%%Y-%%m-%%d', DATE(usage_start_time, 'UTC')) AS usage_date,
	  SUM(CAST(ROUND(cost * 1000000) AS INT64) + IFNULL((SELECT SUM(CAST(ROUND(credit.amount * 1000000) AS INT64)) FROM UNNEST(credits) AS credit), 0)) AS net_micros,
  currency
FROM %s
WHERE billing_account_id = @billing_account_id
  AND usage_start_time >= @start_time
  AND usage_start_time < @end_time
GROUP BY usage_date, currency
ORDER BY usage_date, currency`, qualifiedTable)
	parameters := []bigquery.QueryParameter{
		{Name: "billing_account_id", Value: request.accountID},
		{Name: "start_time", Value: request.start},
		{Name: "end_time", Value: request.end},
	}
	return sql, parameters
}
