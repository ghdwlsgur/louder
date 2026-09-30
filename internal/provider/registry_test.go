package provider

import (
	"context"
	"errors"
	"testing"
)

func TestRegistryConstructsProviderByName(t *testing.T) {
	want := &registryTestProvider{}
	registry := NewRegistry()
	if err := registry.Register("aws", func() (Provider, error) { return want, nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := registry.Resolve("aws")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != want {
		t.Fatalf("Resolve() provider = %T %p, want registered instance %p", got, got, want)
	}
	if err := got.ValidateCredentials(context.Background()); err != nil {
		t.Fatalf("resolved provider ValidateCredentials() error = %v", err)
	}
}

func TestRegistryRejectsInvalidDuplicateAndUnknownProviders(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("", func() (Provider, error) { return &registryTestProvider{}, nil }); !errors.Is(err, ErrInvalidProviderRegistration) {
		t.Fatalf("Register() with blank name error = %v, want ErrInvalidProviderRegistration", err)
	}
	if err := registry.Register("AWS", func() (Provider, error) { return &registryTestProvider{}, nil }); !errors.Is(err, ErrInvalidProviderRegistration) {
		t.Fatalf("Register() with noncanonical name error = %v, want ErrInvalidProviderRegistration", err)
	}
	if err := registry.Register("aws", nil); !errors.Is(err, ErrInvalidProviderRegistration) {
		t.Fatalf("Register() with nil factory error = %v, want ErrInvalidProviderRegistration", err)
	}
	if err := registry.Register("aws", func() (Provider, error) { return &registryTestProvider{}, nil }); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("aws", func() (Provider, error) { return &registryTestProvider{}, nil }); !errors.Is(err, ErrProviderAlreadyRegistered) {
		t.Fatalf("duplicate Register() error = %v, want ErrProviderAlreadyRegistered", err)
	}
	if _, err := registry.Resolve("gcp"); !errors.Is(err, ErrProviderNotRegistered) {
		t.Fatalf("Resolve() unknown provider error = %v, want ErrProviderNotRegistered", err)
	}
}

func TestRegistryRejectsNilProviderFromFactory(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("aws", func() (Provider, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("aws"); !errors.Is(err, ErrInvalidProviderRegistration) {
		t.Fatalf("Resolve() error = %v, want ErrInvalidProviderRegistration", err)
	}
}

type registryTestProvider struct{}

func (*registryTestProvider) ValidateCredentials(context.Context) error { return nil }
func (*registryTestProvider) CollectCosts(context.Context, CollectRequest) ([]RawCostRecord, error) {
	return nil, nil
}
func (*registryTestProvider) Metadata(context.Context) ProviderMetadata {
	return ProviderMetadata{Name: "aws"}
}
