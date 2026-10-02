// This file is in the INTERNAL package (unlike product's other tests,
// which are external `product_test` packages) because it asserts against
// validateCreateField and validateUpdateField, which are unexported by
// design — the rules are an implementation detail everywhere except here,
// and routing every case through Service would mean a database for
// assertions that touch none.
//
// The point of these tests: migration 000139 enforces the kind-specific
// column rules with CHECK constraints, so anything this validation lets
// through becomes a constraint violation rendered to a merchant as a 500.
// Every case below is a 422 that would otherwise have been a 500.
package product

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// requireFieldError asserts err is a validation failure naming `field`.
//
// It reads Details["field"] rather than matching on the message, because
// that is what the API renders into details{} and what the admin form
// binds its inline errors to. The message is prose and may be reworded;
// the field name is a contract.
//
// An earlier draft of this file matched on the message instead, and six
// cases failed for a reason worth keeping in mind: the prose says "print
// area" and "option values" while the fields are "print_area" and
// "options", so a substring match both missed real errors and would have
// passed on unrelated ones.
func requireFieldError(t *testing.T, err error, field string) {
	t.Helper()
	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	require.Equal(t, apperrors.CodeValidationFailed, ae.Code)
	require.Equal(t, field, ae.Details["field"],
		"error names the wrong field: %s", ae.Message)
}

func intp(v int) *int       { return &v }
func strp(v string) *string { return &v }
func decp(v string) *decimal.Decimal {
	d := decimal.RequireFromString(v)
	return &d
}

func baseReq(kind string) CreatePersonalisationFieldRequest {
	return CreatePersonalisationFieldRequest{
		Key:   "photo",
		Label: "Your photo",
		Kind:  kind,
	}
}

func TestValidateCreateField_RejectsUnknownKind(t *testing.T) {
	req := baseReq("hologram")
	_, err := validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.kind")
}

func TestValidateCreateField_KeyFormat(t *testing.T) {
	// The DB has the same CHECK (personalisation_fields_key_format). If
	// these diverge, a key this accepts fails at insert instead of here.
	bad := []string{
		"", "Photo", "photo-name", "photo__name", "_photo", "photo_",
		"photo name", "photo.name", strings.Repeat("a", 61),
	}
	for _, k := range bad {
		req := baseReq(PersonalisationKindText)
		req.Key = k
		_, err := validateCreateField(&req)
		require.Errorf(t, err, "key %q should be rejected", k)
		requireFieldError(t, err, "personalisation_field.key")
	}

	for _, k := range []string{"photo", "photo_name", "a1", "name_2_line"} {
		req := baseReq(PersonalisationKindText)
		req.Key = k
		_, err := validateCreateField(&req)
		require.NoErrorf(t, err, "key %q should be accepted", k)
	}
}

func TestValidateCreateField_LabelAndHelpText(t *testing.T) {
	req := baseReq(PersonalisationKindText)
	req.Label = "   "
	_, err := validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.label")

	req = baseReq(PersonalisationKindText)
	req.Label = strings.Repeat("x", 121)
	_, err = validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.label")

	req = baseReq(PersonalisationKindText)
	req.HelpText = strp(strings.Repeat("x", 301))
	_, err = validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.help_text")
}

func TestValidateCreateField_ImageDefaultsAndBounds(t *testing.T) {
	req := baseReq(PersonalisationKindImage)
	row, err := validateCreateField(&req)
	require.NoError(t, err)
	require.NotNil(t, row.MaxImages)
	require.Equal(t, defaultMaxImages, *row.MaxImages,
		"an image field with no max_images must still carry one — the DB allows NULL, the buyer UI does not")
	require.Nil(t, row.MaxLength)
	require.Nil(t, row.PriceDelta)

	for _, n := range []int{0, -1, 11} {
		req := baseReq(PersonalisationKindImage)
		req.MaxImages = intp(n)
		_, err := validateCreateField(&req)
		requireFieldError(t, err, "personalisation_field.max_images")
	}

	req = baseReq(PersonalisationKindImage)
	req.MinPx = intp(0)
	_, err = validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.min_px")
}

func TestValidateCreateField_ImageRejectsColumnsOfOtherKinds(t *testing.T) {
	req := baseReq(PersonalisationKindImage)
	req.MaxLength = intp(50)
	_, err := validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.max_length")

	req = baseReq(PersonalisationKindImage)
	req.PriceDelta = decp("5.00")
	_, err = validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.price_delta")

	req = baseReq(PersonalisationKindImage)
	req.Options = []PersonalisationOptionSpec{{Value: "a", Label: "A"}}
	_, err = validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.options")
}

