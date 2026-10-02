// Package product — repository_personalisation.go: reads and writes for
// product_personalisation_fields and product_personalisation_options.
//
// Every statement is scoped by store_id AND tenant_id, and additionally
// requires the owning product to be live. The field row carries both
// columns itself (unlike product_media, which has to reach them through
// its product), so the scope is cheap — but the products EXISTS clause
// stays, because a soft-deleted product's fields must not be editable
// through an id someone kept.
package product

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// isUniqueViolation reports whether err is a Postgres unique violation.
//
// translateUniqueViolation in repository.go maps the handle and SKU
// constraints to specific errors; these two tables need only "was this a
// duplicate", and their constraint names are already unambiguous from the
// call site, so the narrow check stays local.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// liveProductScope is the EXISTS clause shared by every field statement.
const liveProductScope = `EXISTS (
	SELECT 1 FROM products p
	WHERE p.id = product_personalisation_fields.product_id
	  AND p.store_id = product_personalisation_fields.store_id
	  AND p.deleted_at IS NULL)`

// fieldScopeViaParent scopes an options statement through its field.
const fieldScopeViaParent = `EXISTS (
	SELECT 1 FROM product_personalisation_fields f
	WHERE f.id = product_personalisation_options.field_id
	  AND f.store_id = ? AND f.tenant_id = ?
	  AND EXISTS (SELECT 1 FROM products p
	              WHERE p.id = f.product_id AND p.store_id = f.store_id
	                AND p.deleted_at IS NULL))`

// ListPersonalisationFields returns every field on a product, in author
// order, with each select's options preloaded in their own order.
//
// Returns an empty slice, not an error, for a product with no fields —
// which is every product until a merchant adds one.
func (r *gormRepository) ListPersonalisationFields(ctx context.Context, productID, storeID, tenantID string) ([]PersonalisationField, error) {
	var out []PersonalisationField
	err := r.db.WithContext(ctx).
		Preload("Options", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC, value ASC")
		}).
		Where("product_id = ? AND store_id = ? AND tenant_id = ?", productID, storeID, tenantID).
		Where(liveProductScope).
		Order("position ASC, key ASC").
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("product: list personalisation fields: %w", err)
	}
	return out, nil
}

// GetPersonalisationField loads one field by id with its options.
// Returns apperrors.NotFound for a cross-tenant, cross-store or
// deleted-product id — never a different error, so the id cannot be used
// to probe for existence in another merchant's catalog.
func (r *gormRepository) GetPersonalisationField(ctx context.Context, fieldID, storeID, tenantID string) (*PersonalisationField, error) {
	var f PersonalisationField
	err := r.db.WithContext(ctx).
		Preload("Options", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC, value ASC")
		}).
		Where("id = ? AND store_id = ? AND tenant_id = ?", fieldID, storeID, tenantID).
		Where(liveProductScope).
		First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("personalisation_field")
	}
	if err != nil {
		return nil, fmt.Errorf("product: get personalisation field: %w", err)
	}
	return &f, nil
}

// CountPersonalisationFields counts the fields already on a product. The
// service calls this before an insert to enforce both the per-plan limit
// and MaxPersonalisationFieldsPerProduct.
func (r *gormRepository) CountPersonalisationFields(ctx context.Context, productID, storeID, tenantID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&PersonalisationField{}).
		Where("product_id = ? AND store_id = ? AND tenant_id = ?", productID, storeID, tenantID).
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("product: count personalisation fields: %w", err)
	}
	return n, nil
}

// InsertPersonalisationFieldInTx inserts one field. The caller must have set
// TenantID, StoreID and ProductID, and must already have verified the
// product belongs to that scope — the composite FK will reject a
// store_id that does not match the product's, but it cannot tell us the
// product was never ours to begin with.
//
// A duplicate key on the same product surfaces as apperrors.Conflict
// rather than a raw constraint error, because a merchant typing a key
// twice is a validation outcome, not a server fault.
func (r *gormRepository) InsertPersonalisationFieldInTx(ctx context.Context, tx *gorm.DB, f *PersonalisationField) error {
	err := tx.WithContext(ctx).Create(f).Error
	if err == nil {
		return nil
	}
	if isUniqueViolation(err) {
		return apperrors.ValidationFailed("personalisation_field.key",
			"a field with this key already exists on the product")
	}
	return fmt.Errorf("product: insert personalisation field: %w", err)
}

