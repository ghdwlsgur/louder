package alibabaprovider

import (
	"context"
	"errors"
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

type fakeBillSource struct {
	rows []billRow
	err  error
}

func (f fakeBillSource) collect(_ context.Context, _ billRequest, consume func(billRow) error) error {
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

func TestCollectCostsAggregatesDailyPayableAmountsWithExactDecimals(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "1234567890123456", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	adapter := newWithBillSource(fakeBillSource{rows: []billRow{
		{billingDate: "2026-09-29", amount: "12.34567890123456789", currency: "USD"},
		{billingDate: "2026-09-29", amount: "0.00432109876543211", currency: "USD"},
	}})

	records, err := adapter.CollectCosts(context.Background(), request)
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	want := provider.RawCostRecord{
		Provider: "alibaba", SourceRecordID: "alibaba-instance-1234567890123456-2026-09-29-USD",
		BillingScope: request.AccountID, CostBasis: provider.CostBasis("alibaba_pretax_cost"),
		Amount: "12.35", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1),
	}
	if len(records) != 1 || records[0] != want {
		t.Fatalf("CollectCosts() = %#v, want %#v", records, want)
	}
}

func TestCollectCostsKeepsCurrenciesSeparateAndReturnsNoPartialRowsOnFailure(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "1234567890123456", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	source := fakeBillSource{rows: []billRow{
		{billingDate: "2026-09-29", amount: "12.34", currency: "USD"},
		{billingDate: "2026-09-29", amount: "10.00", currency: "CNY"},
	}}
	records, err := newWithBillSource(source).CollectCosts(context.Background(), request)
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	if len(records) != 2 || records[0].Currency != "CNY" || records[1].Currency != "USD" || records[0].SourceRecordID == records[1].SourceRecordID {
		t.Fatalf("CollectCosts() = %#v, want separate daily records by currency", records)
	}

	partial := partialFailureBillSource{}
	if records, err := newWithBillSource(partial).CollectCosts(context.Background(), request); records != nil || !provider.IsErrorClass(err, provider.ErrorPermissionDenied) {
		t.Fatalf("CollectCosts() = (%#v, %v), want no partial records and PermissionDenied", records, err)
	}
}

type partialFailureBillSource struct{}

func (partialFailureBillSource) collect(_ context.Context, _ billRequest, consume func(billRow) error) error {
	if err := consume(billRow{billingDate: "2026-09-29", amount: "12.34", currency: "USD"}); err != nil {
		return err
	}
	return &provider.ProviderError{Class: provider.ErrorPermissionDenied}
}

func TestSignMatchesAlibabaRPCSignatureVector(t *testing.T) {
	params := url.Values{
		"AccessKeyId": {"testid"}, "Action": {"DescribeDedicatedHosts"}, "Format": {"JSON"},
		"RegionId": {"cn-beijing"}, "SignatureMethod": {"HMAC-SHA1"},
		"SignatureNonce": {"edb2b34af0af9a6d14deaf7c1a5315eb"}, "SignatureVersion": {"1.0"},
		"Timestamp": {"2023-03-13T08:34:30Z"}, "Version": {"2014-05-26"},
	}
	if err := sign(params, "testsecret"); err != nil {
		t.Fatalf("sign() error = %v", err)
	}
	if got, want := params.Get("Signature"), "9NaGiOspFP5UPcwX8Iwt2YJXXuk="; got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

func TestBSSSourcePaginatesEachDailyDateAcrossMonthBoundary(t *testing.T) {
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "synthetic-key-id")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "synthetic-key-secret")
	var requests []url.Values
	client := &http.Client{Transport: bssRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		params, err := url.ParseQuery(request.URL.RawQuery)
		if err != nil {
			t.Fatalf("parse query: %v", err)
		}
		requests = append(requests, params)
		var response string
		if params.Get("BillingDate") == "2026-09-30" && params.Get("NextToken") == "" {
			response = readBSSFixture(t, "daily-page-1.json")
		} else if params.Get("BillingDate") == "2026-09-30" && params.Get("NextToken") == "page-2" {
			response = readBSSFixture(t, "daily-page-2.json")
		} else if params.Get("BillingDate") == "2026-10-01" && params.Get("NextToken") == "" {
			response = `{"Success":true,"Data":{"AccountID":"1234567890123456","Items":[{"BillingDate":"2026-10-01","PretaxAmount":4,"Currency":"CNY","Item":"PayAsYouGoBill","InstanceID":"i-3"}]}}`
		} else {
			t.Errorf("unexpected page request: %v", params)
			response = `{"Success":true,"Data":{"AccountID":"1234567890123456","Items":[]}}`
		}
		return bssResponse(request, http.StatusOK, response), nil
	})}
	source := newBSSSource(client, func() time.Time { return time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC) }, "https://bss.test")
	rows := make([]billRow, 0)
	err := source.collect(context.Background(), billRequest{
		accountID: "1234567890123456", start: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), end: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
	}, func(row billRow) error { rows = append(rows, row); return nil })
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}
	if len(requests) != 3 || len(rows) != 3 {
		t.Fatalf("requests=%d rows=%d; want 3 each", len(requests), len(rows))
	}
	if requests[0].Get("BillingCycle") != "2026-09" || requests[1].Get("NextToken") != "page-2" || requests[2].Get("BillingCycle") != "2026-10" {
		t.Fatalf("billing requests = %#v", requests)
	}
}

