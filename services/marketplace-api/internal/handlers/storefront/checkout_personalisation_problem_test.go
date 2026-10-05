package storefront

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// #966: a personalisation rejection has to say WHICH line and WHICH
// field, because the one that matters is an expired upload.
//
// Uploads are swept at 72h. A cart line lives in localStorage forever. So
// a buyer returns after four days to a cart that renders perfectly,
// presses Pay, and gets a flat "that image is no longer available" with
// three personalised lines on screen and no indication which is broken —
// at the exact moment they had decided to buy.
//
// What must NOT regress: the REASON stays one message for every lookup
// failure (wrong cart, wrong field, wrong store, unverified, absent), and
// the upload id is never echoed. Naming the field leaks nothing — the
// buyer filled that form in.

func TestAsPersonalisationError_CarriesLineFieldAndCode(t *testing.T) {
	err := withLine(2, &personalisationProblem{
		FieldKey:   "photo",
		FieldLabel: "Your photo",
		Code:       ProblemUploadUnavailable,
		Reason:     `The image for "Your photo" is no longer available — please upload it again.`,
	})

	ae := AsPersonalisationError(err)
	require.Equal(t, apperrors.CodeValidationFailed, ae.Code)
	require.Equal(t, 2, ae.Details["line_index"],
		"without the line index the storefront cannot put the message on the right cart line")
	require.Equal(t, "photo", ae.Details["field_key"])
	require.Equal(t, "Your photo", ae.Details["field_label"])
	require.Equal(t, ProblemUploadUnavailable, ae.Details["problem"])

	// details.field stays populated with the key, so a client that only
	// reads the conventional field — every other validation error in this
	// service — still has something to attach to an input.
	require.Equal(t, "photo", ae.Details["field"])

	require.Contains(t, ae.Message, "Your photo")
	require.Contains(t, ae.Message, "upload it again",
		"the buyer needs to be told the action, not just the fault")
}

func TestAsPersonalisationError_NeverEchoesAnUploadID(t *testing.T) {
	// The id is the thing a prober would want confirmed.
	const uploadID = "11111111-2222-3333-4444-555555555555"
	p := &personalisationProblem{
		FieldKey: "photo", FieldLabel: "Your photo",
		Code:   ProblemUploadUnavailable,
		Reason: `The image for "Your photo" is no longer available — please upload it again.`,
	}
	ae := AsPersonalisationError(withLine(0, p))

	require.NotContains(t, ae.Message, uploadID)
	for k, v := range ae.Details {
		require.NotContains(t, fmt.Sprint(v), uploadID, "details[%s] leaked the upload id", k)
	}
}

func TestAsPersonalisationError_PassesThroughAnUntypedError(t *testing.T) {
	// An infrastructure failure must not be dressed up as something the
	// buyer did wrong with a field they can fix.
	ae := AsPersonalisationError(errors.New("storefront: load personalisation fields: conn reset"))
	require.Equal(t, "personalisation", ae.Details["field"])
	require.Nil(t, ae.Details["problem"],
		"an unrecognised failure must not claim a machine-readable problem code")
	require.Nil(t, ae.Details["line_index"])
}

func TestWithLine_StampsIndexOnAProblemAndWrapsAnythingElse(t *testing.T) {
	p := &personalisationProblem{FieldKey: "name", Code: ProblemRequired, Reason: "needed"}
	out := withLine(7, p)

	var got *personalisationProblem
	require.True(t, errors.As(out, &got))
	require.Equal(t, 7, got.LineIndex)

	// A plain error keeps its identity and gains only context, so
	// errors.Is still works for callers upstream.
	sentinel := errors.New("boom")
	wrapped := withLine(1, sentinel)
	require.ErrorIs(t, wrapped, sentinel)
	require.Contains(t, wrapped.Error(), "cart line 1")

	var none *personalisationProblem
	require.False(t, errors.As(wrapped, &none),
		"a plain error must not become a buyer-facing field problem")
}

func TestProblemCodes_AreTheStringsTheStorefrontBranchesOn(t *testing.T) {
	// These are API surface: the storefront switches on them to decide
	// whether to offer a re-upload. Renaming one is a breaking change, so
	// pin the literals rather than comparing a constant to itself.
	require.Equal(t, "upload_unavailable", ProblemUploadUnavailable)
	require.Equal(t, "required", ProblemRequired)
	require.Equal(t, "invalid", ProblemInvalid)
}
