package awsprovider

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/smithy-go"
	"github.com/ghdwlsgur/louder/internal/provider"
	"github.com/ghdwlsgur/louder/internal/provider/contracttest"
)

func TestCollectCostsQueriesUnblendedDailyAccountTotalsAndAllPages(t *testing.T) {
	firstToken := "page-two"
	client := &fakeCostExplorerClient{outputs: []*costexplorer.GetCostAndUsageOutput{
		{
			ResultsByTime: []types.ResultByTime{{
				TimePeriod: &types.DateInterval{Start: aws.String("2026-09-29"), End: aws.String("2026-09-30")},
				Total:      map[string]types.MetricValue{"UnblendedCost": {Amount: aws.String("12.3400"), Unit: aws.String("USD")}},
			}},
			NextPageToken: &firstToken,
		},
		{
			ResultsByTime: []types.ResultByTime{{
				TimePeriod: &types.DateInterval{Start: aws.String("2026-09-30"), End: aws.String("2026-10-01")},
				Total:      map[string]types.MetricValue{"UnblendedCost": {Amount: aws.String("0"), Unit: aws.String("USD")}},
			}},
		},
	}}
	collector := New(client, credentials.NewStaticCredentialsProvider("synthetic-access-key", "synthetic-secret-key", ""))
	request := provider.CollectRequest{
		AccountID:    "123456789012",
		StartTime:    time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC),
		EndTime:      time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
		CollectionID: "2026-09-29/2026-10-01",
	}

	records, err := collector.CollectCosts(context.Background(), request)
	if err != nil {
		t.Fatalf("CollectCosts() error = %v", err)
	}
	if len(client.inputs) != 2 {
		t.Fatalf("Cost Explorer calls = %d, want 2 pages", len(client.inputs))
	}
	first := client.inputs[0]
	if first.Granularity != types.GranularityDaily || len(first.Metrics) != 1 || first.Metrics[0] != "UnblendedCost" {
		t.Errorf("first request metric/granularity = (%v, %v), want (UnblendedCost, DAILY)", first.Metrics, first.Granularity)
	}
	if first.TimePeriod == nil || aws.ToString(first.TimePeriod.Start) != "2026-09-29" || aws.ToString(first.TimePeriod.End) != "2026-10-01" {
		t.Errorf("request period = %#v, want inclusive 2026-09-29 to exclusive 2026-10-01", first.TimePeriod)
	}
	if first.Filter == nil || first.Filter.Dimensions == nil || first.Filter.Dimensions.Key != types.DimensionLinkedAccount || len(first.Filter.Dimensions.Values) != 1 || first.Filter.Dimensions.Values[0] != request.AccountID {
		t.Errorf("account filter = %#v, want LINKED_ACCOUNT=%s", first.Filter, request.AccountID)
	}
	if client.inputs[1].NextPageToken == nil || *client.inputs[1].NextPageToken != firstToken {
		t.Errorf("second page token = %v, want %q", client.inputs[1].NextPageToken, firstToken)
	}
	want := []provider.RawCostRecord{
		{Provider: "aws", SourceRecordID: "aws-cost-explorer-123456789012-2026-09-29", BillingScope: request.AccountID, Amount: "12.3400", Currency: "USD", UsageStart: time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC), UsageEnd: time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)},
		{Provider: "aws", SourceRecordID: "aws-cost-explorer-123456789012-2026-09-30", BillingScope: request.AccountID, Amount: "0", Currency: "USD", UsageStart: time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC), UsageEnd: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)},
	}
	if len(records) != len(want) {
		t.Fatalf("CollectCosts() records = %#v, want %#v", records, want)
	}
	for i := range want {
		if records[i] != want[i] {
			t.Errorf("record[%d] = %#v, want %#v", i, records[i], want[i])
		}
	}
}

func TestCollectCostsClassifiesAWSErrorsWithoutExposingUpstreamMessages(t *testing.T) {
	tests := []struct {
		name      string
		awsCode   string
		wantClass provider.ErrorClass
	}{
		{name: "invalid authentication", awsCode: "UnrecognizedClientException", wantClass: provider.ErrorAuthenticationFailed},
		{name: "permission denied", awsCode: "AccessDeniedException", wantClass: provider.ErrorPermissionDenied},
		{name: "rate limited", awsCode: "LimitExceededException", wantClass: provider.ErrorRateLimited},
		{name: "service unavailable", awsCode: "ServiceUnavailableException", wantClass: provider.ErrorProviderUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeCostExplorerClient{errors: []error{&smithy.GenericAPIError{Code: test.awsCode, Message: "sensitive upstream details"}}}
			collector := New(client, credentials.NewStaticCredentialsProvider("synthetic-access-key", "synthetic-secret-key", ""))
			_, err := collector.CollectCosts(context.Background(), validRequest())
			if !provider.IsErrorClass(err, test.wantClass) {
				t.Fatalf("CollectCosts() error = %v, want class %q", err, test.wantClass)
			}
			if strings.Contains(err.Error(), "sensitive upstream details") {
				t.Errorf("error exposed upstream response message: %v", err)
			}
		})
	}
}

func TestCollectCostsRejectsMalformedDailyResult(t *testing.T) {
	client := &fakeCostExplorerClient{outputs: []*costexplorer.GetCostAndUsageOutput{{
		ResultsByTime: []types.ResultByTime{{
			TimePeriod: &types.DateInterval{Start: aws.String("2026-09-29"), End: aws.String("2026-09-30")},
			Total:      map[string]types.MetricValue{},
		}},
	}}}
	collector := New(client, credentials.NewStaticCredentialsProvider("synthetic-access-key", "synthetic-secret-key", ""))
	if records, err := collector.CollectCosts(context.Background(), validRequest()); !provider.IsErrorClass(err, provider.ErrorInvalidResponse) || len(records) != 0 {
		t.Fatalf("CollectCosts() = (%#v, %v), want no records and InvalidResponse", records, err)
	}
}

