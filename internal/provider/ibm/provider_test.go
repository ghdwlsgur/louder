package ibmprovider

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/provider/contracttest"
)

type fakeFocusSource struct {
	rows []focusRow
	err  error
}

func (f fakeFocusSource) collect(_ context.Context, _ focusRequest, consume func(focusRow) error) error {
	if f.err != nil {
		return f.err
	}
	for _, row := range f.rows {
		if err := consume(row); err != nil {
			return err
		}
	}
	return nil
}

func TestCollectCostsAggregatesDailyFOCUSRowsWithoutLosingDecimalPrecision(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "ibm-account-123", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	adapter := newWithFocusSource(fakeFocusSource{rows: []focusRow{
		{start: start, end: start.AddDate(0, 0, 1), billedCost: "12.34567890123456789", currency: "USD"},
		{start: start, end: start.AddDate(0, 0, 1), billedCost: "0.00432109876543211", currency: "USD"},
	}})

	records, err := adapter.CollectCosts(context.Background(), request)
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	want := provider.RawCostRecord{Provider: "ibm", SourceRecordID: "ibm-focus-ibm-account-123-2026-09-29-USD", BillingScope: request.AccountID, CostBasis: provider.CostBasis("ibm_billed_cost"), Amount: "12.35", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1)}
	if len(records) != 1 || records[0] != want {
		t.Fatalf("CollectCosts() = %#v, want %#v", records, want)
	}
}

func TestCollectCostsRejectsFOCUSChargePeriodOverlappingWindowButNotDaily(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	rows := []focusRow{{start: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), end: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), billedCost: "90.00", currency: "USD"}}
	adapter := newWithFocusSource(fakeFocusSource{rows: rows})
	request := provider.CollectRequest{AccountID: "ibm-account-123", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	if _, err := adapter.CollectCosts(context.Background(), request); !provider.IsErrorClass(err, provider.ErrorInvalidResponse) {
		t.Fatalf("CollectCosts() error = %v, want InvalidResponse for overlapping non-daily charge period", err)
	}
}

func TestIBMRESTSourceExchangesAPIKeyAndStreamsAllOverlappingMonthlyFOCUSCSVs(t *testing.T) {
	t.Setenv("IBM_CLOUD_API_KEY", "synthetic-api-key")
	iamCalls, reportCalls := 0, 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "iam.test" {
			iamCalls++
			if r.Method != http.MethodPost || r.URL.Path != "/identity/token" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Errorf("IAM request = %s %s headers=%v", r.Method, r.URL, r.Header)
			}
			form, err := url.ParseQuery(mustReadBody(t, r))
			if err != nil || form.Get("grant_type") != "urn:ibm:params:oauth:grant-type:apikey" || form.Get("apikey") != "synthetic-api-key" {
				t.Errorf("IAM form = %v, err=%v", form, err)
			}
			return syntheticHTTPResponse(r, http.StatusOK, `{"access_token":"synthetic-token"}`, nil), nil
		}
		reportCalls++
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer synthetic-token" || r.Header.Get("Accept") != "text/csv" || r.URL.Query().Get("format") != "csv" {
			t.Errorf("FOCUS request = %s %s headers=%v", r.Method, r.URL, r.Header)
		}
		var fixture string
		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/v4/accounts/ibm-account-123/focus/2026-09":
			fixture = readIBMCsvFixture(t, "focus-september.csv")
		case "/v4/accounts/ibm-account-123/focus/2026-10":
			fixture = readIBMCsvFixture(t, "focus-october.csv")
		default:
			t.Errorf("unexpected IBM report path %q", r.URL.Path)
		}
		return syntheticHTTPResponse(r, http.StatusOK, fixture, http.Header{"Content-Type": []string{"text/csv"}}), nil
	})}
	source := newRESTSource(client, "https://iam.test", "https://billing.test")
	adapter := newWithFocusSource(source)
	start := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "ibm-account-123", StartTime: start, EndTime: start.AddDate(0, 0, 2)}
	records, err := adapter.CollectCosts(context.Background(), request)
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	if iamCalls != 1 || reportCalls != 2 || len(records) != 2 || records[0].Amount != "12.34000000000000001" || records[1].Amount != "4.25" {
		t.Fatalf("iamCalls=%d reportCalls=%d records=%#v", iamCalls, reportCalls, records)
	}
}

func readIBMCsvFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read IBM FOCUS fixture %q: %v", name, err)
	}
	return string(data)
}

func mustReadBody(t *testing.T, r *http.Request) string {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return string(data)
}

func syntheticHTTPResponse(r *http.Request, status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCollectCostsPreservesProviderErrorClassificationFromSource(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	adapter := newWithFocusSource(fakeFocusSource{err: &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}})
	request := provider.CollectRequest{AccountID: "ibm-account-123", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	if _, err := adapter.CollectCosts(context.Background(), request); !provider.IsErrorClass(err, provider.ErrorInvalidCredentialShape) {
		t.Fatalf("CollectCosts() error = %v, want InvalidCredentialShape", err)
	}
}

func TestIBMProviderContract(t *testing.T) {
	t.Setenv("IBM_CLOUD_API_KEY", "synthetic-api-key")
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "ibm-account-123", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	want := []provider.RawCostRecord{{Provider: "ibm", SourceRecordID: "ibm-focus-ibm-account-123-2026-09-29-USD", BillingScope: request.AccountID, CostBasis: provider.CostBasisIBMBilled, Amount: "12.34", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1)}}
	contracttest.Run(t, contracttest.Suite{
		Credentials: []contracttest.CredentialScenario{{Name: "API key secret", Provider: New()}},
		Collections: []contracttest.CollectionScenario{
			{Name: "daily billed cost", Provider: newWithFocusSource(fakeFocusSource{rows: []focusRow{{start: start, end: start.AddDate(0, 0, 1), billedCost: "12.34", currency: "USD"}}}), Request: request, WantRecords: want},
			{Name: "empty result", Provider: newWithFocusSource(fakeFocusSource{}), Request: request, WantRecords: []provider.RawCostRecord{}},
		},
	})
}

func TestIBMRESTSourceClassifiesIAMAndBillingHTTPFailures(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		status  int
		class   provider.ErrorClass
		failIAM bool
	}{
		{name: "IAM authentication", status: http.StatusBadRequest, class: provider.ErrorAuthenticationFailed, failIAM: true},
		{name: "billing authentication", status: http.StatusUnauthorized, class: provider.ErrorAuthenticationFailed},
		{name: "billing permission", status: http.StatusForbidden, class: provider.ErrorPermissionDenied},
		{name: "rate limit", status: http.StatusTooManyRequests, class: provider.ErrorRateLimited},
		{name: "upstream unavailable", status: http.StatusBadGateway, class: provider.ErrorProviderUnavailable},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Setenv("IBM_CLOUD_API_KEY", "synthetic-api-key")
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if scenario.failIAM || r.URL.Host == "billing.test" {
					return syntheticHTTPResponse(r, scenario.status, "safe synthetic body", nil), nil
				}
				return syntheticHTTPResponse(r, http.StatusOK, `{"access_token":"synthetic-token"}`, nil), nil
			})}
			adapter := newWithFocusSource(newRESTSource(client, "https://iam.test", "https://billing.test"))
			start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
			request := provider.CollectRequest{AccountID: "ibm-account-123", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
			_, err := adapter.CollectCosts(context.Background(), request)
			if !provider.IsErrorClass(err, scenario.class) {
				t.Fatalf("CollectCosts() error = %v, want %s", err, scenario.class)
			}
			if strings.Contains(err.Error(), "synthetic-api-key") || strings.Contains(err.Error(), "synthetic-token") || strings.Contains(err.Error(), "safe synthetic body") {
				t.Fatalf("CollectCosts() error reveals credential or response body: %v", err)
			}
		})
	}
}

