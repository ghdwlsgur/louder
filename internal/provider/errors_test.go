package provider

import (
	"errors"
	"testing"
)

func TestProviderErrorExposesStableClassAndWrappedCause(t *testing.T) {
	cause := errors.New("upstream request failed")
	err := &ProviderError{Class: ErrorRateLimited, Err: cause}

	if !IsErrorClass(err, ErrorRateLimited) {
		t.Fatalf("IsErrorClass() = false, want true for %q", ErrorRateLimited)
	}
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(%v, cause) = false, want true", err)
	}
}
