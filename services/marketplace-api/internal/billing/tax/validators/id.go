package validators

import (
	"context"
	"regexp"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

// ID DJP NPWP: 15 digits, accepted in plain or canonical dashed form
// NN.NNN.NNN.N-NNN.NNN. §19.3 ID row: manual review.
//
// Type name is IDValidator (not ID*) to avoid colliding with builtin/unexported
// `id` patterns elsewhere in the codebase.
var (
	idNPWPPlain  = regexp.MustCompile(`^\d{15}$`)
	idNPWPDashed = regexp.MustCompile(`^\d{2}\.\d{3}\.\d{3}\.\d-\d{3}\.\d{3}$`)
)

// IDValidator enqueues every well-formed Indonesian NPWP for manual review.
type IDValidator struct{}

// NewID constructs the manual-review-only Indonesian validator.
func NewID() *IDValidator { return &IDValidator{} }

// Country returns ISO-3166 alpha-2.
func (v *IDValidator) Country() string { return "ID" }

// Validate format-checks (either plain or dashed NPWP) then queues for review.
func (v *IDValidator) Validate(_ context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "ID" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	if !idNPWPPlain.MatchString(req.TaxID) && !idNPWPDashed.MatchString(req.TaxID) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	return tax.ValidationResult{
		Valid:                false,
		ManualReviewRequired: true,
		QueueReason:          "djp_manual",
	}, nil
}
