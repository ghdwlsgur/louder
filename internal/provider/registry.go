package provider

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidProviderRegistration = errors.New("invalid provider registration")
	ErrProviderAlreadyRegistered   = errors.New("provider already registered")
	ErrProviderNotRegistered       = errors.New("provider is not registered")
)

type Factory func() (Provider, error)

type Registry struct {
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

func (r *Registry) Register(name string, factory Factory) error {
	if name == "" || strings.TrimSpace(name) != name || strings.ToLower(name) != name || factory == nil {
		return fmt.Errorf("%w: lowercase provider name and factory are required", ErrInvalidProviderRegistration)
	}
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("%w: %s", ErrProviderAlreadyRegistered, name)
	}
	r.factories[name] = factory
	return nil
}

func (r *Registry) Resolve(name string) (Provider, error) {
	factory, exists := r.factories[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotRegistered, name)
	}
	instance, err := factory()
	if err != nil {
		return nil, fmt.Errorf("construct provider %s: %w", name, err)
	}
	if instance == nil {
		return nil, fmt.Errorf("construct provider %s: %w", name, ErrInvalidProviderRegistration)
	}
	return instance, nil
}