func TestBSSSourceClassifiesHTTPFailuresWithoutExposingResponseBody(t *testing.T) {
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "synthetic-key-id")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "synthetic-key-secret")
	for _, scenario := range []struct {
		status int
		class  provider.ErrorClass
	}{
		{status: http.StatusUnauthorized, class: provider.ErrorAuthenticationFailed},
		{status: http.StatusForbidden, class: provider.ErrorPermissionDenied},
		{status: http.StatusTooManyRequests, class: provider.ErrorRateLimited},
		{status: http.StatusBadGateway, class: provider.ErrorProviderUnavailable},
	} {
		client := &http.Client{Transport: bssRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return bssResponse(request, scenario.status, "synthetic-private-response"), nil
		})}
		source := newBSSSource(client, time.Now, "https://bss.test")
		err := source.collect(context.Background(), billRequest{
			accountID: "1234567890123456", start: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), end: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		}, func(billRow) error { return nil })
		if !provider.IsErrorClass(err, scenario.class) {
			t.Fatalf("collect() error = %v, want %s", err, scenario.class)
		}
		if strings.Contains(err.Error(), "synthetic-private-response") {
			t.Fatalf("collect() error exposed response body: %v", err)
		}
	}
}

func TestBSSSourceClassifiesAlibabaRPCErrorCodes(t *testing.T) {
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "synthetic-key-id")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "synthetic-key-secret")
	for _, scenario := range []struct {
		code  string
		class provider.ErrorClass
	}{
		{code: "InvalidAccessKeyId.NotFound", class: provider.ErrorAuthenticationFailed},
		{code: "Forbidden.RAM", class: provider.ErrorPermissionDenied},
		{code: "Throttling.User", class: provider.ErrorRateLimited},
	} {
		client := &http.Client{Transport: bssRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return bssResponse(request, http.StatusOK, `{"Code":"`+scenario.code+`","Message":"synthetic sensitive message","Success":false}`), nil
		})}
		source := newBSSSource(client, time.Now, "https://bss.test")
		err := source.collect(context.Background(), billRequest{
			accountID: "1234567890123456", start: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), end: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		}, func(billRow) error { return nil })
		if !provider.IsErrorClass(err, scenario.class) {
			t.Fatalf("collect() error = %v, want %s", err, scenario.class)
		}
		if strings.Contains(err.Error(), "synthetic sensitive message") {
			t.Fatalf("collect() error exposed provider message: %v", err)
		}
	}
}

func TestBSSSourceRejectsMalformedResponsePayload(t *testing.T) {
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "synthetic-key-id")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "synthetic-key-secret")
	for _, body := range []string{
		"not-json",
		`{"Success":true,"Data":{"AccountID":"1234567890123456","Items":[{"BillingDate":"2026-09-30","PretaxAmount":1.25}]}}`,
	} {
		client := &http.Client{Transport: bssRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return bssResponse(request, http.StatusOK, body), nil
		})}
		source := newBSSSource(client, time.Now, "https://bss.test")
		err := source.collect(context.Background(), billRequest{
			accountID: "1234567890123456", start: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), end: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		}, func(billRow) error { return nil })
		if !provider.IsErrorClass(err, provider.ErrorInvalidResponse) {
			t.Fatalf("collect() error = %v, want InvalidResponse for %q", err, body)
		}
	}
}

func TestBSSSourceDoesNotExposeSignedURLInTransportErrors(t *testing.T) {
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "synthetic-key-id")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "synthetic-key-secret")
	client := &http.Client{Transport: bssRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, errors.New("synthetic transport failure")
	})}
	source := newBSSSource(client, time.Now, "https://bss.test")
	err := source.collect(context.Background(), billRequest{
		accountID: "1234567890123456", start: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), end: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}, func(billRow) error { return nil })
	if err == nil {
		t.Fatal("collect() error = nil, want transport failure")
	}
	for _, sensitive := range []string{"synthetic-key-id", "synthetic-key-secret", "Signature="} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("collect() error exposed %q: %v", sensitive, err)
		}
	}
}

func TestValidateCredentialsRequiresAlibabaAccessKeyPair(t *testing.T) {
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "synthetic-key-secret")
	if err := New().ValidateCredentials(context.Background()); !provider.IsErrorClass(err, provider.ErrorInvalidCredentialShape) {
		t.Fatalf("ValidateCredentials() error = %v, want InvalidCredentialShape", err)
	}
}

func TestAlibabaProviderContract(t *testing.T) {
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "synthetic-key-id")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "synthetic-key-secret")
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "1234567890123456", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	want := []provider.RawCostRecord{{
		Provider: "alibaba", SourceRecordID: "alibaba-instance-1234567890123456-2026-09-29-USD",
		BillingScope: request.AccountID, CostBasis: provider.CostBasisAlibabaPretax, Amount: "12.34", Currency: "USD",
		UsageStart: start, UsageEnd: start.AddDate(0, 0, 1),
	}}
	contracttest.Run(t, contracttest.Suite{
		Credentials: []contracttest.CredentialScenario{{Name: "AccessKey pair", Provider: New()}},
		Collections: []contracttest.CollectionScenario{
			{Name: "daily pretax bill", Provider: newWithBillSource(fakeBillSource{rows: []billRow{{billingDate: "2026-09-29", amount: "12.34", currency: "USD"}}}), Request: request, WantRecords: want},
			{Name: "empty bill", Provider: newWithBillSource(fakeBillSource{}), Request: request, WantRecords: []provider.RawCostRecord{}},
		},
	})
}

type bssRoundTripFunc func(*http.Request) (*http.Response, error)

func (f bssRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func bssResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}
}

func readBSSFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return string(data)
}
