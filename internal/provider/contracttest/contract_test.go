package contracttest

import (
	"context"
	"testing"

	"github.com/ghdwlsgur/louder/internal/provider"
)

func TestRunProviderContract(t *testing.T) {
	request := provider.CollectRequest{AccountID: "synthetic-account", CollectionID: "collection-1"}
	wantRecords := []provider.RawCostRecord{{
		Provider:       "aws",
		SourceRecordID: "record-1",
		BillingScope:   request.AccountID,
		Amount:         "12.50",
		Currency:       "USD",
	}}
	Run(t, Suite{
		Credentials: []CredentialScenario{
			{Name: "valid credentials", Provider: contractTestProvider{}},
			{Name: "invalid credentials", Provider: contractTestProvider{validateErr: &provider.ProviderError{Class: provider.ErrorAuthenticationFailed}}, WantErrorClass: provider.ErrorAuthenticationFailed},
			{Name: "deadline", Provider: contractTestProvider{contextError: true}, Context: canceledContext(), WantErrorClass: provider.ErrorTimeout},
		},
		Collections: []CollectionScenario{
			{Name: "complete logical result", Provider: contractTestProvider{records: wantRecords}, Request: request, WantRecords: wantRecords},
			{Name: "empty result", Provider: contractTestProvider{}, Request: request, WantRecords: []provider.RawCostRecord{}},
			{Name: "classified rate limit", Provider: contractTestProvider{collectErr: &provider.ProviderError{Class: provider.ErrorRateLimited}}, Request: request, WantErrorClass: provider.ErrorRateLimited},
			{Name: "deadline", Provider: contractTestProvider{contextError: true}, Context: canceledContext(), Request: request, WantErrorClass: provider.ErrorTimeout},
		},
	})
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

type contractTestProvider struct {
	validateErr  error
	records      []provider.RawCostRecord
	collectErr   error
	contextError bool
}

func (p contractTestProvider) ValidateCredentials(ctx context.Context) error {
	if p.contextError && ctx.Err() != nil {
		return &provider.ProviderError{Class: provider.ErrorTimeout, Err: ctx.Err()}
	}
	return p.validateErr
}
func (p contractTestProvider) CollectCosts(ctx context.Context, _ provider.CollectRequest) ([]provider.RawCostRecord, error) {
	if p.contextError && ctx.Err() != nil {
		return nil, &provider.ProviderError{Class: provider.ErrorTimeout, Err: ctx.Err()}
	}
	return p.records, p.collectErr
}
func (contractTestProvider) Metadata(context.Context) provider.ProviderMetadata {
	return provider.ProviderMetadata{Name: "aws"}
}
