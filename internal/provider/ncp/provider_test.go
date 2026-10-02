package ncpprovider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/provider/contracttest"
)

type fakeDemandSource struct {
	rows []demandRow
	err  error
	req  demandRequest
}

type ncpRoundTripFunc func(*http.Request) (*http.Response, error)

func (f ncpRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestNCPRESTSourceSignsAndPaginatesMonthlyBillingRequests(t *testing.T) {
	t.Setenv("NCP_ACCESS_KEY", "synthetic-access-key")
	t.Setenv("NCP_SECRET_KEY", "synthetic-secret-key")
	fixedNow := time.Date(2026, 10, 2, 3, 4, 5, 6_000_000, time.UTC)
	pageCalls := 0
	client := &http.Client{Transport: ncpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		pageCalls++
		if request.Method != http.MethodGet || request.URL.Path != "/billing/v1/cost/getDemandCostList" {
			t.Errorf("request = %s %s, want GET billing cost endpoint", request.Method, request.URL)
		}
		query := request.URL.Query()
		if query.Get("startMonth") != "202609" || query.Get("endMonth") != "202610" || query.Get("pageSize") != "1000" || query.Get("responseFormatType") != "json" {
			t.Errorf("query = %v", query)
		}
		if got := request.Header.Get("x-ncp-apigw-timestamp"); got != "1790910245006" {
			t.Errorf("timestamp = %q", got)
		}
		if request.Header.Get("x-ncp-iam-access-key") != "synthetic-access-key" {
			t.Errorf("access key header = %q", request.Header.Get("x-ncp-iam-access-key"))
		}
		message := request.Method + " " + request.URL.RequestURI() + "\n" + request.Header.Get("x-ncp-apigw-timestamp") + "\nsynthetic-access-key"
		mac := hmac.New(sha256.New, []byte("synthetic-secret-key"))
		_, _ = io.WriteString(mac, message)
		if got, want := request.Header.Get("x-ncp-apigw-signature-v2"), base64.StdEncoding.EncodeToString(mac.Sum(nil)); got != want {
			t.Errorf("signature = %q, want %q", got, want)
		}
		page := query.Get("pageNo")
		fixtureName := "demand-cost-page-1.json"
		if page == "2" {
			fixtureName = "demand-cost-page-2.json"
		}
		body := readNCPFixture(t, fixtureName)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	source := newRESTSource(client, "https://billing.test/billing/v1", func() time.Time { return fixedNow })
	var rows []demandRow
	err := source.collect(context.Background(), demandRequest{accountID: "2760000", startMonth: "202609", endMonth: "202610"}, func(row demandRow) error {
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}
	if pageCalls != 2 || len(rows) != 2 || rows[0].memberNo != "2760000" || rows[0].amountIncludingVAT != "12.34" || rows[1].demandMonth != "202610" || rows[1].currency != "USD" {
		t.Fatalf("pageCalls=%d rows=%#v", pageCalls, rows)
	}
}

func readNCPFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read NCP fixture %q: %v", name, err)
	}
	return string(data)
}

func (f *fakeDemandSource) collect(_ context.Context, request demandRequest, consume func(demandRow) error) error {
	f.req = request
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

func TestCollectCostsAggregatesMonthlyInvoiceAmountsByCurrency(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	source := &fakeDemandSource{rows: []demandRow{
		{memberNo: "2760000", demandMonth: "202609", amountIncludingVAT: "12.34000000000000001", currency: "KRW"},
		{memberNo: "2760000", demandMonth: "202609", amountIncludingVAT: "0.00999999999999999", currency: "KRW"},
		{memberNo: "2760000", demandMonth: "202609", amountIncludingVAT: "2.50", currency: "USD"},
	}}
	adapter := newWithDemandSource(source)
	request := provider.CollectRequest{AccountID: "2760000", StartTime: start, EndTime: start.AddDate(0, 0, 8)}

	records, err := adapter.CollectCosts(context.Background(), request)
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("CollectCosts() returned %d records, want 2: %#v", len(records), records)
	}
	want := map[string]provider.RawCostRecord{
		"KRW": {Provider: "ncp", SourceRecordID: "ncp-monthly-2760000-2026-09-KRW", BillingScope: "2760000", CostBasis: provider.CostBasisNCPMonthly, Amount: "12.35", Currency: "KRW", UsageStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), UsageEnd: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		"USD": {Provider: "ncp", SourceRecordID: "ncp-monthly-2760000-2026-09-USD", BillingScope: "2760000", CostBasis: provider.CostBasisNCPMonthly, Amount: "2.5", Currency: "USD", UsageStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), UsageEnd: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, record := range records {
		if expected := want[record.Currency]; record != expected {
			t.Errorf("record for %s = %#v, want %#v", record.Currency, record, expected)
		}
	}
	if source.req.startMonth != "202609" || source.req.endMonth != "202610" {
		t.Errorf("requested months = %s..%s, want 202609..202610", source.req.startMonth, source.req.endMonth)
	}
}

func TestCollectCostsRejectsAnotherAccountInMonthlyBillingResponse(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	adapter := newWithDemandSource(&fakeDemandSource{rows: []demandRow{{memberNo: "other-account", demandMonth: "202609", amountIncludingVAT: "12.34", currency: "KRW"}}})
	request := provider.CollectRequest{AccountID: "2760000", StartTime: start, EndTime: start.AddDate(0, 1, 0)}
	if _, err := adapter.CollectCosts(context.Background(), request); !provider.IsErrorClass(err, provider.ErrorInvalidResponse) {
		t.Fatalf("CollectCosts() error = %v, want InvalidResponse for account mismatch", err)
	}
}

func TestCollectCostsRequestsOnlyMonthsIntersectingTheHalfOpenWindow(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	source := &fakeDemandSource{rows: []demandRow{
		{memberNo: "2760000", demandMonth: "202609", amountIncludingVAT: "10", currency: "KRW"},
	}}
	adapter := newWithDemandSource(source)
	request := provider.CollectRequest{AccountID: "2760000", StartTime: start, EndTime: start.AddDate(0, 0, 2)}
	records, err := adapter.CollectCosts(context.Background(), request)
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	if source.req.startMonth != "202609" || source.req.endMonth != "202609" {
		t.Fatalf("requested months = %s..%s, want only 202609", source.req.startMonth, source.req.endMonth)
	}
	if len(records) != 1 || records[0].UsageStart.Month() != time.September {
		t.Fatalf("CollectCosts() records = %#v, want only September", records)
	}
}

func TestValidateCredentialsRequiresBothSignatureKeys(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		accessKey string
		secretKey string
		valid     bool
	}{
		{name: "missing access key", secretKey: "secret"},
		{name: "missing secret key", accessKey: "access"},
		{name: "both keys", accessKey: "access", secretKey: "secret", valid: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Setenv("NCP_ACCESS_KEY", scenario.accessKey)
			t.Setenv("NCP_SECRET_KEY", scenario.secretKey)
			err := New().ValidateCredentials(context.Background())
			if scenario.valid && err != nil {
				t.Fatalf("ValidateCredentials() error = %v, want nil", err)
			}
			if !scenario.valid && !provider.IsErrorClass(err, provider.ErrorInvalidCredentialShape) {
				t.Fatalf("ValidateCredentials() error = %v, want InvalidCredentialShape", err)
			}
		})
	}
}