// UpdatePersonalisationField applies fields to one row under full scope.
// Returns apperrors.NotFound when nothing matched.
func (r *gormRepository) UpdatePersonalisationField(ctx context.Context, fieldID, storeID, tenantID string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	res := r.db.WithContext(ctx).Model(&PersonalisationField{}).
		Where("id = ? AND store_id = ? AND tenant_id = ?", fieldID, storeID, tenantID).
		Where(liveProductScope).
		Updates(fields)
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return apperrors.ValidationFailed("personalisation_field.key",
				"a field with this key already exists on the product")
		}
		return fmt.Errorf("product: update personalisation field: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("personalisation_field")
	}
	return nil
}

// DeletePersonalisationField hard-deletes a field. Its options cascade.
//
// Hard delete, not soft: an order that referenced this field snapshotted
// its key and label onto the order line, so nothing historical depends
// on the row surviving.
func (r *gormRepository) DeletePersonalisationField(ctx context.Context, fieldID, storeID, tenantID string) error {
	res := r.db.WithContext(ctx).
		Where("id = ? AND store_id = ? AND tenant_id = ?", fieldID, storeID, tenantID).
		Where(liveProductScope).
		Delete(&PersonalisationField{})
	if res.Error != nil {
		return fmt.Errorf("product: delete personalisation field: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("personalisation_field")
	}
	return nil
}

// CountPersonalisationOptions counts the values on a select field.
func (r *gormRepository) CountPersonalisationOptions(ctx context.Context, fieldID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&PersonalisationOption{}).
		Where("field_id = ?", fieldID).Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("product: count personalisation options: %w", err)
	}
	return n, nil
}

// InsertPersonalisationOptionInTx inserts one option value. The caller
// must have loaded the parent field through GetPersonalisationField
// first, so scope is already proven; FieldID must be set on o.
//
// Takes a tx because a select field and its first values are created
// together — a select with no values is a control a buyer cannot answer,
// so it must never exist, not even between two statements.
func (r *gormRepository) InsertPersonalisationOptionInTx(ctx context.Context, tx *gorm.DB, o *PersonalisationOption) error {
	err := tx.WithContext(ctx).Create(o).Error
	if err == nil {
		return nil
	}
	if isUniqueViolation(err) {
		return apperrors.ValidationFailed("personalisation_option.value",
			"a value with this name already exists on the field")
	}
	return fmt.Errorf("product: insert personalisation option: %w", err)
}

// UpdatePersonalisationOption applies fields to one option, scoped
// through its field so a cross-store option id cannot be written.
func (r *gormRepository) UpdatePersonalisationOption(ctx context.Context, fieldID, optionID, storeID, tenantID string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	res := r.db.WithContext(ctx).Model(&PersonalisationOption{}).
		Where("id = ? AND field_id = ?", optionID, fieldID).
		Where(fieldScopeViaParent, storeID, tenantID).
		Updates(fields)
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return apperrors.ValidationFailed("personalisation_option.value",
				"a value with this name already exists on the field")
		}
		return fmt.Errorf("product: update personalisation option: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("personalisation_option")
	}
	return nil
}

// DeletePersonalisationOption hard-deletes one option value.
func (r *gormRepository) DeletePersonalisationOption(ctx context.Context, fieldID, optionID, storeID, tenantID string) error {
	res := r.db.WithContext(ctx).
		Where("id = ? AND field_id = ?", optionID, fieldID).
		Where(fieldScopeViaParent, storeID, tenantID).
		Delete(&PersonalisationOption{})
	if res.Error != nil {
		return fmt.Errorf("product: delete personalisation option: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("personalisation_option")
	}
	return nil
}