func TestValidateCreateField_PrintAreaNeedsAMockup(t *testing.T) {
	req := baseReq(PersonalisationKindImage)
	req.PrintArea = &PrintAreaRect{X: 10, Y: 10, W: 50, H: 50}
	_, err := validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.print_area")
}

func TestValidateCreateField_PrintAreaBounds(t *testing.T) {
	bad := []PrintAreaRect{
		{X: 0, Y: 0, W: 0, H: 10},   // zero width
		{X: 0, Y: 0, W: 10, H: 0},   // zero height
		{X: -1, Y: 0, W: 10, H: 10}, // negative origin
		{X: 60, Y: 0, W: 50, H: 10}, // runs off the right edge
		{X: 0, Y: 60, W: 10, H: 50}, // runs off the bottom
		{X: 0, Y: 0, W: 101, H: 10}, // wider than the mockup
	}
	for _, r := range bad {
		req := baseReq(PersonalisationKindImage)
		req.MockupStorageKey = strp("tenants/t/mockups/abc/shirt.jpg")
		rect := r
		req.PrintArea = &rect
		_, err := validateCreateField(&req)
		require.Errorf(t, err, "print area %+v should be rejected", r)
		requireFieldError(t, err, "personalisation_field.print_area")
	}
}

func TestValidateCreateField_PrintAreaEncodesAsPercentages(t *testing.T) {
	req := baseReq(PersonalisationKindImage)
	req.MockupStorageKey = strp("tenants/t/mockups/abc/shirt.jpg")
	req.PrintArea = &PrintAreaRect{X: 25, Y: 30, W: 50, H: 40}

	row, err := validateCreateField(&req)
	require.NoError(t, err)
	require.NotNil(t, row.PrintArea)

	var got PrintAreaRect
	require.NoError(t, json.Unmarshal(*row.PrintArea, &got))
	require.Equal(t, PrintAreaRect{X: 25, Y: 30, W: 50, H: 40}, got)
}

func TestValidateCreateField_TextLengthDefaultsPerKind(t *testing.T) {
	req := baseReq(PersonalisationKindText)
	row, err := validateCreateField(&req)
	require.NoError(t, err)
	require.Equal(t, defaultTextMaxLength, *row.MaxLength)

	req = baseReq(PersonalisationKindTextarea)
	row, err = validateCreateField(&req)
	require.NoError(t, err)
	require.Equal(t, defaultTextareaMaxLength, *row.MaxLength,
		"a gift message needs more room than a name, so the default differs by kind")
}

func TestValidateCreateField_TextLengthCeilingDiffersByKind(t *testing.T) {
	req := baseReq(PersonalisationKindText)
	req.MaxLength = intp(maxTextMaxLength + 1)
	_, err := validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.max_length")

	// The same number is fine on a textarea.
	req = baseReq(PersonalisationKindTextarea)
	req.MaxLength = intp(maxTextMaxLength + 1)
	_, err = validateCreateField(&req)
	require.NoError(t, err)

	req = baseReq(PersonalisationKindTextarea)
	req.MaxLength = intp(maxTextareaMaxLength + 1)
	_, err = validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.max_length")

	req = baseReq(PersonalisationKindText)
	req.MaxLength = intp(0)
	_, err = validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.max_length")
}

func TestValidateCreateField_NonImageKindsRejectImageColumns(t *testing.T) {
	for _, kind := range []string{
		PersonalisationKindText, PersonalisationKindTextarea,
		PersonalisationKindSelect, PersonalisationKindCheckbox,
	} {
		req := baseReq(kind)
		req.MaxImages = intp(2)
		_, err := validateCreateField(&req)
		requireFieldError(t, err, "personalisation_field.max_images")

		req = baseReq(kind)
		req.MinPx = intp(800)
		_, err = validateCreateField(&req)
		requireFieldError(t, err, "personalisation_field.min_px")

		req = baseReq(kind)
		req.MockupStorageKey = strp("k")
		_, err = validateCreateField(&req)
		requireFieldError(t, err, "personalisation_field.mockup_storage_key")

		req = baseReq(kind)
		req.PrintArea = &PrintAreaRect{X: 0, Y: 0, W: 10, H: 10}
		_, err = validateCreateField(&req)
		requireFieldError(t, err, "personalisation_field.print_area")
	}
}