func TestIBMRESTSourceReturnsNoPartialRowsWhenLaterMonthFails(t *testing.T) {
	t.Setenv("IBM_CLOUD_API_KEY", "synthetic-api-key")
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "iam.test" {
			return syntheticHTTPResponse(r, http.StatusOK, `{"access_token":"synthetic-token"}`, nil), nil
		}
		if strings.HasSuffix(r.URL.Path, "/2026-09") {
			return syntheticHTTPResponse(r, http.StatusOK, readIBMCsvFixture(t, "focus-september.csv"), nil), nil
		}
		return syntheticHTTPResponse(r, http.StatusServiceUnavailable, "", nil), nil
	})}
	adapter := newWithFocusSource(newRESTSource(client, "https://iam.test", "https://billing.test"))
	start := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "ibm-account-123", StartTime: start, EndTime: start.AddDate(0, 0, 2)}
	records, err := adapter.CollectCosts(context.Background(), request)
	if records != nil || !provider.IsErrorClass(err, provider.ErrorProviderUnavailable) {
		t.Fatalf("CollectCosts() = (%#v, %v), want no partial rows", records, err)
	}
}

func TestIBMRESTSourceRejectsMalformedFOCUSCSV(t *testing.T) {
	t.Setenv("IBM_CLOUD_API_KEY", "synthetic-api-key")
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "iam.test" {
			return syntheticHTTPResponse(r, http.StatusOK, `{"access_token":"synthetic-token"}`, nil), nil
		}
		return syntheticHTTPResponse(r, http.StatusOK, "not,a,FOCUS,report\n1,2,3,4\n", nil), nil
	})}
	adapter := newWithFocusSource(newRESTSource(client, "https://iam.test", "https://billing.test"))
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "ibm-account-123", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	if _, err := adapter.CollectCosts(context.Background(), request); !provider.IsErrorClass(err, provider.ErrorInvalidResponse) {
		t.Fatalf("CollectCosts() error = %v, want InvalidResponse", err)
	}
}

func TestIBMRESTSourceClassifiesRequestTimeout(t *testing.T) {
	t.Setenv("IBM_CLOUD_API_KEY", "synthetic-api-key")
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}
	adapter := newWithFocusSource(newRESTSource(client, "https://iam.test", "https://billing.test"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "ibm-account-123", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	if _, err := adapter.CollectCosts(ctx, request); !provider.IsErrorClass(err, provider.ErrorTimeout) {
		t.Fatalf("CollectCosts() error = %v, want Timeout", err)
	}
}

func TestCollectCostsKeepsCurrenciesSeparateWithinTheSameChargeDay(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	rows := []focusRow{
		{start: start, end: start.AddDate(0, 0, 1), billedCost: "12.34", currency: "USD"},
		{start: start, end: start.AddDate(0, 0, 1), billedCost: "10.00", currency: "EUR"},
	}
	request := provider.CollectRequest{AccountID: "ibm-account-123", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	records, err := newWithFocusSource(fakeFocusSource{rows: rows}).CollectCosts(context.Background(), request)
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	if len(records) != 2 || records[0].Currency != "EUR" || records[1].Currency != "USD" || records[0].SourceRecordID == records[1].SourceRecordID {
		t.Fatalf("CollectCosts() records = %#v, want separate stable currency records", records)
	}
}

func TestValidateCredentialsRejectsMissingIBMAPIKey(t *testing.T) {
	t.Setenv("IBM_CLOUD_API_KEY", "")
	if err := New().ValidateCredentials(context.Background()); !provider.IsErrorClass(err, provider.ErrorInvalidCredentialShape) {
		t.Fatalf("ValidateCredentials() error = %v, want InvalidCredentialShape", err)
	}
}
