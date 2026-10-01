package alibabaprovider

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ghdwlsgur/louder/internal/provider"
)

const (
	defaultEndpoint = "https://business.aliyuncs.com"
	bssAPIVersion   = "2017-12-14"
)

type bssSource struct {
	client   *http.Client
	endpoint string
	now      func() time.Time
}

func newBSSSource(client *http.Client, now func() time.Time, endpoint string) *bssSource {
	return &bssSource{client: client, now: now, endpoint: endpoint}
}

func (s *bssSource) collect(ctx context.Context, request billRequest, consume func(billRow) error) error {
	accessKeyID := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
	accessKeySecret := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
	if strings.TrimSpace(accessKeyID) == "" || strings.TrimSpace(accessKeySecret) == "" {
		return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
	}
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	endpoint := s.endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	now := s.now
	if now == nil {
		now = time.Now
	}
	for day := request.start; day.Before(request.end); day = day.AddDate(0, 0, 1) {
		cycle := day.Format("2006-01")
		token := ""
		seen := make(map[string]struct{})
		for page := 0; page < 1000; page++ {
			requestNonce, err := nonce()
			if err != nil {
				return &provider.ProviderError{Class: provider.ErrorProviderUnavailable}
			}
			params := url.Values{
				"Action":           {"DescribeInstanceBill"},
				"AccessKeyId":      {accessKeyID},
				"BillingCycle":     {cycle},
				"BillingDate":      {day.Format("2006-01-02")},
				"Format":           {"JSON"},
				"Granularity":      {"DAILY"},
				"IsBillingItem":    {"false"},
				"MaxResults":       {"300"},
				"SignatureMethod":  {"HMAC-SHA1"},
				"SignatureNonce":   {requestNonce},
				"SignatureVersion": {"1.0"},
				"Timestamp":        {now().UTC().Format("2006-01-02T15:04:05Z")},
				"Version":          {bssAPIVersion},
			}
			if token != "" {
				params.Set("NextToken", token)
			}
			if err := sign(params, accessKeySecret); err != nil {
				return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
			}
			target := *base
			target.Path = strings.TrimRight(target.Path, "/") + "/"
			target.RawQuery = params.Encode()
			httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
			if err != nil {
				return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
			}
			response, err := client.Do(httpRequest)
			if err != nil {
				return classifyError(err)
			}
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				response.Body.Close()
				return classifyHTTPStatus(response.StatusCode)
			}
			var result billResponse
			decodeErr := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&result)
			response.Body.Close()
			if decodeErr != nil {
				return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
			}
			if !result.Success || result.Data == nil {
				return classifyAPIErrorCode(result.Code)
			}
			if result.Data.AccountID != "" && result.Data.AccountID != request.accountID {
				return &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
			}
			for _, item := range result.Data.Items {
				amount := string(item.PretaxAmount)
				if amount == "" || item.Currency == "" || item.BillingDate == "" {
					return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
				}
				if err := consume(billRow{billingDate: item.BillingDate, amount: amount, currency: item.Currency}); err != nil {
					return err
				}
			}
			next := result.Data.NextToken
			if next == "" {
				break
			}
			if _, exists := seen[next]; exists || next == token {
				return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
			}
			seen[next] = struct{}{}
			token = next
			if page == 999 {
				return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
			}
		}
	}
	return nil
}

type billResponse struct {
	Code    string `json:"Code"`
	Success bool   `json:"Success"`
	Data    *struct {
		AccountID string `json:"AccountID"`
		NextToken string `json:"NextToken"`
		Items     []struct {
			BillingDate  string          `json:"BillingDate"`
			PretaxAmount json.RawMessage `json:"PretaxAmount"`
			Currency     string          `json:"Currency"`
		} `json:"Items"`
	} `json:"Data"`
}

func nonce() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func sign(params url.Values, secret string) error {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "Signature" {
			continue
		}
		pairs = append(pairs, rfc3986(key)+"="+rfc3986(params.Get(key)))
	}
	canonical := strings.Join(pairs, "&")
	stringToSign := "GET&%2F&" + rfc3986(canonical)
	// The BSS RPC v1 signature scheme requires HMAC-SHA1.
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	if _, err := mac.Write([]byte(stringToSign)); err != nil {
		return err
	}
	params.Set("Signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return nil
}

func rfc3986(value string) string {
	encoded := url.QueryEscape(value)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
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
		return &provider.ProviderError{Class: provider.ErrorProviderUnavailable}
	}
}

func classifyAPIErrorCode(code string) error {
	code = strings.ToLower(code)
	if code == "" {
		return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	switch {
	case strings.Contains(code, "invalidaccesskey"), strings.Contains(code, "signaturedoesnotmatch"), strings.Contains(code, "invalidsecuritytoken"):
		return &provider.ProviderError{Class: provider.ErrorAuthenticationFailed}
	case strings.Contains(code, "forbidden"), strings.Contains(code, "nopermission"), strings.Contains(code, "unauthorized"):
		return &provider.ProviderError{Class: provider.ErrorPermissionDenied}
	case strings.Contains(code, "throttl"), strings.Contains(code, "toomanyrequests"):
		return &provider.ProviderError{Class: provider.ErrorRateLimited}
	default:
		return &provider.ProviderError{Class: provider.ErrorProviderUnavailable}
	}
}
