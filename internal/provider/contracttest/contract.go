package contracttest

import (
	"context"
	"slices"
	"testing"

	"github.com/ghdwlsgur/louder/internal/provider"
)

type Suite struct {
	Credentials []CredentialScenario
	Collections []CollectionScenario
}

type CredentialScenario struct {
	Name           string
	Provider       provider.Provider
	Context        context.Context
	WantErrorClass provider.ErrorClass
}

type CollectionScenario struct {
	Name           string
	Provider       provider.Provider
	Context        context.Context
	Request        provider.CollectRequest
	WantRecords    []provider.RawCostRecord
	WantErrorClass provider.ErrorClass
}

func Run(t *testing.T, suite Suite) {
	t.Helper()
	for _, scenario := range suite.Credentials {
		t.Run("credentials/"+scenario.Name, func(t *testing.T) {
			if scenario.Provider == nil {
				t.Fatal("Provider is nil")
			}
			ctx := scenario.Context
			if ctx == nil {
				ctx = context.Background()
			}
			assertErrorClass(t, scenario.Provider.ValidateCredentials(ctx), scenario.WantErrorClass)
		})
	}
	for _, scenario := range suite.Collections {
		t.Run("collection/"+scenario.Name, func(t *testing.T) {
			if scenario.Provider == nil {
				t.Fatal("Provider is nil")
			}
			ctx := scenario.Context
			if ctx == nil {
				ctx = context.Background()
			}
			records, err := scenario.Provider.CollectCosts(ctx, scenario.Request)
			assertErrorClass(t, err, scenario.WantErrorClass)
			if err != nil {
				return
			}
			if !slices.Equal(records, scenario.WantRecords) {
				t.Errorf("CollectCosts() records = %#v, want %#v", records, scenario.WantRecords)
			}
		})
	}
}

func assertErrorClass(t *testing.T, err error, want provider.ErrorClass) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("error = %v, want nil", err)
		}
		return
	}
	if !provider.IsErrorClass(err, want) {
		t.Fatalf("error = %v, want provider error class %q", err, want)
	}
}
