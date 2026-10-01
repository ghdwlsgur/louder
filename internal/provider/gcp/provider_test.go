package gcpprovider

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/provider/contracttest"
	"google.golang.org/api/googleapi"
)

type fakeDailyQuery struct {
	request queryRequest
	rows    []dailyCost
	err     error
}

func TestValidateCredentialsRequiresServiceAccountJSON(t *testing.T) {
	previous, existed := os.LookupEnv(credentialsEnv)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(credentialsEnv, previous)
		} else {
			_ = os.Unsetenv(credentialsEnv)
		}
	})
	_ = os.Setenv(credentialsEnv, `{"type":"service_account","client_email":"collector@example.iam.gserviceaccount.com","private_key":"synthetic"}`)
	if err := New().ValidateCredentials(context.Background()); err != nil {
		t.Fatalf("ValidateCredentials() error = %v", err)
	}
	_ = os.Setenv(credentialsEnv, `{}`)
	if err := New().ValidateCredentials(context.Background()); !provider.IsErrorClass(err, provider.ErrorInvalidCredentialShape) {
		t.Fatalf("ValidateCredentials() error = %v, want InvalidCredentialShape", err)
	}
}

func TestCollectCostsRejectsUnsafeTableIdentifier(t *testing.T) {
	query := &fakeDailyQuery{}
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	p := newWithQuery(query)
	_, err := p.CollectCosts(context.Background(), provider.CollectRequest{AccountID: "ABC", StartTime: start, EndTime: start.AddDate(0, 0, 1), ProviderConfig: map[string]string{"projectId": "billing-query", "datasetId": "billing", "tableId": "export` WHERE TRUE --"}})
	if !provider.IsErrorClass(err, provider.ErrorUnsupportedBillingScope) {
		t.Fatalf("CollectCosts() error = %v, want UnsupportedBillingScope", err)
	}
	if query.request.tableID != "" {
		t.Fatalf("query invoked with table identifier %q", query.request.tableID)
	}
}

func (f *fakeDailyQuery) queryDailyNetCosts(_ context.Context, request queryRequest) ([]dailyCost, error) {
	f.request = request
	return f.rows, f.err
}

func TestCollectCostsClassifiesBigQueryPermissionFailures(t *testing.T) {
	query := &fakeDailyQuery{err: &googleapi.Error{Code: 403}}
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	p := newWithQuery(query)
	_, err := p.CollectCosts(context.Background(), provider.CollectRequest{AccountID: "ABC", StartTime: start, EndTime: start.AddDate(0, 0, 1), ProviderConfig: map[string]string{"projectId": "billing-query", "datasetId": "billing", "tableId": "export"}})
	if !provider.IsErrorClass(err, provider.ErrorPermissionDenied) {
		t.Fatalf("CollectCosts() error = %v, want PermissionDenied", err)
	}
}

func TestCollectCostsMapsBigQueryDailyNetTotals(t *testing.T) {
	query := &fakeDailyQuery{rows: []dailyCost{{Date: "2026-09-29", Micros: 1234567, Currency: "USD"}}}
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	p := newWithQuery(query)

	records, err := p.CollectCosts(context.Background(), provider.CollectRequest{AccountID: "ABC", StartTime: start, EndTime: end, ProviderConfig: map[string]string{"projectId": "billing-query", "datasetId": "billing", "tableId": "gcp_billing_export_v1_ABC"}})
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("CollectCosts() returned %d records, want 1", len(records))
	}
	want := provider.RawCostRecord{Provider: "gcp", SourceRecordID: "gcp-bigquery-ABC-2026-09-29-USD", BillingScope: "ABC", CostBasis: provider.CostBasisNet, Amount: "1.234567", Currency: "USD", UsageStart: start, UsageEnd: end}
	if records[0] != want {
		t.Errorf("record = %#v, want %#v", records[0], want)
	}
	if query.request.projectID != "billing-query" || query.request.datasetID != "billing" || query.request.tableID != "gcp_billing_export_v1_ABC" || query.request.accountID != "ABC" || !query.request.start.Equal(start) || !query.request.end.Equal(end) {
		t.Errorf("query request = %#v", query.request)
	}
}

func TestBigQueryProviderContract(t *testing.T) {
	previous, existed := os.LookupEnv(credentialsEnv)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(credentialsEnv, previous)
		} else {
			_ = os.Unsetenv(credentialsEnv)
		}
	})
	_ = os.Setenv(credentialsEnv, `{"type":"service_account","client_email":"collector@example.iam.gserviceaccount.com","private_key":"synthetic"}`)
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "ABC", StartTime: start, EndTime: start.AddDate(0, 0, 1), ProviderConfig: map[string]string{"projectId": "billing-query", "datasetId": "billing", "tableId": "export"}}
	want := []provider.RawCostRecord{{Provider: "gcp", SourceRecordID: "gcp-bigquery-ABC-2026-09-29-USD", BillingScope: "ABC", CostBasis: provider.CostBasisNet, Amount: "1.234567", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1)}}
	contracttest.Run(t, contracttest.Suite{
		Credentials: []contracttest.CredentialScenario{
			{Name: "service account credentials", Provider: newWithQuery(&fakeDailyQuery{})},
			{Name: "expired context", Provider: newWithQuery(&fakeDailyQuery{}), Context: canceledContext(), WantErrorClass: provider.ErrorTimeout},
		},
		Collections: []contracttest.CollectionScenario{
			{Name: "one account-day net total", Provider: newWithQuery(&fakeDailyQuery{rows: []dailyCost{{Date: "2026-09-29", Micros: 1234567, Currency: "USD"}}}), Request: request, WantRecords: want},
			{Name: "permission failure", Provider: newWithQuery(&fakeDailyQuery{err: &googleapi.Error{Code: 403}}), Request: request, WantErrorClass: provider.ErrorPermissionDenied},
		},
	})
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestBuildQuerySumsCostAndCreditsWithBoundedBillingScope(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := queryRequest{projectID: "billing-query", datasetID: "billing", tableID: "export", accountID: "ABC", start: start, end: start.AddDate(0, 0, 1)}
	sql, parameters := buildQuery(request)
	if !strings.Contains(sql, "SUM(CAST(ROUND(cost * 1000000) AS INT64)") || !strings.Contains(sql, "UNNEST(credits)") || !strings.Contains(sql, "billing_account_id = @billing_account_id") || !strings.Contains(sql, "usage_start_time < @end_time") {
		t.Fatalf("query does not include required net cost aggregation and bounds: %s", sql)
	}
	want := []bigquery.QueryParameter{{Name: "billing_account_id", Value: "ABC"}, {Name: "start_time", Value: start}, {Name: "end_time", Value: start.AddDate(0, 0, 1)}}
	if !reflect.DeepEqual(parameters, want) {
		t.Fatalf("query parameters = %#v, want %#v", parameters, want)
	}
}