func TestValidateCreateField_SelectMustArriveWithItsValues(t *testing.T) {
	req := baseReq(PersonalisationKindSelect)
	_, err := validateCreateField(&req)
	// A select with no values is a control a buyer cannot answer, so it
	// must never exist — not even between two statements.
	requireFieldError(t, err, "personalisation_field.options")
}

func TestValidateCreateField_SelectRejectsDuplicateValues(t *testing.T) {
	req := baseReq(PersonalisationKindSelect)
	req.Options = []PersonalisationOptionSpec{
		{Value: "gold", Label: "Gold"},
		{Value: " gold ", Label: "Gold again"},
	}
	_, err := validateCreateField(&req)
	// Compared trimmed, because the DB unique index sees the trimmed
	// value too: " gold " and "gold" are one value, not two.
	requireFieldError(t, err, "personalisation_field.options")
}

func TestValidateCreateField_SelectRejectsNegativeDelta(t *testing.T) {
	req := baseReq(PersonalisationKindSelect)
	req.Options = []PersonalisationOptionSpec{
		{Value: "engraved", Label: "Engraved", PriceDelta: decimal.RequireFromString("-1.00")},
	}
	_, err := validateCreateField(&req)
	require.ErrorContains(t, err, "price_delta",
		"a negative delta is a discount nobody authorised")
}

func TestValidateCreateField_SelectCapsItsValues(t *testing.T) {
	req := baseReq(PersonalisationKindSelect)
	for i := 0; i <= MaxPersonalisationOptionsPerField; i++ {
		req.Options = append(req.Options, PersonalisationOptionSpec{
			Value: "v" + strings.Repeat("x", i), Label: "V",
		})
	}
	_, err := validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.options")
}

func TestValidateCreateField_CheckboxDefaultsToFree(t *testing.T) {
	req := baseReq(PersonalisationKindCheckbox)
	row, err := validateCreateField(&req)
	require.NoError(t, err)
	require.NotNil(t, row.PriceDelta)
	require.True(t, row.PriceDelta.IsZero(),
		"an add-on with no stated price is free, not NULL — the column is the only record of what was charged")

	req = baseReq(PersonalisationKindCheckbox)
	req.PriceDelta = decp("-0.01")
	_, err = validateCreateField(&req)
	requireFieldError(t, err, "personalisation_field.price_delta")
}

func TestValidateUpdateField_RejectsColumnsOfOtherKinds(t *testing.T) {
	_, err := validateUpdateField(PersonalisationKindImage, UpdatePersonalisationFieldRequest{
		MaxLength: intp(50),
	})
	requireFieldError(t, err, "personalisation_field.max_length")

	_, err = validateUpdateField(PersonalisationKindText, UpdatePersonalisationFieldRequest{
		MaxImages: intp(2),
	})
	require.ErrorContains(t, err, "max_images")

	_, err = validateUpdateField(PersonalisationKindSelect, UpdatePersonalisationFieldRequest{
		PriceDelta: decp("1.00"),
	})
	require.ErrorContains(t, err, "price_delta",
		"a select's money lives on its options, never on the field")
}

func TestValidateUpdateField_EmptyPatchTouchesNothing(t *testing.T) {
	fields, err := validateUpdateField(PersonalisationKindText, UpdatePersonalisationFieldRequest{})
	require.NoError(t, err)
	require.Empty(t, fields, "an empty patch must not write updated_at either")
}

func TestValidateUpdateField_RequiredFalseIsNotAbsent(t *testing.T) {
	// The classic pointer bug: making a required field optional sends
	// false, which a zero-check would discard.
	no := false
	fields, err := validateUpdateField(PersonalisationKindText, UpdatePersonalisationFieldRequest{
		Required: &no,
	})
	require.NoError(t, err)
	require.Equal(t, false, fields["required"])
}

func TestValidateUpdateField_PositionZeroIsNotAbsent(t *testing.T) {
	zero := 0
	fields, err := validateUpdateField(PersonalisationKindText, UpdatePersonalisationFieldRequest{
		Position: &zero,
	})
	require.NoError(t, err)
	require.Equal(t, 0, fields["position"],
		"moving a field to the top sends 0; a zero-check would silently ignore it")
}

func TestIsValidPersonalisationKind(t *testing.T) {
	for _, k := range []string{"image", "text", "textarea", "select", "checkbox"} {
		require.True(t, IsValidPersonalisationKind(k), k)
	}
	for _, k := range []string{"", "IMAGE", "file", "number", "date"} {
		require.False(t, IsValidPersonalisationKind(k), k)
	}
}
