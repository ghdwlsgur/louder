package ncpprovider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ghdwlsgur/louder/internal/provider"
)

const defaultBillingEndpoint = "https://billingapi.apigw.ntruss.com/billing/v1"

type restSource struct {
	client   *http.Client
	endpoint string
	now      func() time.Time
}

func newRESTSource(client *http.Client, endpoint string, now func() time.Time) *restSource {
	return &restSource{client: client, endpoint: endpoint, now: now}
}

func (s *restSource) collect(ctx context.Context, request demandRequest, consume func(demandRow) error) error {
	accessKey, secretKey := os.Getenv("NCP_ACCESS_KEY"), os.Getenv("NCP_SECRET_KEY")
	if strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
	}
	endpoint := strings.TrimRight(s.endpoint, "/")
	if endpoint == "" {
		endpoint = defaultBillingEndpoint
	}
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	pageSize := 1000
	var expectedRows int
	collected := 0
	for pageNo := 1; ; pageNo++ {
		requestURL := *base
		requestURL.Path = strings.TrimRight(base.Path, "/") + "/cost/getDemandCostList"
		query := requestURL.Query()
		query.Set("startMonth", request.startMonth)
		query.Set("endMonth", request.endMonth)
		query.Set("pageNo", strconv.Itoa(pageNo))
		query.Set("pageSize", strconv.Itoa(pageSize))
		query.Set("responseFormatType", "json")
		requestURL.RawQuery = query.Encode()
		timestamp := strconv.FormatInt(now().UTC().UnixMilli(), 10)
		httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		httpRequest.Header.Set("x-ncp-apigw-timestamp", timestamp)
		httpRequest.Header.Set("x-ncp-iam-access-key", accessKey)
		httpRequest.Header.Set("x-ncp-apigw-signature-v2", makeSignature(http.MethodGet, httpRequest.URL.RequestURI(), timestamp, accessKey, secretKey))
		response, err := client.Do(httpRequest)
		if err != nil {
			return classifyError(err)
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			_ = response.Body.Close()
			return classifyHTTPStatus(response.StatusCode)
		}
		var result struct {
			Response struct {
				TotalRows      int `json:"totalRows"`
				DemandCostList []struct {
					MemberNo           string      `json:"memberNo"`
					DemandMonth        string      `json:"demandMonth"`
					AmountIncludingVAT json.Number `json:"thisMonthAmountIncludingVat"`
					PayCurrency        struct {
						Code string `json:"code"`
					} `json:"payCurrency"`
				} `json:"demandCostList"`
				ReturnCode string `json:"returnCode"`
			} `json:"getDemandCostListResponse"`
		}
		decoder := json.NewDecoder(io.LimitReader(response.Body, 16<<20))
		decoder.UseNumber()
		decodeErr := decoder.Decode(&result)
		_ = response.Body.Close()
		if decodeErr != nil || result.Response.TotalRows < 0 || result.Response.ReturnCode != "0" {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		if pageNo == 1 {
			expectedRows = result.Response.TotalRows
		} else if result.Response.TotalRows != expectedRows {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		if len(result.Response.DemandCostList) == 0 && collected < expectedRows {
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		for _, item := range result.Response.DemandCostList {
			if item.MemberNo == "" || item.DemandMonth == "" || item.AmountIncludingVAT == "" || item.PayCurrency.Code == "" {
				return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
			}
			if err := consume(demandRow{memberNo: item.MemberNo, demandMonth: item.DemandMonth, amountIncludingVAT: item.AmountIncludingVAT.String(), currency: item.PayCurrency.Code}); err != nil {
				return err
			}
			collected++
		}
		if collected >= expectedRows {
			return nil
		}
	}
}

func makeSignature(method, uri, timestamp, accessKey, secretKey string) string {
	message := method + " " + uri + "\n" + timestamp + "\n" + accessKey
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = io.WriteString(mac, message)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func classifyHTTPStatus(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return &provider.ProviderError{Class: provider.ErrorAuthenticationFailed}
	case http.StatusForbidden:
		return &provider.ProviderError{Class: provider.ErrorPermissionDenied}
	case http.StatusTooManyRequests:
		return &provider.ProviderError{Class: provider.ErrorRateLimited}
	default:
		return &provider.ProviderError{Class: provider.ErrorProviderUnavailable, Err: fmt.Errorf("NCP billing API returned HTTP %d", status)}
	}
}
