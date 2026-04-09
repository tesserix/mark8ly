// Package product — repository_media.go: ReplaceMediaInTx and
// ReplaceCategoryLinksInTx. Pulled out of repository.go to keep files
// small and topical.
package product

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// InsertMediaInTx inserts a single product_media row. The caller must
// have already set ProductID on m. Returns an unwrapped error on any
// DB failure.
func (r *gormRepository) InsertMediaInTx(ctx context.Context, tx *gorm.DB, m *Media) error {
	tx = tx.WithContext(ctx)
	if err := tx.Create(m).Error; err != nil {
		return fmt.Errorf("product: insert media: %w", err)
	}
	return nil
}

// UpdateMediaInTx applies the given fields to a single product_media row,
// scoped so cross-tenant / cross-store / deleted-product writes are
// impossible. Returns apperrors.NotFound when no row matches.
func (r *gormRepository) UpdateMediaInTx(ctx context.Context, tx *gorm.DB, productID, mediaID, storeID, tenantID string, fields map[string]any) error {
	tx = tx.WithContext(ctx)
	res := tx.Model(&Media{}).
		Where(`id = ? AND product_id = ? AND EXISTS (
				SELECT 1 FROM products p
				WHERE p.id = product_media.product_id
				  AND p.store_id = ? AND p.tenant_id = ? AND p.deleted_at IS NULL)`,
			mediaID, productID, storeID, tenantID).
		Updates(fields)
	if res.Error != nil {
		return fmt.Errorf("product: update media: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("media")
	}
	return nil
}

// DeleteMediaInTx hard-deletes a product_media row under the same
// scoping as UpdateMediaInTx. Returns apperrors.NotFound when no row
// matches.
func (r *gormRepository) DeleteMediaInTx(ctx context.Context, tx *gorm.DB, productID, mediaID, storeID, tenantID string) error {
	tx = tx.WithContext(ctx)
	res := tx.
		Where(`id = ? AND product_id = ? AND EXISTS (
				SELECT 1 FROM products p
				WHERE p.id = product_media.product_id
				  AND p.store_id = ? AND p.tenant_id = ? AND p.deleted_at IS NULL)`,
			mediaID, productID, storeID, tenantID).
		Delete(&Media{})
	if res.Error != nil {
		return fmt.Errorf("product: delete media: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("media")
	}
	return nil
}

// ReplaceMediaInTx reconciles product_media for a product to match the
// target set. Existing rows matched by (product_id, storage_key) are
// preserved (so their id stays stable, which matters for FK references
// from cart/order snapshots). New keys are inserted. Stale keys are
// deleted. Position/Alt updates on preserved rows ARE applied.
//
// Simple DELETE+INSERT would be easier but would churn ids needlessly.
func (r *gormRepository) ReplaceMediaInTx(ctx context.Context, tx *gorm.DB, productID string, media []Media) error {
	tx = tx.WithContext(ctx)

	var existing []Media
	if err := tx.Where("product_id = ?", productID).Find(&existing).Error; err != nil {
		return fmt.Errorf("product: load existing media: %w", err)
	}

	existingByKey := make(map[string]Media, len(existing))
	for _, m := range existing {
		existingByKey[m.StorageKey] = m
	}
	incomingByKey := make(map[string]Media, len(media))
	for _, m := range media {
		incomingByKey[m.StorageKey] = m
	}

	// Delete stale rows.
	var staleIDs []string
	for key, m := range existingByKey {
		if _, ok := incomingByKey[key]; !ok {
			staleIDs = append(staleIDs, m.ID)
		}
	}
	if len(staleIDs) > 0 {
		if err := tx.Where("id IN ?", staleIDs).Delete(&Media{}).Error; err != nil {
			return fmt.Errorf("product: delete stale media: %w", err)
		}
	}

	// Update preserved rows.
	for key, in := range incomingByKey {
		if ex, ok := existingByKey[key]; ok {
			if err := tx.Model(&Media{}).
				Where("id = ?", ex.ID).
				Updates(map[string]any{
					"url":       in.URL,
					"alt":       in.Alt,
					"position":  in.Position,
					"media_type": in.MediaType,
					"width":     in.Width,
					"height":    in.Height,
					"bytes":     in.Bytes,
					"variant_id": in.VariantID,
				}).Error; err != nil {
				return fmt.Errorf("product: update preserved media: %w", err)
			}
		}
	}

	// Insert fresh rows.
	var fresh []Media
	for key, in := range incomingByKey {
		if _, ok := existingByKey[key]; ok {
			continue
		}
		in.ProductID = productID
		fresh = append(fresh, in)
	}
	if len(fresh) > 0 {
		if err := tx.Create(&fresh).Error; err != nil {
			return fmt.Errorf("product: insert fresh media: %w", err)
		}
	}
	return nil
}

// ReplaceCategoryLinksInTx deletes and reinserts product_categories.
// Simple enough that DELETE+INSERT is the right tool; link rows have no
// id that needs to stay stable.
func (r *gormRepository) ReplaceCategoryLinksInTx(ctx context.Context, tx *gorm.DB, productID string, categoryIDs []string) error {
	tx = tx.WithContext(ctx)

	if err := tx.Where("product_id = ?", productID).
		Delete(&ProductCategory{}).Error; err != nil {
		return fmt.Errorf("product: delete category links: %w", err)
	}
	if len(categoryIDs) == 0 {
		return nil
	}
	rows := make([]ProductCategory, 0, len(categoryIDs))
	for _, cid := range categoryIDs {
		rows = append(rows, ProductCategory{ProductID: productID, CategoryID: cid})
	}
	if err := tx.Create(&rows).Error; err != nil {
		return fmt.Errorf("product: insert category links: %w", err)
	}
	return nil
}
