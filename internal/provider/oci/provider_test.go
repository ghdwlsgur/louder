package ociprovider

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/provider/contracttest"
)

type fakeUsageQuery struct {
	items []usageItem
	err   error
}

func (f fakeUsageQuery) queryDailyCosts(context.Context, usageRequest) ([]usageItem, error) {
	return f.items, f.err
}

func TestCollectCostsReturnsDailyTenancyCostWithExactDecimal(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{
		AccountID: "ocid1.tenancy.oc1..synthetictenancy",
		StartTime: start,
		EndTime:   start.AddDate(0, 0, 1),
	}
	adapter := newWithUsageQuery(fakeUsageQuery{items: []usageItem{{
		start:    start,
		amount:   "12.34567890123456789",
		currency: "USD",
	}}})

	records, err := adapter.CollectCosts(context.Background(), request)
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	want := provider.RawCostRecord{
		Provider:       "oci",
		SourceRecordID: "oci-usage-ocid1.tenancy.oc1..synthetictenancy-2026-09-29-USD",
		BillingScope:   request.AccountID,
		CostBasis:      provider.CostBasis("oci_cost"),
		Amount:         "12.34567890123456789",
		Currency:       "USD",
		UsageStart:     start,
		UsageEnd:       start.AddDate(0, 0, 1),
	}
	if len(records) != 1 || records[0] != want {
		t.Fatalf("CollectCosts() records = %#v, want %#v", records, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestOCIUsageQuerySignsDailyCostRequestAndFollowsPageHeader(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	creds := ociCredentials{tenantID: "ocid1.tenancy.oc1..synthetic", userID: "ocid1.user.oc1..synthetic", fingerprint: "aa:bb:cc", region: "us-ashburn-1", privateKey: privateKey}
	pages := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		pages++
		if r.Method != http.MethodPost || r.URL.Scheme != "https" || r.URL.Host != "usageapi.us-ashburn-1.oci.oraclecloud.com" || r.URL.Path != "/20200107/usage" {
			t.Errorf("request = %s %s, want OCI usage POST", r.Method, r.URL.String())
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), `Signature version="1",keyId="`+creds.tenantID+"/"+creds.userID+"/"+creds.fingerprint+`"`) {
			t.Errorf("Authorization header missing OCI signature: %q", r.Header.Get("Authorization"))
		}
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		bodyHash := sha256.Sum256(bodyBytes)
		if r.Header.Get("x-content-sha256") != base64.StdEncoding.EncodeToString(bodyHash[:]) {
			t.Error("x-content-sha256 does not match request body")
		}
		canonical := "date: " + r.Header.Get("date") + "\n" + "(request-target): post " + r.URL.RequestURI() + "\n" + "host: " + r.URL.Host + "\n" + "x-content-sha256: " + r.Header.Get("x-content-sha256") + "\n" + "content-type: application/json\n" + "content-length: " + r.Header.Get("content-length")
		digest := sha256.Sum256([]byte(canonical))
		signatureStart := strings.Index(r.Header.Get("Authorization"), `signature="`)
		if signatureStart < 0 {
			t.Error("Authorization header has no signature")
		} else {
			signatureText := strings.SplitN(r.Header.Get("Authorization")[signatureStart+len(`signature="`):], `"`, 2)[0]
			signature, decodeErr := base64.StdEncoding.DecodeString(signatureText)
			if decodeErr != nil || rsa.VerifyPKCS1v15(&privateKey.PublicKey, crypto.SHA256, digest[:], signature) != nil {
				t.Error("OCI request signature does not verify against the canonical request")
			}
		}
		var body usageRequestBody
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
		if body.TenantID != creds.tenantID || body.Granularity != "DAILY" || body.QueryType != "COST" || body.IsAggregateByTime || len(body.GroupBy) != 0 || body.TimeUsageStarted != start.Format(time.RFC3339) || body.TimeUsageEnded != start.AddDate(0, 0, 2).Add(-time.Microsecond).Format("2006-01-02T15:04:05.000000Z") {
			t.Errorf("request body = %#v", body)
		}
		response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}
		if pages == 1 {
			if r.URL.Query().Get("page") != "" {
				t.Errorf("first request page token = %q", r.URL.Query().Get("page"))
			}
			response.Header.Set("opc-next-page", "synthetic-page-2")
			response.Body = io.NopCloser(bytes.NewReader(readOCIQueryFixture(t, "usage-page-1.json")))
			return response, nil
		}
		if r.URL.Query().Get("page") != "synthetic-page-2" {
			t.Errorf("second request page token = %q", r.URL.Query().Get("page"))
		}
		response.Body = io.NopCloser(bytes.NewReader(readOCIQueryFixture(t, "usage-page-2.json")))
		return response, nil
	})}
	adapter := newWithUsageQuery(ociUsageQuery{client: client, creds: &creds})
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: creds.tenantID, StartTime: start, EndTime: start.AddDate(0, 0, 2)}
	records, err := adapter.CollectCosts(context.Background(), request)
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	if pages != 2 || len(records) != 2 || records[0].Amount != "12.34567890123456789" || records[1].Amount != "4.25" {
		t.Fatalf("pages=%d records=%#v, want two precision-preserved daily items", pages, records)
	}
}

func readOCIQueryFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read OCI fixture %q: %v", name, err)
	}
	return data
}

func TestCollectCostsRejectsDuplicateDailyCurrencyItems(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	row := usageItem{start: start, amount: "2.00", currency: "USD"}
	adapter := newWithUsageQuery(fakeUsageQuery{items: []usageItem{row, row}})
	request := provider.CollectRequest{AccountID: "ocid1.tenancy.oc1..synthetictenancy", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	if _, err := adapter.CollectCosts(context.Background(), request); !provider.IsErrorClass(err, provider.ErrorInvalidResponse) {
		t.Fatalf("CollectCosts() error = %v, want InvalidResponse for duplicate tenancy/day/currency", err)
	}
}

func TestOCIUsageQueryMapsHTTPFailures(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		class  provider.ErrorClass
	}{
		{name: "authentication", status: http.StatusUnauthorized, class: provider.ErrorAuthenticationFailed},
		{name: "permission", status: http.StatusForbidden, class: provider.ErrorPermissionDenied},
		{name: "rate limited", status: http.StatusTooManyRequests, class: provider.ErrorRateLimited},
		{name: "provider failure", status: http.StatusBadGateway, class: provider.ErrorProviderUnavailable},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
			if err != nil {
				t.Fatal(err)
			}
			creds := ociCredentials{tenantID: "ocid1.tenancy.oc1..synthetictenancy", userID: "ocid1.user.oc1..syntheticuser", fingerprint: "aa:bb", region: "us-ashburn-1", privateKey: privateKey}
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: scenario.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("synthetic error")), Request: r}, nil
			})}
			adapter := newWithUsageQuery(ociUsageQuery{client: client, creds: &creds})
			start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
			request := provider.CollectRequest{AccountID: creds.tenantID, StartTime: start, EndTime: start.AddDate(0, 0, 1)}
			_, err = adapter.CollectCosts(context.Background(), request)
			if !provider.IsErrorClass(err, scenario.class) {
				t.Fatalf("CollectCosts() error = %v, want %s", err, scenario.class)
			}
		})
	}
}

func TestOCIProviderContract(t *testing.T) {
	setValidOCIEnvironment(t)
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	request := provider.CollectRequest{AccountID: "ocid1.tenancy.oc1..synthetictenancy", StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	want := []provider.RawCostRecord{{Provider: "oci", SourceRecordID: "oci-usage-ocid1.tenancy.oc1..synthetictenancy-2026-09-29-USD", BillingScope: request.AccountID, CostBasis: provider.CostBasisOCI, Amount: "12.34", Currency: "USD", UsageStart: start, UsageEnd: start.AddDate(0, 0, 1)}}
	contracttest.Run(t, contracttest.Suite{
		Credentials: []contracttest.CredentialScenario{{Name: "API signing key environment", Provider: New()}},
		Collections: []contracttest.CollectionScenario{
			{Name: "daily tenancy cost", Provider: newWithUsageQuery(fakeUsageQuery{items: []usageItem{{start: start, amount: "12.34", currency: "USD"}}}), Request: request, WantRecords: want},
			{Name: "empty result", Provider: newWithUsageQuery(fakeUsageQuery{}), Request: request, WantRecords: []provider.RawCostRecord{}},
		},
	})
}

func setValidOCIEnvironment(t *testing.T) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	for name, value := range map[string]string{
		"OCI_TENANCY_OCID": "ocid1.tenancy.oc1..synthetictenancy",
		"OCI_USER_OCID":    "ocid1.user.oc1..syntheticuser",
		"OCI_FINGERPRINT":  "00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff",
		"OCI_REGION":       "us-ashburn-1",
		"OCI_PRIVATE_KEY":  string(privatePEM),
	} {
		t.Setenv(name, value)
	}
}

