package awsprovider

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscredentials "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/smithy-go"
	"github.com/ghdwlsgur/louder/internal/provider"
)

type CostExplorerClient interface {
	GetCostAndUsage(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
}

type Provider struct {
	client      CostExplorerClient
	credentials aws.CredentialsProvider
}

func New(client CostExplorerClient, credentials aws.CredentialsProvider) *Provider {
	return &Provider{client: client, credentials: credentials}
}

func NewFromConfig(config aws.Config) *Provider {
	return New(costexplorer.NewFromConfig(config), config.Credentials)
}

func (*Provider) Metadata(context.Context) provider.ProviderMetadata {
	return provider.ProviderMetadata{Name: "aws"}
}

func (p *Provider) ValidateCredentials(ctx context.Context) error {
	if ctx.Err() != nil {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	if p.credentials == nil {
		return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
	}
	credentials, err := p.credentials.Retrieve(ctx)
	if err != nil {
		var emptyCredentials *awscredentials.StaticCredentialsEmptyError
		if errors.As(err, &emptyCredentials) {
			return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
		}
		return classifyError(ctx, err)
	}
	if credentials.AccessKeyID == "" || credentials.SecretAccessKey == "" {
		return &provider.ProviderError{Class: provider.ErrorInvalidCredentialShape}
	}
	return nil
}

func (p *Provider) CollectCosts(ctx context.Context, request provider.CollectRequest) ([]provider.RawCostRecord, error) {
	if ctx.Err() != nil {
		return nil, &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	accountID, err := validateRequest(request)
	if err != nil {
		return nil, err
	}
	input := &costexplorer.GetCostAndUsageInput{
		Granularity: types.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod:  &types.DateInterval{Start: aws.String(accountID.start), End: aws.String(accountID.end)},
		Filter: &types.Expression{Dimensions: &types.DimensionValues{
			Key:    types.DimensionLinkedAccount,
			Values: []string{request.AccountID},
		}},
	}

	var results []types.ResultByTime
	seenTokens := make(map[string]struct{})
	for {
		output, requestErr := p.client.GetCostAndUsage(ctx, input)
		if requestErr != nil {
			return nil, classifyError(ctx, requestErr)
		}
		if output == nil {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		results = append(results, output.ResultsByTime...)
		if output.NextPageToken == nil || *output.NextPageToken == "" {
			break
		}
		if _, exists := seenTokens[*output.NextPageToken]; exists {
			return nil, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		seenTokens[*output.NextPageToken] = struct{}{}
		input.NextPageToken = output.NextPageToken
	}

	records := make([]provider.RawCostRecord, 0, len(results))
	for _, result := range results {
		record, mapErr := mapDailyResult(request.AccountID, result)
		if mapErr != nil {
			return nil, mapErr
		}
		records = append(records, record)
	}
	return records, nil
}

type dateRange struct {
	start string
	end   string
}

func validateRequest(request provider.CollectRequest) (dateRange, error) {
	if len(request.AccountID) != 12 {
		return dateRange{}, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
	}
	for _, digit := range request.AccountID {
		if digit < '0' || digit > '9' {
			return dateRange{}, &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
		}
	}
	start, validStart := utcDate(request.StartTime)
	end, validEnd := utcDate(request.EndTime)
	if !validStart || !validEnd || !request.StartTime.Before(request.EndTime) {
		return dateRange{}, fmt.Errorf("AWS Cost Explorer requires a non-empty UTC date range aligned to midnight")
	}
	return dateRange{start: start, end: end}, nil
}

func utcDate(value time.Time) (string, bool) {
	utc := value.UTC()
	if utc.Hour() != 0 || utc.Minute() != 0 || utc.Second() != 0 || utc.Nanosecond() != 0 {
		return "", false
	}
	return utc.Format("2006-01-02"), true
}

func mapDailyResult(accountID string, result types.ResultByTime) (provider.RawCostRecord, error) {
	if result.TimePeriod == nil || result.TimePeriod.Start == nil || result.TimePeriod.End == nil {
		return provider.RawCostRecord{}, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	start, err := time.Parse("2006-01-02", *result.TimePeriod.Start)
	if err != nil {
		return provider.RawCostRecord{}, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	end, err := time.Parse("2006-01-02", *result.TimePeriod.End)
	if err != nil || !end.Equal(start.AddDate(0, 0, 1)) {
		return provider.RawCostRecord{}, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	metric, exists := result.Total["UnblendedCost"]
	if !exists || metric.Amount == nil || metric.Unit == nil || *metric.Unit == "" {
		return provider.RawCostRecord{}, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	if _, ok := new(big.Rat).SetString(*metric.Amount); !ok {
		return provider.RawCostRecord{}, &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	return provider.RawCostRecord{
		Provider:       "aws",
		SourceRecordID: "aws-cost-explorer-" + accountID + "-" + *result.TimePeriod.Start,
		BillingScope:   accountID,
		CostBasis:      provider.CostBasisUnblended,
		Amount:         *metric.Amount,
		Currency:       *metric.Unit,
		UsageStart:     start,
		UsageEnd:       end,
	}, nil
}

func classifyError(ctx context.Context, err error) *provider.ProviderError {
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return &provider.ProviderError{Class: provider.ErrorTimeout}
	}
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		switch apiError.ErrorCode() {
		case "UnrecognizedClientException", "InvalidClientTokenId", "SignatureDoesNotMatch":
			return &provider.ProviderError{Class: provider.ErrorAuthenticationFailed}
		case "AccessDeniedException", "AccessDenied", "UnauthorizedOperation":
			return &provider.ProviderError{Class: provider.ErrorPermissionDenied}
		case "LimitExceededException", "ThrottlingException", "TooManyRequestsException":
			return &provider.ProviderError{Class: provider.ErrorRateLimited}
		case "DataUnavailableException", "ServiceUnavailableException", "InternalErrorException":
			return &provider.ProviderError{Class: provider.ErrorProviderUnavailable}
		case "BillExpirationException", "ValidationException":
			return &provider.ProviderError{Class: provider.ErrorUnsupportedBillingScope}
		case "InvalidNextTokenException", "RequestChangedException":
			return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
		}
		if apiError.ErrorFault() == smithy.FaultServer {
			return &provider.ProviderError{Class: provider.ErrorProviderUnavailable}
		}
		return &provider.ProviderError{Class: provider.ErrorInvalidResponse}
	}
	return &provider.ProviderError{Class: provider.ErrorProviderUnavailable}
}
