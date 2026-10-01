package azureprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/provider/contracttest"
)

type fakeCostQuery struct {
	request queryRequest
	rows    []dailyCost
	err     error
}

func TestAzureQueryFollowsPagesAndMapsByColumnName(t *testing.T) {
	pageCount := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		pageCount++
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer synthetic-token" {
			t.Errorf("request method/auth = %s/%q", request.Method, request.Header.Get("Authorization"))
		}
		var definition queryDefinition
		if err := json.NewDecoder(request.Body).Decode(&definition); err != nil {
			t.Errorf("decode query body: %v", err)
		}
		if definition.Type != "ActualCost" || definition.Dataset.Granularity != "Daily" || definition.Dataset.Aggregation["totalCost"].Name != "PreTaxCost" {
			t.Errorf("query definition = %#v", definition)
		}
		if definition.Timeframe != "Custom" || !definition.TimePeriod.From.Equal(time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)) || definition.TimePeriod.To.Format(time.RFC3339Nano) != "2026-09-30T23:59:59.999999999Z" {
			t.Errorf("query time period = %#v", definition.TimePeriod)
		}
		response.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/subscriptions/00000000-0000-0000-0000-000000000001/providers/Microsoft.CostManagement/query" {
			response.WriteHeader(http.StatusOK)
			_, _ = response.Write([]byte(`{"properties":{"columns":[{"name":"UsageDate"},{"name":"Currency"},{"name":"PreTaxCost"}],"rows":[[20260929,"USD",12.345678]],"nextLink":"` + server.URL + `/next?api-version=2023-03-01&$skiptoken=synthetic"}}`))
			return
		}
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(`{"properties":{"columns":[{"name":"PreTaxCost"},{"name":"Currency"},{"name":"UsageDate"}],"rows":[[0.25,"USD",20260930]]}}`))
	}))
	defer server.Close()
	query := azureRESTQuery{endpoint: server.URL, httpClient: server.Client(), credential: syntheticCredential{}}
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	rows, err := query.queryDailyCosts(context.Background(), queryRequest{subscriptionID: "00000000-0000-0000-0000-000000000001", start: start, end: start.AddDate(0, 0, 2)})
	if err != nil {
		t.Fatalf("queryDailyCosts() error = %v", err)
	}
	if pageCount != 2 || len(rows) != 2 || rows[0].amount != "12.345678" || rows[1].date != "2026-09-30" {
		t.Fatalf("pages=%d rows=%#v", pageCount, rows)
	}
}

type syntheticCredential struct{}

func (syntheticCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "synthetic-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func TestAzureProviderContract(t *testing.T) {
	setCredentialEnvironment(t)
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "00000000-0000-0000-0000-000000000001", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	want := []provider.RawCostRecord{{Provider: "azure", SourceRecordID: "azure-cost-query-00000000-0000-0000-0000-000000000001-2026-09-29", BillingScope: request.AccountID, CostBasis: provider.CostBasisActualPreTax, Amount: "12.34", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1)}}
	contracttest.Run(t, contracttest.Suite{
		Credentials: []contracttest.CredentialScenario{{Name: "service principal environment", Provider: New()}},
		Collections: []contracttest.CollectionScenario{{Name: "daily actual cost", Provider: newWithQuery(&fakeCostQuery{rows: []dailyCost{{date: "2026-09-29", amount: "12.34", currency: "USD"}}}), Request: request, WantRecords: want}},
	})
}

func TestValidateCredentialsRejectsMissingServicePrincipalValues(t *testing.T) {
	t.Setenv("AZURE_TENANT_ID", "")
	t.Setenv("AZURE_CLIENT_ID", "")
	t.Setenv("AZURE_CLIENT_SECRET", "")
	if err := New().ValidateCredentials(context.Background()); !provider.IsErrorClass(err, provider.ErrorInvalidCredentialShape) {
		t.Fatalf("ValidateCredentials() error = %v, want InvalidCredentialShape", err)
	}
}

func TestAzureQueryRejectsOffHostPaginationLink(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests++
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"properties":{"columns":[{"name":"PreTaxCost"},{"name":"UsageDate"},{"name":"Currency"}],"rows":[],"nextLink":"https://attacker.example/steal-token"}}`))
	}))
	defer server.Close()
	query := azureRESTQuery{endpoint: server.URL, httpClient: server.Client(), credential: syntheticCredential{}}
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	_, err := query.queryDailyCosts(context.Background(), queryRequest{subscriptionID: "00000000-0000-0000-0000-000000000001", start: start, end: start.AddDate(0, 0, 1)})
	if !provider.IsErrorClass(err, provider.ErrorInvalidResponse) || requests != 1 {
		t.Fatalf("queryDailyCosts() = error %v after %d requests, want InvalidResponse after one request", err, requests)
	}
}

func setCredentialEnvironment(t *testing.T) {
	t.Helper()
	for key, value := range map[string]string{"AZURE_TENANT_ID": "00000000-0000-0000-0000-000000000002", "AZURE_CLIENT_ID": "00000000-0000-0000-0000-000000000003", "AZURE_CLIENT_SECRET": "synthetic-secret"} {
		previous, existed := os.LookupEnv(key)
		if err := os.Setenv(key, value); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(key, previous)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}

func TestCollectCostsRejectsInvalidSubscriptionScope(t *testing.T) {
	query := &fakeCostQuery{}
	_, err := newWithQuery(query).CollectCosts(context.Background(), provider.CollectRequest{AccountID: "not-a-subscription"})
	if !provider.IsErrorClass(err, provider.ErrorUnsupportedBillingScope) {
		t.Fatalf("CollectCosts() error = %v, want UnsupportedBillingScope", err)
	}
	if query.request.subscriptionID != "" {
		t.Fatalf("query invoked for invalid subscription %q", query.request.subscriptionID)
	}
}

func TestCollectCostsClassifiesAzureRateLimiting(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	query := &fakeCostQuery{err: &azcore.ResponseError{StatusCode: http.StatusTooManyRequests}}
	request := provider.CollectRequest{AccountID: "00000000-0000-0000-0000-000000000001", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	_, err := newWithQuery(query).CollectCosts(context.Background(), request)
	if !provider.IsErrorClass(err, provider.ErrorRateLimited) {
		t.Fatalf("CollectCosts() error = %v, want RateLimited", err)
	}
}

func (f *fakeCostQuery) queryDailyCosts(_ context.Context, request queryRequest) ([]dailyCost, error) {
	f.request = request
	return f.rows, f.err
}

func TestCollectCostsQueriesSubscriptionActualCostForRequestedDays(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	query := &fakeCostQuery{rows: []dailyCost{{date: "2026-09-29", amount: "12.345678", currency: "USD"}}}
	p := newWithQuery(query)
	request := provider.CollectRequest{AccountID: "00000000-0000-0000-0000-000000000001", StartTime: start, EndTime: start.AddDate(0, 0, 1)}

	records, err := p.CollectCosts(context.Background(), request)
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	want := provider.RawCostRecord{Provider: "azure", SourceRecordID: "azure-cost-query-00000000-0000-0000-0000-000000000001-2026-09-29", BillingScope: request.AccountID, CostBasis: provider.CostBasisActualPreTax, Amount: "12.345678", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1)}
	if len(records) != 1 || records[0] != want {
		t.Fatalf("CollectCosts() records = %#v, want %#v", records, []provider.RawCostRecord{want})
	}
	if query.request.subscriptionID != request.AccountID || !query.request.start.Equal(request.StartTime) || !query.request.end.Equal(request.EndTime) {
		t.Errorf("query request = %#v", query.request)
	}
}
