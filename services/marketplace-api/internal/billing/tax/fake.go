package tax

import (
	"context"
	"sync"
)

// FakeValidator is a test helper that implements Validator. CountryCode is
// returned by Country(); Result and Err are returned verbatim from Validate().
// LastReq captures the most recent request for assertion. Safe for concurrent
// use.
type FakeValidator struct {
	CountryCode string
	Result      ValidationResult
	Err         error

	mu      sync.Mutex
	lastReq ValidationRequest
}

// Country returns the bound ISO-3166 alpha-2 code.
func (f *FakeValidator) Country() string { return f.CountryCode }

// Validate captures the request and returns the configured Result/Err.
func (f *FakeValidator) Validate(_ context.Context, req ValidationRequest) (ValidationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastReq = req
	return f.Result, f.Err
}

// LastRequest returns a copy of the most recent ValidationRequest. Returns the
// zero value if Validate was never called.
func (f *FakeValidator) LastRequest() ValidationRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastReq
}
