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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/shopspring/decimal"
)

const usageAPIVersion = "20200107"

var tenancyOCIDPattern = regexp.MustCompile(`^ocid1\.tenancy\.[a-zA-Z0-9-]+\.\.[a-zA-Z0-9_-]+$`)
var userOCIDPattern = regexp.MustCompile(`^ocid1\.user\.[a-zA-Z0-9-]+\.\.[a-zA-Z0-9_-]+$`)
var fingerprintPattern = regexp.MustCompile(`(?i)^(?:[0-9a-f]{2}:){15}[0-9a-f]{2}$`)
var regionPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

type usageItem struct {
	start    time.Time
	amount   string
	currency string
}

type usageQuery interface {
	queryDailyCosts(context.Context, usageRequest) ([]usageItem, error)
}

type usageRequest struct {
	tenantID string
	start    time.Time
	end      time.Time
}

type Provider struct{ query usageQuery }

func New() *Provider { return &Provider{query: ociUsageQuery{}} }

func newWithUsageQuery(query usageQuery) *Provider { return &Provider{query: query} }

func (*Provider) Metadata(context.Context) provider.ProviderMetadata {
	return provider.ProviderMetadata{Name: "oci"}
}

func (*Provider) ValidateCredentials(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	if _, err := credentialsFromEnvironment(); err != nil {
		return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
	}
	return nil
}

func (p *Provider) CollectCosts(ctx context.Context, request provider.CollectRequest) ([]provider.RawCostRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	if p.query == nil {
		return nil, fmt.Errorf("OCI Usage API client is required")
	}
	if !tenancyOCIDPattern.MatchString(request.AccountID) {
		return nil, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	start, end := request.StartTime.UTC(), request.EndTime.UTC()
	if !start.Before(end) || !isUTCMidnight(start) || !isUTCMidnight(end) {
		return nil, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	items, err := p.query.queryDailyCosts(ctx, usageRequest{tenantID: request.AccountID, start: start, end: end})
	if err != nil {
		return nil, classifyError(err)
	}
	byDateCurrency := make(map[string]provider.RawCostRecord, len(items))
	for _, item := range items {
		day := item.start.UTC().Truncate(24 * time.Hour)
		amount, parseErr := decimal.NewFromString(item.amount)
		if parseErr != nil || item.currency == "" || !day.Equal(item.start) || day.Before(start) || !day.Before(end) {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		key := day.Format("2006-01-02") + ":" + item.currency
		if _, exists := byDateCurrency[key]; exists {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		byDateCurrency[key] = provider.RawCostRecord{
			Provider: "oci", SourceRecordID: "oci-usage-" + request.AccountID + "-" + day.Format("2006-01-02") + "-" + item.currency,
			BillingScope: request.AccountID, CostBasis: provider.CostBasisOCI, Amount: amount.String(), Currency: item.currency,
			UsageStart: day, UsageEnd: day.AddDate(0, 0, 1),
		}
	}
	records := make([]provider.RawCostRecord, 0, len(byDateCurrency))
	for _, record := range byDateCurrency {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].SourceRecordID < records[j].SourceRecordID })
	return records, nil
}

func isUTCMidnight(value time.Time) bool {
	return value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
}

type ociCredentials struct {
	tenantID, userID, fingerprint, region string
	privateKey                            *rsa.PrivateKey
}

func credentialsFromEnvironment() (ociCredentials, error) {
	values := []string{os.Getenv("OCI_TENANCY_OCID"), os.Getenv("OCI_USER_OCID"), os.Getenv("OCI_FINGERPRINT"), os.Getenv("OCI_REGION"), os.Getenv("OCI_PRIVATE_KEY")}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return ociCredentials{}, errors.New("required OCI credential is missing")
		}
	}
	if !tenancyOCIDPattern.MatchString(values[0]) || !userOCIDPattern.MatchString(values[1]) || !fingerprintPattern.MatchString(values[2]) || !regionPattern.MatchString(values[3]) {
		return ociCredentials{}, errors.New("invalid OCI signing credential metadata")
	}
	block, _ := pem.Decode([]byte(values[4]))
	if block == nil || x509.IsEncryptedPEMBlock(block) {
		return ociCredentials{}, errors.New("OCI private key must be an unencrypted PEM key")
	}
	var key *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, _ = parsed.(*rsa.PrivateKey)
	}
	if key == nil {
		key, _ = x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	if key == nil || key.Validate() != nil {
		return ociCredentials{}, errors.New("invalid OCI RSA private key")
	}
	return ociCredentials{tenantID: values[0], userID: values[1], fingerprint: values[2], region: values[3], privateKey: key}, nil
}

type ociUsageQuery struct {
	endpoint string
	client   *http.Client
	creds    *ociCredentials
}

type usageRequestBody struct {
	TenantID          string   `json:"tenantId"`
	TimeUsageStarted  string   `json:"timeUsageStarted"`
	TimeUsageEnded    string   `json:"timeUsageEnded"`
	Granularity       string   `json:"granularity"`
	QueryType         string   `json:"queryType"`
	IsAggregateByTime bool     `json:"isAggregateByTime"`
	GroupBy           []string `json:"groupBy"`
}