func TestNCPProviderContract(t *testing.T) {
	t.Setenv("NCP_ACCESS_KEY", "synthetic-access-key")
	t.Setenv("NCP_SECRET_KEY", "synthetic-secret-key")
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "2760000", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	want := []provider.RawCostRecord{{
		Provider: "ncp", SourceRecordID: "ncp-monthly-2760000-2026-09-KRW", BillingScope: request.AccountID,
		CostBasis: provider.CostBasisNCPMonthly, Amount: "12.34", Currency: "KRW",
		UsageStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), UsageEnd: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}}
	contracttest.Run(t, contracttest.Suite{
		Credentials: []contracttest.CredentialScenario{{Name: "Access Key and Secret Key", Provider: New()}},
		Collections: []contracttest.CollectionScenario{
			{Name: "monthly invoice amount", Provider: newWithDemandSource(&fakeDemandSource{rows: []demandRow{{memberNo: "2760000", demandMonth: "202609", amountIncludingVAT: "12.34", currency: "KRW"}}}), Request: request, WantRecords: want},
			{Name: "empty invoice list", Provider: newWithDemandSource(&fakeDemandSource{}), Request: request, WantRecords: []provider.RawCostRecord{}},
		},
	})
}

func TestNCPRESTSourceClassifiesHTTPFailuresWithoutLeakingCredentials(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		class  provider.ErrorClass
	}{
		{name: "authentication", status: http.StatusUnauthorized, class: provider.ErrorAuthenticationFailed},
		{name: "permission", status: http.StatusForbidden, class: provider.ErrorPermissionDenied},
		{name: "rate limit", status: http.StatusTooManyRequests, class: provider.ErrorRateLimited},
		{name: "server failure", status: http.StatusBadGateway, class: provider.ErrorProviderUnavailable},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			const secret = "synthetic-secret-that-must-not-leak"
			t.Setenv("NCP_ACCESS_KEY", "synthetic-access-key")
			t.Setenv("NCP_SECRET_KEY", secret)
			client := &http.Client{Transport: ncpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: scenario.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(secret)), Request: request}, nil
			})}
			source := newRESTSource(client, "https://billing.test/billing/v1", nil)
			err := source.collect(context.Background(), demandRequest{accountID: "2760000", startMonth: "202609", endMonth: "202609"}, func(demandRow) error { return nil })
			if !provider.IsErrorClass(err, scenario.class) {
				t.Fatalf("collect() error = %v, want %s", err, scenario.class)
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "synthetic-access-key") {
				t.Fatalf("collect() error leaked an API credential: %v", err)
			}
		})
	}
}