func TestOCIUsageQueryDoesNotReturnPartialPageResults(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	creds := ociCredentials{tenantID: "ocid1.tenancy.oc1..synthetictenancy", userID: "ocid1.user.oc1..syntheticuser", fingerprint: "aa:bb", region: "us-ashburn-1", privateKey: privateKey}
	pages := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		pages++
		response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}
		if pages == 1 {
			response.Header.Set("opc-next-page", "page-two")
			response.Body = io.NopCloser(bytes.NewReader(readOCIQueryFixture(t, "usage-page-1.json")))
			return response, nil
		}
		response.StatusCode = http.StatusServiceUnavailable
		return response, nil
	})}
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	adapter := newWithUsageQuery(ociUsageQuery{client: client, creds: &creds})
	request := provider.CollectRequest{AccountID: creds.tenantID, StartTime: start, EndTime: start.AddDate(0, 0, 2)}
	records, err := adapter.CollectCosts(context.Background(), request)
	if records != nil || !provider.IsErrorClass(err, provider.ErrorProviderUnavailable) || pages != 2 {
		t.Fatalf("CollectCosts() = (%#v, %v), pages=%d, want no partial rows and ProviderUnavailable", records, err, pages)
	}
}

func TestOCIUsageQueryRejectsRepeatedPageToken(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	creds := ociCredentials{tenantID: "ocid1.tenancy.oc1..synthetictenancy", userID: "ocid1.user.oc1..syntheticuser", fingerprint: "aa:bb", region: "us-ashburn-1", privateKey: privateKey}
	pages := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		pages++
		response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"items":[]}`)), Request: r}
		response.Header.Set("opc-next-page", "same-token")
		return response, nil
	})}
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	adapter := newWithUsageQuery(ociUsageQuery{client: client, creds: &creds})
	request := provider.CollectRequest{AccountID: creds.tenantID, StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	if _, err := adapter.CollectCosts(context.Background(), request); !provider.IsErrorClass(err, provider.ErrorInvalidResponse) || pages != 2 {
		t.Fatalf("CollectCosts() error=%v pages=%d, want InvalidResponse after detecting repeated token", err, pages)
	}
}

func TestOCIUsageQueryClassifiesRequestTimeout(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	creds := ociCredentials{tenantID: "ocid1.tenancy.oc1..synthetictenancy", userID: "ocid1.user.oc1..syntheticuser", fingerprint: "aa:bb", region: "us-ashburn-1", privateKey: privateKey}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	adapter := newWithUsageQuery(ociUsageQuery{client: client, creds: &creds})
	request := provider.CollectRequest{AccountID: creds.tenantID, StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	if _, err := adapter.CollectCosts(ctx, request); !provider.IsErrorClass(err, provider.ErrorTimeout) {
		t.Fatalf("CollectCosts() error = %v, want Timeout", err)
	}
}

func TestOCIUsageQueryRejectsMalformedResponse(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	creds := ociCredentials{tenantID: "ocid1.tenancy.oc1..synthetictenancy", userID: "ocid1.user.oc1..syntheticuser", fingerprint: "aa:bb", region: "us-ashburn-1", privateKey: privateKey}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"items":[{"computedAmount":null,"currency":"USD"}]}`)), Request: r}, nil
	})}
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	adapter := newWithUsageQuery(ociUsageQuery{client: client, creds: &creds})
	request := provider.CollectRequest{AccountID: creds.tenantID, StartTime: start, EndTime: start.AddDate(0, 0, 1)}
	if _, err := adapter.CollectCosts(context.Background(), request); !provider.IsErrorClass(err, provider.ErrorInvalidResponse) {
		t.Fatalf("CollectCosts() error = %v, want InvalidResponse", err)
	}
}

func TestValidateCredentialsRejectsMalformedKeyMetadata(t *testing.T) {
	setValidOCIEnvironment(t)
	t.Setenv("OCI_FINGERPRINT", "not-a-fingerprint")
	if err := New().ValidateCredentials(context.Background()); !provider.IsErrorClass(err, provider.ErrorInvalidCredentialShape) {
		t.Fatalf("ValidateCredentials() error = %v, want InvalidCredentialShape", err)
	}
}