type usageResponse struct {
	Items []struct {
		TimeUsageStarted string          `json:"timeUsageStarted"`
		ComputedAmount   json.RawMessage `json:"computedAmount"`
		Currency         string          `json:"currency"`
	} `json:"items"`
}

func (q ociUsageQuery) queryDailyCosts(ctx context.Context, request usageRequest) ([]usageItem, error) {
	creds := q.creds
	if creds == nil {
		loaded, err := credentialsFromEnvironment()
		if err != nil {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
		}
		creds = &loaded
	}
	endpoint := q.endpoint
	if endpoint == "" {
		endpoint = "https://usageapi." + creds.region + ".oci.oraclecloud.com"
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || parsedEndpoint.Scheme != "https" || parsedEndpoint.Host == "" || parsedEndpoint.User != nil {
		return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	body, err := json.Marshal(usageRequestBody{
		TenantID: request.tenantID, TimeUsageStarted: request.start.Format(time.RFC3339), TimeUsageEnded: request.end.Add(-time.Microsecond).Format("2006-01-02T15:04:05.000000Z"),
		Granularity: "DAILY", QueryType: "COST", IsAggregateByTime: false, GroupBy: []string{},
	})
	if err != nil {
		return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	client := q.client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	path := "/" + usageAPIVersion + "/usage"
	pageToken := ""
	seenTokens := make(map[string]struct{})
	items := make([]usageItem, 0)
	for page := 0; page < 100; page++ {
		target := path
		if pageToken != "" {
			queryValues := url.Values{"page": []string{pageToken}}
			target += "?" + queryValues.Encode()
		}
		httpRequest, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+target, bytes.NewReader(body))
		if requestErr != nil {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		if signErr := signRequest(httpRequest, body, *creds); signErr != nil {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
		}
		response, requestErr := client.Do(httpRequest)
		if requestErr != nil {
			return nil, classifyError(requestErr)
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			_ = response.Body.Close()
			return nil, classifyStatus(response.StatusCode)
		}
		var result usageResponse
		decodeErr := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&result)
		pageToken = response.Header.Get("opc-next-page")
		_ = response.Body.Close()
		if decodeErr != nil {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		for _, item := range result.Items {
			var number json.Number
			if err := json.Unmarshal(item.ComputedAmount, &number); err != nil {
				return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
			}
			started, err := time.Parse(time.RFC3339Nano, item.TimeUsageStarted)
			if err != nil {
				started, err = time.Parse("2006-01-02T15:04:05.000Z", item.TimeUsageStarted)
			}
			if err != nil {
				return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
			}
			items = append(items, usageItem{start: started.UTC(), amount: number.String(), currency: item.Currency})
		}
		if pageToken == "" {
			return items, nil
		}
		if _, exists := seenTokens[pageToken]; exists {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		seenTokens[pageToken] = struct{}{}
	}
	return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
}

func signRequest(request *http.Request, body []byte, credentials ociCredentials) error {
	bodyHash := sha256.Sum256(body)
	request.Header.Set("date", time.Now().UTC().Format(http.TimeFormat))
	request.Header.Set("x-content-sha256", base64.StdEncoding.EncodeToString(bodyHash[:]))
	request.Header.Set("content-type", "application/json")
	request.Header.Set("content-length", strconv.Itoa(len(body)))
	request.ContentLength = int64(len(body))
	canonical := "date: " + request.Header.Get("date") + "\n" +
		"(request-target): " + strings.ToLower(request.Method) + " " + request.URL.RequestURI() + "\n" +
		"host: " + request.URL.Host + "\n" +
		"x-content-sha256: " + request.Header.Get("x-content-sha256") + "\n" +
		"content-type: " + request.Header.Get("content-type") + "\n" +
		"content-length: " + request.Header.Get("content-length")
	digest := sha256.Sum256([]byte(canonical))
	signature, err := rsa.SignPKCS1v15(rand.Reader, credentials.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return err
	}
	keyID := credentials.tenantID + "/" + credentials.userID + "/" + credentials.fingerprint
	auth := `Signature version="1",keyId="` + keyID + `",algorithm="rsa-sha256",headers="date (request-target) host x-content-sha256 content-type content-length",signature="` + base64.StdEncoding.EncodeToString(signature) + `"`
	request.Header.Set("Authorization", auth)
	return nil
}

func classifyStatus(status int) error {
	class := provider.ErrorProviderUnavailable
	switch status {
	case http.StatusUnauthorized:
		class = provider.ErrorAuthenticationFailed
	case http.StatusForbidden:
		class = provider.ErrorPermissionDenied
	case http.StatusTooManyRequests:
		class = provider.ErrorRateLimited
	case http.StatusBadRequest:
		class = provider.ErrorInvalidResponse
	}
	return &provider.ProviderError{Class: class}
}

func classifyError(err error) error {
	var existing *provider.ProviderError
	if errors.As(err, &existing) {
		return err
	}
	var networkError net.Error
	if (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	return &provider.ProviderError{Class: provider.ErrorProviderUnavailable, Err: err}
}
