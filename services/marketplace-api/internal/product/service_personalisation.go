// Package product — service_personalisation.go: authoring the fields a
// buyer fills in on a product (#962).
//
// The validation here is the whole point of the file. Every field kind
// makes some of the constraint columns meaningful and the rest nonsense,
// and migration 000139 enforces that with CHECKs — so without validation
// a merchant typing a character limit onto an image field would get a
// constraint violation rendered as a 500. Everything is checked before
// any DB work, and the error names the field the merchant can see.
//
// The per-PLAN field limit is NOT enforced here. It lives in the handler,
// next to the subscription lookup, exactly as the images-per-product cap
// does (internal/handlers/admin/media.go). What this layer enforces is
// MaxPersonalisationFieldsPerProduct: the ceiling that applies even when
// the plan says Unlimited.
package product

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Bounds on the kind-specific columns. The defaults exist so a merchant
// who ignores the advanced inputs still gets a usable control rather than
// an unbounded one; the ceilings exist because every one of these values
// is spent on the checkout hot path or in a print pipeline.
const (
	defaultTextMaxLength     = 100
	maxTextMaxLength         = 200
	defaultTextareaMaxLength = 500
	maxTextareaMaxLength     = 2000
	defaultMaxImages         = 1
	maxMaxImages             = 10
	maxHelpTextLength        = 300
	maxFieldLabelLength      = 120
	maxFieldKeyLength        = 60
	maxOptionValueLength     = 200
)