func TestCollectCostsRejectsInvalidAccountScopeBeforeCallingAWS(t *testing.T) {
	client := &fakeCostExplorerClient{}
	collector := New(client, credentials.NewStaticCredentialsProvider("synthetic-access-key", "synthetic-secret-key", ""))
	request := validRequest()
	request.AccountID = "not-an-account"
	if _, err := collector.CollectCosts(context.Background(), request); !provider.IsErrorClass(err, provider.ErrorUnsupportedBillingScope) {
		t.Fatalf("CollectCosts() error = %v, want UnsupportedBillingScope", err)
	}
	if len(client.inputs) != 0 {
		t.Errorf("Cost Explorer calls = %d, want 0 for invalid account scope", len(client.inputs))
	}
}

func TestValidateCredentialsRejectsMissingProvider(t *testing.T) {
	collector := New(&fakeCostExplorerClient{}, nil)
	if err := collector.ValidateCredentials(context.Background()); !provider.IsErrorClass(err, provider.ErrorInvalidCredentialShape) {
		t.Fatalf("ValidateCredentials() error = %v, want InvalidCredentialShape", err)
	}
}

func TestCostExplorerProviderContract(t *testing.T) {
	request := validRequest()
	want := []provider.RawCostRecord{{
		Provider:       "aws",
		SourceRecordID: "aws-cost-explorer-123456789012-2026-09-29",
		BillingScope:   request.AccountID,
		Amount:         "3.21",
		Currency:       "USD",
		UsageStart:     request.StartTime,
		UsageEnd:       request.EndTime,
	}}
	contracttest.Run(t, contracttest.Suite{
		Credentials: []contracttest.CredentialScenario{
			{Name: "valid static credentials", Provider: New(&fakeCostExplorerClient{}, credentials.NewStaticCredentialsProvider("synthetic-access-key", "synthetic-secret-key", ""))},
			{Name: "empty static credentials", Provider: New(&fakeCostExplorerClient{}, credentials.NewStaticCredentialsProvider("", "", "")), WantErrorClass: provider.ErrorInvalidCredentialShape},
			{Name: "missing credential source", Provider: New(&fakeCostExplorerClient{}, nil), WantErrorClass: provider.ErrorInvalidCredentialShape},
			{Name: "canceled credential check", Provider: New(&fakeCostExplorerClient{}, credentials.NewStaticCredentialsProvider("synthetic-access-key", "synthetic-secret-key", "")), Context: canceledContext(), WantErrorClass: provider.ErrorTimeout},
		},
		Collections: []contracttest.CollectionScenario{
			{Name: "one daily total", Provider: New(&fakeCostExplorerClient{outputs: []*costexplorer.GetCostAndUsageOutput{dailyOutput("2026-09-29", "2026-09-30", "3.21", "USD")}}, credentials.NewStaticCredentialsProvider("synthetic-access-key", "synthetic-secret-key", "")), Request: request, WantRecords: want},
			{Name: "empty result", Provider: New(&fakeCostExplorerClient{outputs: []*costexplorer.GetCostAndUsageOutput{{}}}, credentials.NewStaticCredentialsProvider("synthetic-access-key", "synthetic-secret-key", "")), Request: request, WantRecords: []provider.RawCostRecord{}},
			{Name: "rate limit", Provider: New(&fakeCostExplorerClient{errors: []error{&smithy.GenericAPIError{Code: "LimitExceededException"}}}, credentials.NewStaticCredentialsProvider("synthetic-access-key", "synthetic-secret-key", "")), Request: request, WantErrorClass: provider.ErrorRateLimited},
			{Name: "canceled request", Provider: New(&fakeCostExplorerClient{}, credentials.NewStaticCredentialsProvider("synthetic-access-key", "synthetic-secret-key", "")), Context: canceledContext(), Request: request, WantErrorClass: provider.ErrorTimeout},
		},
	})
}

func dailyOutput(start, end, amount, currency string) *costexplorer.GetCostAndUsageOutput {
	return &costexplorer.GetCostAndUsageOutput{ResultsByTime: []types.ResultByTime{{
		TimePeriod: &types.DateInterval{Start: aws.String(start), End: aws.String(end)},
		Total:      map[string]types.MetricValue{"UnblendedCost": {Amount: aws.String(amount), Unit: aws.String(currency)}},
	}}}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func validRequest() provider.CollectRequest {
	return provider.CollectRequest{
		AccountID:    "123456789012",
		StartTime:    time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC),
		EndTime:      time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC),
		CollectionID: "2026-09-29/2026-09-30",
	}
}

type fakeCostExplorerClient struct {
	inputs  []*costexplorer.GetCostAndUsageInput
	outputs []*costexplorer.GetCostAndUsageOutput
	errors  []error
}

func (f *fakeCostExplorerClient) GetCostAndUsage(_ context.Context, input *costexplorer.GetCostAndUsageInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	f.inputs = append(f.inputs, input)
	index := len(f.inputs) - 1
	if index < len(f.errors) && f.errors[index] != nil {
		return nil, f.errors[index]
	}
	if index >= len(f.outputs) {
		return nil, nil
	}
	return f.outputs[index], nil
}
