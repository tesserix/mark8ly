package validators

import (
	"context"
	"regexp"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

// TH RD tax ID: 13 digits (national ID format).
// §19.3 TH row: manual review until RD exposes a B2B verification API.
var thRDRegex = regexp.MustCompile(`^\d{13}$`)

// THValidator enqueues every well-formed Thai tax ID for manual review.
type THValidator struct{}

// NewTH constructs the manual-review-only Thai validator.
func NewTH() *THValidator { return &THValidator{} }

// Country returns ISO-3166 alpha-2.
func (v *THValidator) Country() string { return "TH" }

// Validate format-checks then queues for manual review.
func (v *THValidator) Validate(_ context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "TH" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	if !thRDRegex.MatchString(req.TaxID) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	return tax.ValidationResult{
		Valid:                false,
		ManualReviewRequired: true,
		QueueReason:          "rd_manual",
	}, nil
}