// personalisationKeyRe matches the DB's own key format CHECK. Kept in
// sync by hand; the integration test asserts a key this rejects is also
// rejected by Postgres.
var personalisationKeyRe = regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)*$`)

// PersonalisationOptionSpec is one value of a select, as supplied by the
// merchant.
type PersonalisationOptionSpec struct {
	Value      string
	Label      string
	PriceDelta decimal.Decimal
	Position   int
}

// CreatePersonalisationFieldRequest is the service-level DTO.
//
// Options is required for kind=select and must be empty for every other
// kind: a select with no values is a control a buyer cannot answer, so it
// is created with its values or not at all.
type CreatePersonalisationFieldRequest struct {
	ProductID string
	StoreID   string
	TenantID  string

	Key      string
	Label    string
	Kind     string
	Required bool
	Position int
	HelpText *string

	MaxLength        *int
	MaxImages        *int
	MinPx            *int
	MockupStorageKey *string
	PrintArea        *PrintAreaRect
	PriceDelta       *decimal.Decimal

	Options []PersonalisationOptionSpec
}

// UpdatePersonalisationFieldRequest patches a field. Nil means "leave
// alone"; Kind is absent deliberately — changing an image field into a
// text field would orphan uploads and invalidate every order snapshot
// that referenced it. Delete and recreate instead.
type UpdatePersonalisationFieldRequest struct {
	FieldID  string
	StoreID  string
	TenantID string

	Label    *string
	Required *bool
	Position *int
	HelpText *string

	MaxLength        *int
	MaxImages        *int
	MinPx            *int
	MockupStorageKey *string
	PrintArea        *PrintAreaRect
	PriceDelta       *decimal.Decimal
}

// ListPersonalisationFields returns a product's fields in author order.
// Verifies the product resolves under the caller's tenant/store first, so
// an unknown product id is NotFound rather than an empty list — the two
// mean different things to the admin UI.
func (s *Service) ListPersonalisationFields(ctx context.Context, productID, storeID, tenantID string) ([]PersonalisationField, error) {
	if _, err := s.repo.GetByIDForStore(ctx, productID, storeID, tenantID); err != nil {
		return nil, err
	}
	return s.repo.ListPersonalisationFields(ctx, productID, storeID, tenantID)
}

// GetPersonalisationField loads one field with its options.
func (s *Service) GetPersonalisationField(ctx context.Context, fieldID, storeID, tenantID string) (*PersonalisationField, error) {
	return s.repo.GetPersonalisationField(ctx, fieldID, storeID, tenantID)
}

// CountPersonalisationFields is what the handler's plan gate reads before
// allowing a create.
func (s *Service) CountPersonalisationFields(ctx context.Context, productID, storeID, tenantID string) (int64, error) {
	if _, err := s.repo.GetByIDForStore(ctx, productID, storeID, tenantID); err != nil {
		return 0, err
	}
	return s.repo.CountPersonalisationFields(ctx, productID, storeID, tenantID)
}

// CreatePersonalisationField validates and inserts a field, together with
// its option values when it is a select.
func (s *Service) CreatePersonalisationField(ctx context.Context, req CreatePersonalisationFieldRequest) (*PersonalisationField, error) {
	if _, err := s.repo.GetByIDForStore(ctx, req.ProductID, req.StoreID, req.TenantID); err != nil {
		return nil, err
	}

	normalised, err := validateCreateField(&req)
	if err != nil {
		return nil, err
	}

	n, err := s.repo.CountPersonalisationFields(ctx, req.ProductID, req.StoreID, req.TenantID)
	if err != nil {
		return nil, err
	}
	if int(n)+1 > MaxPersonalisationFieldsPerProduct {
		return nil, apperrors.ValidationFailed("personalisation_fields",
			"a product may not have more than 20 personalisation fields")
	}

	field := normalised
	field.TenantID = req.TenantID
	field.StoreID = req.StoreID
	field.ProductID = req.ProductID

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.InsertPersonalisationFieldInTx(ctx, tx, field); err != nil {
			return err
		}
		for i := range req.Options {
			o := req.Options[i]
			row := &PersonalisationOption{
				FieldID:    field.ID,
				Value:      strings.TrimSpace(o.Value),
				Label:      strings.TrimSpace(o.Label),
				PriceDelta: o.PriceDelta,
				Position:   o.Position,
			}
			if err := s.repo.InsertPersonalisationOptionInTx(ctx, tx, row); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.repo.GetPersonalisationField(ctx, field.ID, req.StoreID, req.TenantID)
}

// UpdatePersonalisationField patches a field. The kind is read from the
// stored row, not the request, so the kind-specific rules are enforced
// against what the field actually is.
func (s *Service) UpdatePersonalisationField(ctx context.Context, req UpdatePersonalisationFieldRequest) (*PersonalisationField, error) {
	existing, err := s.repo.GetPersonalisationField(ctx, req.FieldID, req.StoreID, req.TenantID)
	if err != nil {
		return nil, err
	}

	fields, err := validateUpdateField(existing.Kind, req)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return existing, nil
	}
	fields["updated_at"] = gorm.Expr("now()")

	if err := s.repo.UpdatePersonalisationField(ctx, req.FieldID, req.StoreID, req.TenantID, fields); err != nil {
		return nil, err
	}
	return s.repo.GetPersonalisationField(ctx, req.FieldID, req.StoreID, req.TenantID)
}

// DeletePersonalisationField removes a field and cascades its options.
func (s *Service) DeletePersonalisationField(ctx context.Context, fieldID, storeID, tenantID string) error {
	return s.repo.DeletePersonalisationField(ctx, fieldID, storeID, tenantID)
}

// AddPersonalisationOption appends a value to a select field.
func (s *Service) AddPersonalisationOption(ctx context.Context, fieldID, storeID, tenantID string, spec PersonalisationOptionSpec) (*PersonalisationField, error) {
	field, err := s.repo.GetPersonalisationField(ctx, fieldID, storeID, tenantID)
	if err != nil {
		return nil, err
	}
	if field.Kind != PersonalisationKindSelect {
		return nil, apperrors.ValidationFailed("personalisation_option",
			"only a select field has option values")
	}
	if err := validateOptionSpec(spec, 0); err != nil {
		return nil, err
	}
	n, err := s.repo.CountPersonalisationOptions(ctx, fieldID)
	if err != nil {
		return nil, err
	}
	if int(n)+1 > MaxPersonalisationOptionsPerField {
		return nil, apperrors.ValidationFailed("personalisation_options",
			"a select field may not have more than 50 values")
	}

	row := &PersonalisationOption{
		FieldID:    fieldID,
		Value:      strings.TrimSpace(spec.Value),
		Label:      strings.TrimSpace(spec.Label),
		PriceDelta: spec.PriceDelta,
		Position:   spec.Position,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.repo.InsertPersonalisationOptionInTx(ctx, tx, row)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetPersonalisationField(ctx, fieldID, storeID, tenantID)
}

// UpdatePersonalisationOption patches one value of a select.
func (s *Service) UpdatePersonalisationOption(ctx context.Context, fieldID, optionID, storeID, tenantID string, spec PersonalisationOptionSpec, setDelta bool) (*PersonalisationField, error) {
	if _, err := s.repo.GetPersonalisationField(ctx, fieldID, storeID, tenantID); err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if v := strings.TrimSpace(spec.Value); v != "" {
		if len(v) > maxOptionValueLength {
			return nil, apperrors.ValidationFailed("personalisation_option.value",
				"value must be 200 characters or fewer")
		}
		fields["value"] = v
	}
	if l := strings.TrimSpace(spec.Label); l != "" {
		if len(l) > maxOptionValueLength {
			return nil, apperrors.ValidationFailed("personalisation_option.label",
				"label must be 200 characters or fewer")
		}
		fields["label"] = l
	}
	if setDelta {
		if spec.PriceDelta.IsNegative() {
			return nil, apperrors.ValidationFailed("personalisation_option.price_delta",
				"price_delta must not be negative")
		}
		fields["price_delta"] = spec.PriceDelta
	}
	if spec.Position != 0 {
		fields["position"] = spec.Position
	}
	if err := s.repo.UpdatePersonalisationOption(ctx, fieldID, optionID, storeID, tenantID, fields); err != nil {
		return nil, err
	}
	return s.repo.GetPersonalisationField(ctx, fieldID, storeID, tenantID)
}

// DeletePersonalisationOption removes one value of a select.
//
// Refuses to remove the last one: a select with no values is a control a
// buyer cannot answer, and a product that is already active would start
// rejecting every checkout of itself.
func (s *Service) DeletePersonalisationOption(ctx context.Context, fieldID, optionID, storeID, tenantID string) (*PersonalisationField, error) {
	if _, err := s.repo.GetPersonalisationField(ctx, fieldID, storeID, tenantID); err != nil {
		return nil, err
	}
	n, err := s.repo.CountPersonalisationOptions(ctx, fieldID)
	if err != nil {
		return nil, err
	}
	if n <= 1 {
		return nil, apperrors.ValidationFailed("personalisation_options",
			"a select field must keep at least one value; delete the field instead")
	}
	if err := s.repo.DeletePersonalisationOption(ctx, fieldID, optionID, storeID, tenantID); err != nil {
		return nil, err
	}
	return s.repo.GetPersonalisationField(ctx, fieldID, storeID, tenantID)
}

// ---------- validation ----------

// validateCreateField checks the request and returns the row to insert,
// with defaults applied and every column that does not belong to the kind
// left nil. The returned row has no id/tenant/store/product set.
func validateCreateField(req *CreatePersonalisationFieldRequest) (*PersonalisationField, error) {
	if !IsValidPersonalisationKind(req.Kind) {
		return nil, apperrors.ValidationFailed("personalisation_field.kind",
			"kind must be one of image, text, textarea, select, checkbox")
	}

	key := strings.TrimSpace(req.Key)
	if key == "" || len(key) > maxFieldKeyLength || !personalisationKeyRe.MatchString(key) {
		return nil, apperrors.ValidationFailed("personalisation_field.key",
			"key must be lowercase letters, digits and single underscores, 60 characters or fewer")
	}
	label := strings.TrimSpace(req.Label)
	if label == "" || len(label) > maxFieldLabelLength {
		return nil, apperrors.ValidationFailed("personalisation_field.label",
			"label is required and must be 120 characters or fewer")
	}
	if req.HelpText != nil && len(*req.HelpText) > maxHelpTextLength {
		return nil, apperrors.ValidationFailed("personalisation_field.help_text",
			"help text must be 300 characters or fewer")
	}

	row := &PersonalisationField{
		Key:      key,
		Label:    label,
		Kind:     req.Kind,
		Required: req.Required,
		Position: req.Position,
		HelpText: req.HelpText,
	}

	switch req.Kind {
	case PersonalisationKindImage:
		if err := rejectForKind(req, "image", req.MaxLength == nil, "max_length"); err != nil {
			return nil, err
		}
		if err := rejectForKind(req, "image", req.PriceDelta == nil, "price_delta"); err != nil {
			return nil, err
		}
		if len(req.Options) > 0 {
			return nil, apperrors.ValidationFailed("personalisation_field.options",
				"only a select field has option values")
		}
		maxImages := defaultMaxImages
		if req.MaxImages != nil {
			maxImages = *req.MaxImages
		}
		if maxImages < 1 || maxImages > maxMaxImages {
			return nil, apperrors.ValidationFailed("personalisation_field.max_images",
				"max_images must be between 1 and 10")
		}
		row.MaxImages = &maxImages
		if req.MinPx != nil {
			if *req.MinPx <= 0 {
				return nil, apperrors.ValidationFailed("personalisation_field.min_px",
					"min_px must be greater than zero")
			}
			row.MinPx = req.MinPx
		}
		if req.PrintArea != nil && (req.MockupStorageKey == nil || strings.TrimSpace(*req.MockupStorageKey) == "") {
			return nil, apperrors.ValidationFailed("personalisation_field.print_area",
				"a print area needs a mockup image to sit inside")
		}
		if req.MockupStorageKey != nil && strings.TrimSpace(*req.MockupStorageKey) != "" {
			row.MockupStorageKey = req.MockupStorageKey
		}
		if req.PrintArea != nil {
			encoded, err := encodePrintArea(*req.PrintArea)
			if err != nil {
				return nil, err
			}
			row.PrintArea = encoded
		}

	case PersonalisationKindText, PersonalisationKindTextarea:
		if err := rejectImageOnly(req); err != nil {
			return nil, err
		}
		if err := rejectForKind(req, req.Kind, req.PriceDelta == nil, "price_delta"); err != nil {
			return nil, err
		}
		if len(req.Options) > 0 {
			return nil, apperrors.ValidationFailed("personalisation_field.options",
				"only a select field has option values")
		}
		def, ceiling := defaultTextMaxLength, maxTextMaxLength
		if req.Kind == PersonalisationKindTextarea {
			def, ceiling = defaultTextareaMaxLength, maxTextareaMaxLength
		}
		length := def
		if req.MaxLength != nil {
			length = *req.MaxLength
		}
		if length < 1 || length > ceiling {
			return nil, apperrors.ValidationFailed("personalisation_field.max_length",
				"max_length is outside the range allowed for this kind")
		}
		row.MaxLength = &length

	case PersonalisationKindSelect:
		if err := rejectImageOnly(req); err != nil {
			return nil, err
		}
		if err := rejectForKind(req, "select", req.MaxLength == nil, "max_length"); err != nil {
			return nil, err
		}
		if err := rejectForKind(req, "select", req.PriceDelta == nil, "price_delta"); err != nil {
			return nil, err
		}
		if len(req.Options) == 0 {
			return nil, apperrors.ValidationFailed("personalisation_field.options",
				"a select field must be created with at least one value")
		}
		if len(req.Options) > MaxPersonalisationOptionsPerField {
			return nil, apperrors.ValidationFailed("personalisation_field.options",
				"a select field may not have more than 50 values")
		}
		seen := make(map[string]struct{}, len(req.Options))
		for i, o := range req.Options {
			if err := validateOptionSpec(o, i); err != nil {
				return nil, err
			}
			v := strings.TrimSpace(o.Value)
			if _, dup := seen[v]; dup {
				return nil, apperrors.ValidationFailed("personalisation_field.options",
					"option values must be unique within the field")
			}
			seen[v] = struct{}{}
		}

	case PersonalisationKindCheckbox:
		if err := rejectImageOnly(req); err != nil {
			return nil, err
		}
		if err := rejectForKind(req, "checkbox", req.MaxLength == nil, "max_length"); err != nil {
			return nil, err
		}
		if len(req.Options) > 0 {
			return nil, apperrors.ValidationFailed("personalisation_field.options",
				"only a select field has option values")
		}
		delta := decimal.Zero
		if req.PriceDelta != nil {
			delta = *req.PriceDelta
		}
		if delta.IsNegative() {
			return nil, apperrors.ValidationFailed("personalisation_field.price_delta",
				"price_delta must not be negative")
		}
		row.PriceDelta = &delta
	}

	return row, nil
}

// validateUpdateField builds the column map for a patch, rejecting any
// column that does not belong to the stored kind.
func validateUpdateField(kind string, req UpdatePersonalisationFieldRequest) (map[string]any, error) {
	fields := map[string]any{}

	if req.Label != nil {
		label := strings.TrimSpace(*req.Label)
		if label == "" || len(label) > maxFieldLabelLength {
			return nil, apperrors.ValidationFailed("personalisation_field.label",
				"label is required and must be 120 characters or fewer")
		}
		fields["label"] = label
	}
	if req.Required != nil {
		fields["required"] = *req.Required
	}
	if req.Position != nil {
		fields["position"] = *req.Position
	}
	if req.HelpText != nil {
		if len(*req.HelpText) > maxHelpTextLength {
			return nil, apperrors.ValidationFailed("personalisation_field.help_text",
				"help text must be 300 characters or fewer")
		}
		fields["help_text"] = *req.HelpText
	}

	isImage := kind == PersonalisationKindImage
	isText := kind == PersonalisationKindText || kind == PersonalisationKindTextarea

	if req.MaxLength != nil {
		if !isText {
			return nil, kindMismatch("max_length", kind)
		}
		ceiling := maxTextMaxLength
		if kind == PersonalisationKindTextarea {
			ceiling = maxTextareaMaxLength
		}
		if *req.MaxLength < 1 || *req.MaxLength > ceiling {
			return nil, apperrors.ValidationFailed("personalisation_field.max_length",
				"max_length is outside the range allowed for this kind")
		}
		fields["max_length"] = *req.MaxLength
	}
	if req.MaxImages != nil {
		if !isImage {
			return nil, kindMismatch("max_images", kind)
		}
		if *req.MaxImages < 1 || *req.MaxImages > maxMaxImages {
			return nil, apperrors.ValidationFailed("personalisation_field.max_images",
				"max_images must be between 1 and 10")
		}
		fields["max_images"] = *req.MaxImages
	}
	if req.MinPx != nil {
		if !isImage {
			return nil, kindMismatch("min_px", kind)
		}
		if *req.MinPx <= 0 {
			return nil, apperrors.ValidationFailed("personalisation_field.min_px",
				"min_px must be greater than zero")
		}
		fields["min_px"] = *req.MinPx
	}
	if req.MockupStorageKey != nil {
		if !isImage {
			return nil, kindMismatch("mockup_storage_key", kind)
		}
		fields["mockup_storage_key"] = strings.TrimSpace(*req.MockupStorageKey)
	}
	if req.PrintArea != nil {
		if !isImage {
			return nil, kindMismatch("print_area", kind)
		}
		encoded, err := encodePrintArea(*req.PrintArea)
		if err != nil {
			return nil, err
		}
		fields["print_area"] = encoded
	}
	if req.PriceDelta != nil {
		if kind != PersonalisationKindCheckbox {
			return nil, kindMismatch("price_delta", kind)
		}
		if req.PriceDelta.IsNegative() {
			return nil, apperrors.ValidationFailed("personalisation_field.price_delta",
				"price_delta must not be negative")
		}
		fields["price_delta"] = *req.PriceDelta
	}

	return fields, nil
}

func validateOptionSpec(o PersonalisationOptionSpec, _ int) error {
	v := strings.TrimSpace(o.Value)
	if v == "" || len(v) > maxOptionValueLength {
		return apperrors.ValidationFailed("personalisation_option.value",
			"value is required and must be 200 characters or fewer")
	}
	l := strings.TrimSpace(o.Label)
	if l == "" || len(l) > maxOptionValueLength {
		return apperrors.ValidationFailed("personalisation_option.label",
			"label is required and must be 200 characters or fewer")
	}
	if o.PriceDelta.IsNegative() {
		return apperrors.ValidationFailed("personalisation_option.price_delta",
			"price_delta must not be negative")
	}
	return nil
}

// rejectImageOnly refuses the four image-only columns on a non-image kind.
func rejectImageOnly(req *CreatePersonalisationFieldRequest) error {
	if req.MaxImages != nil {
		return kindMismatch("max_images", req.Kind)
	}
	if req.MinPx != nil {
		return kindMismatch("min_px", req.Kind)
	}
	if req.MockupStorageKey != nil {
		return kindMismatch("mockup_storage_key", req.Kind)
	}
	if req.PrintArea != nil {
		return kindMismatch("print_area", req.Kind)
	}
	return nil
}

// rejectForKind is the single-column form: ok must be true for the
// request to be valid on this kind.
func rejectForKind(_ *CreatePersonalisationFieldRequest, kind string, ok bool, column string) error {
	if ok {
		return nil
	}
	return kindMismatch(column, kind)
}

func kindMismatch(column, kind string) *apperrors.Error {
	return apperrors.ValidationFailed("personalisation_field."+column,
		column+" does not apply to a field of kind "+kind)
}

// encodePrintArea validates the rectangle and marshals it for the jsonb
// column. Percentages, so the mockup can be re-exported at a different
// size without invalidating the field.
func encodePrintArea(r PrintAreaRect) (*datatypes.JSON, error) {
	if r.W <= 0 || r.H <= 0 {
		return nil, apperrors.ValidationFailed("personalisation_field.print_area",
			"print area width and height must be greater than zero")
	}
	if r.X < 0 || r.Y < 0 || r.X+r.W > 100 || r.Y+r.H > 100 {
		return nil, apperrors.ValidationFailed("personalisation_field.print_area",
			"print area must lie within the mockup: x, y, w, h are percentages and must not exceed 100")
	}
	b, err := json.Marshal(r)
	if err != nil {
		return nil, apperrors.ValidationFailed("personalisation_field.print_area",
			"print area could not be encoded")
	}
	out := datatypes.JSON(b)
	return &out, nil
}
