package provider

import (
	"errors"
	"fmt"
)

type ErrorClass string

const (
	ErrorAuthenticationFailed    ErrorClass = "AuthenticationFailed"
	ErrorPermissionDenied        ErrorClass = "PermissionDenied"
	ErrorRateLimited             ErrorClass = "RateLimited"
	ErrorTimeout                 ErrorClass = "Timeout"
	ErrorProviderUnavailable     ErrorClass = "ProviderUnavailable"
	ErrorInvalidResponse         ErrorClass = "InvalidResponse"
	ErrorInvalidCredentialShape  ErrorClass = "InvalidCredentialShape"
	ErrorUnsupportedBillingScope ErrorClass = "UnsupportedBillingScope"
)

type ProviderError struct {
	Class ErrorClass
	Err   error
}

func (e *ProviderError) Error() string {
	if e.Err == nil {
		return string(e.Class)
	}
	return fmt.Sprintf("%s: %v", e.Class, e.Err)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

func IsErrorClass(err error, class ErrorClass) bool {
	var providerErr *ProviderError
	return errors.As(err, &providerErr) && providerErr.Class == class
}
