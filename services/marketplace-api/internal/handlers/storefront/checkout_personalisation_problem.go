package storefront

// personalisation_problem.go — naming the line and field a rejection
// belongs to (#966).
//
// # Why a generic message was not good enough
//
// Every personalisation rejection used to collapse into
// apperrors.ValidationFailed("personalisation", msg): one flat string,
// no line, no field. For most rejections that is merely unhelpful. For
// an EXPIRED UPLOAD it is a trap.
//
// Uploads are swept at 72 hours. A cart line survives in localStorage
// indefinitely. So a buyer can come back a week later to a cart that
// looks perfectly intact, press Pay, and be told "that image is no
// longer available" with nothing indicating which of their three
// personalised lines is the problem or what to do about it. They have
// already decided to buy at that point.
//
// So a rejection now carries the line index, the field, and a machine
// code. The storefront puts the message ON the offending line, disables
// that line alone rather than the pay button, and offers a re-upload.
//
// # What is deliberately still vague
//
// The REASON an upload could not be loaded stays one message for every
// failure mode — wrong cart, wrong field, wrong store, not yet verified,
// does not exist. Distinguishing those would tell a prober which upload
// ids are real.
//
// Naming the FIELD leaks nothing: the buyer filled that form in, and the
// field belongs to the product they are looking at. The upload id is
// still never echoed.

import (
	"errors"
	"fmt"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Problem codes. Stable strings — the storefront branches on these, so
// they are API surface and renaming one is a breaking change.
const (
	// ProblemUploadUnavailable is the expired-or-unknown upload. The one
	// the buyer can fix, by uploading the image again.
	ProblemUploadUnavailable = "upload_unavailable"
	// ProblemRequired is a missing answer to a required field.
	ProblemRequired = "required"
	// ProblemInvalid is an answer that is present and wrong — too long,
	// not an offered option.
	ProblemInvalid = "invalid"
)

// personalisationProblem is a rejection that knows where it came from.
type personalisationProblem struct {
	// LineIndex is the index into the request's items array, so the
	// storefront can map it back to the cart line it rendered.
	LineIndex  int
	FieldKey   string
	FieldLabel string
	Code       string
	Reason     string
}

func (p *personalisationProblem) Error() string { return p.Reason }

// withLine stamps the line index on a problem raised deeper down, where
// only the field is known. Returns err untouched when it is not a
// problem, so an infrastructure error is not disguised as a validation
// failure the buyer could act on.
func withLine(i int, err error) error {
	var p *personalisationProblem
	if errors.As(err, &p) {
		p.LineIndex = i
		return p
	}
	return fmt.Errorf("cart line %d: %w", i, err)
}

// AsPersonalisationError converts a rejection into the wire error.
//
// The field name on the envelope stays the field KEY rather than the
// literal "personalisation", so a client that only reads details.field —
// every other validation error in this service — still gets something
// it can attach to an input.
func AsPersonalisationError(err error) *apperrors.Error {
	var p *personalisationProblem
	if !errors.As(err, &p) {
		return apperrors.ValidationFailed("personalisation", err.Error())
	}
	out := apperrors.ValidationFailed(p.FieldKey, p.Reason)
	out.Details["line_index"] = p.LineIndex
	out.Details["field_key"] = p.FieldKey
	out.Details["field_label"] = p.FieldLabel
	out.Details["problem"] = p.Code
	return out
}
