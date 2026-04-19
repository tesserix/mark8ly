// Package tax implements per-country tax-ID validation, the 14-day window
// orchestrator, and the quarterly revalidation cron per spec §19.
//
// Every country-specific validator lives in the validators/ subpackage and
// implements Validator. The orchestrator (service.go) is validator-agnostic
// and holds all DB writes and clock-pause logic in one place.
package tax

import (
	"context"
	"errors"
)

// ValidationRequest is the normalized payload from the admin form. The
// orchestrator has already loaded the subscription and computed the country
// from tax_id_country. TaxID is a raw string; validators handle per-country
// format normalization.
type ValidationRequest struct {
	TenantID       string
	StoreID        string
	Country        string // ISO-3166 alpha-2, uppercase
	TaxID          string
	BusinessName   string
	BillingAddress string
}

// ValidationResult carries the validator's verdict back to the orchestrator.
//
// Valid=true means "registry confirmed this ID". RegistryName is used for the
// fuzzy name cross-check (§19.3). ManualReviewRequired=true signals the SEA
// path; the orchestrator queues it and pauses the clock immediately.
type ValidationResult struct {
	Valid                bool
	RegistryName         string
	RegistryCallID       string
	ManualReviewRequired bool
	QueueReason          string
}

// Validator is the contract every country-specific implementation satisfies.
// Implementations MUST be stateless except for an injected *http.Client and
// registry base URL; orchestration state is owned by tax.Service.
type Validator interface {
	Country() string
	Validate(ctx context.Context, req ValidationRequest) (ValidationResult, error)
}

// Error sentinels — every validator uses these, no custom per-country error
// types. Orchestrator and handlers switch on these.
var (
	ErrInvalidFormat        = errors.New("tax: invalid format for country")
	ErrRegistryUnavailable  = errors.New("tax: registry unavailable (outage tracked)")
	ErrNotFound             = errors.New("tax: id not found in registry")
	ErrManualReviewRequired = errors.New("tax: enters manual review queue")
	ErrValidatorDisabled    = errors.New("tax: validator disabled by feature flag")
	ErrNameMismatch         = errors.New("tax: name mismatch (advisory; orchestrator decides)")
)
