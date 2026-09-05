// Package validators implements per-country tax-ID validators that satisfy
// tax.Validator. Each file is a single-country implementation.
package validators

import (
	"context"
	"regexp"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

// US: the IRS does not expose a public EIN lookup API. Per spec §19.3 US row,
// validation is format-only plus a legally-binding attestation checkbox
// (§19.3.1). The checkbox is recorded elsewhere (business_entity_attestations
// table) and is managed by the admin handler — this validator only confirms
// the EIN shape.
//
// EIN format: NN-NNNNNNN or NNNNNNNNN (9 digits, optional dash after first 2).
var einRegex = regexp.MustCompile(`^\d{2}-?\d{7}$`)

// USValidator is a format-only EIN validator backed by the attestation flow.
type USValidator struct{}

// NewUS constructs the format-only US validator.
func NewUS() *USValidator { return &USValidator{} }

// Country returns ISO-3166 alpha-2.
func (v *USValidator) Country() string { return "US" }

// Validate checks EIN shape only. There is no US registry lookup, so NO
// registry name is returned and the orchestrator records the name match as
// "not_checked" (#707).
//
// It previously echoed req.BusinessName back, which made CompareNames compare
// a string with itself and write "matched" — recording that a registry had
// confirmed the name when none was consulted. Passing format validation is
// still correct; asserting a match that never happened was not.
//
// The attestation checkbox remains the real integrity gate for US merchants.
func (v *USValidator) Validate(_ context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "US" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	if !einRegex.MatchString(req.TaxID) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	return tax.ValidationResult{
		Valid: true,
		// Deliberately empty: no registry was consulted. CompareNames maps an
		// empty registry name to NameNotChecked (#707).
		RegistryName: "",
	}, nil
}
