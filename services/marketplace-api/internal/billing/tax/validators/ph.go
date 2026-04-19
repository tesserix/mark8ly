package validators

import (
	"context"
	"regexp"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

// PH BIR TIN: 9 or 12 digits, dashes optional. §19.3 PH row: manual review.
var phBIRRegex = regexp.MustCompile(`^\d{3}-?\d{3}-?\d{3}(-?\d{3})?$`)

// PHValidator enqueues every well-formed Philippine TIN for manual review.
type PHValidator struct{}

// NewPH constructs the manual-review-only Philippine validator.
func NewPH() *PHValidator { return &PHValidator{} }

// Country returns ISO-3166 alpha-2.
func (v *PHValidator) Country() string { return "PH" }

// Validate format-checks then queues for manual review.
func (v *PHValidator) Validate(_ context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "PH" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	if !phBIRRegex.MatchString(req.TaxID) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	return tax.ValidationResult{
		Valid:                false,
		ManualReviewRequired: true,
		QueueReason:          "bir_manual",
	}, nil
}
