package product

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// ExportRow is a minimal projection of a product for CSV export. It skips
// heavy relations (options, variants, media) that ListAdmin preloads and
// instead flattens the first variant's price/SKU/stock inline.
type ExportRow struct {
	ID          string
	Handle      string
	Title       string
	Description *string
	Status      string
	SKU         string
	Price       string
	Stock       int
}

// ForEachExportRow iterates over all non-deleted products in a store,
// calling fn for each row. Uses GORM's Rows() cursor so the full result
// set is never loaded into memory. Suitable for streaming CSV export.
func (r *gormRepository) ForEachExportRow(ctx context.Context, storeID, tenantID string, fn func(ExportRow) error) error {
	rows, err := r.db.WithContext(ctx).
		Raw(`SELECT p.id, p.handle, p.title, p.description, p.status,
		            COALESCE(v.sku, '') AS sku,
		            COALESCE(v.price::text, '0') AS price,
		            COALESCE(v.inventory_quantity, 0) AS stock
		     FROM products p
		     LEFT JOIN LATERAL (
		         SELECT sku, price, inventory_quantity
		         FROM product_variants
		         WHERE product_id = p.id AND deleted_at IS NULL
		         ORDER BY position ASC
		         LIMIT 1
		     ) v ON true
		     WHERE p.store_id = ? AND p.tenant_id = ? AND p.deleted_at IS NULL
		     ORDER BY p.updated_at DESC`,
			storeID, tenantID).
		Rows()
	if err != nil {
		return fmt.Errorf("product: export rows: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var row ExportRow
		if err := rows.Scan(&row.ID, &row.Handle, &row.Title, &row.Description,
			&row.Status, &row.SKU, &row.Price, &row.Stock); err != nil {
			return fmt.Errorf("product: scan export row: %w", err)
		}
		if err := fn(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ExportRepository is the interface for streaming CSV export. Extracted so
// the handler can depend on a narrow interface rather than the full Repository.
type ExportRepository interface {
	ForEachExportRow(ctx context.Context, storeID, tenantID string, fn func(ExportRow) error) error
}

// NewExportRepository constructs an ExportRepository backed by the given
// *gorm.DB. This is a separate constructor so callers that only need
// export functionality don't have to depend on the full Repository interface.
func NewExportRepository(db *gorm.DB) ExportRepository {
	return &gormRepository{db: db}
}

// Compile-time check.
var _ ExportRepository = (*gormRepository)(nil)
