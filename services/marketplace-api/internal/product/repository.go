// Package product — repository.go: Create / Get / List / UpdateBasics /
// SoftDelete and unique-violation translation helpers for the product
// aggregate.
//
// Methods that mutate variants, stock, media, or category links live in
// sibling files (repository_variants.go, repository_media.go) to keep
// each file under ~500 lines. The split is purely cosmetic — all methods
// satisfy the same `Repository` interface below.
//
// Design notes:
//   - Every method is tenant-scoped: WHERE tenant_id = ? (and store_id
//     where applicable). There is no `GetByID` that skips tenant_id.
//   - `CreateAggregateInTx` requires the caller to supply an open
//     transaction because the product graph spans six tables and partial
//     inserts would break invariants. The caller owns commit/rollback.
//   - Unique-violation translation is the repository's responsibility.
//     The caller above (service layer) can rewrite the "suggested"
//     handle/sku with a better candidate but does not have to guess at
//     the pg error code.
//   - Suggested-handle/SKU on a collision is conservatively "<value>-2";
//     the service layer's Create path calls `SuggestNextHandle` pre-tx
//     so the collision only happens on lost-write races. When it does,
//     "-2" is good enough for the wire response — a human clicking
//     "retry" will see it and pick something else. Deeper suggestion
//     logic belongs in the service, not here.
package product

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the persistence contract for the product aggregate.
// See models.go for the underlying gorm types.
type Repository interface {
	CreateAggregateInTx(ctx context.Context, tx *gorm.DB, a *Aggregate) error
	GetByIDForStore(ctx context.Context, id, storeID, tenantID string) (*Aggregate, error)
	ListAdmin(ctx context.Context, q ListAdminQuery) ([]Aggregate, int64, error)
	ListPublished(ctx context.Context, q ListPublishedQuery) ([]Aggregate, error)
	ListPublishedBySlugs(ctx context.Context, storeID string, slugs []string) ([]Aggregate, error)
	GetPublishedByHandle(ctx context.Context, storeID, handle string) (*Aggregate, error)
	UpdateBasicsInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string, fields map[string]any) error
	ApplyVariantDiffInTx(ctx context.Context, tx *gorm.DB, productID, storeID string, diff VariantDiff) error
	ReplaceCategoryLinksInTx(ctx context.Context, tx *gorm.DB, productID string, categoryIDs []string) error
	ReplaceMediaInTx(ctx context.Context, tx *gorm.DB, productID string, media []Media) error
	InsertMediaInTx(ctx context.Context, tx *gorm.DB, m *Media) error
	UpdateMediaInTx(ctx context.Context, tx *gorm.DB, productID, mediaID, storeID, tenantID string, fields map[string]any) error
	DeleteMediaInTx(ctx context.Context, tx *gorm.DB, productID, mediaID, storeID, tenantID string) error
	UpdateVariantStockInTx(ctx context.Context, tx *gorm.DB, variantID, locationID string, quantity int) error
	UpdateVariantBasicsInTx(ctx context.Context, tx *gorm.DB, productID, variantID, storeID, tenantID string, fields map[string]any) error
	SoftDeleteInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string) error
}

// Aggregate is the in-memory full product graph. Not a gorm model;
// composed from the per-table types in models.go. `Variants` are
// the flat list (with OptionValueLinks nested per variant). Prefer
// reading `Options[i].Values` over a separate VariantOption slice.
type Aggregate struct {
	Product       Product
	Options       []Option
	Variants      []Variant
	Media         []Media
	CategoryLinks []ProductCategory
}

// ListAdminQuery is the input shape for ListAdmin.
type ListAdminQuery struct {
	StoreID  string
	TenantID string
	Status   string // "", "draft", "active", "archived"
	Search   string // optional; full-text over title+description
	Page     int    // 1-based
	PageSize int
}

// ListPublishedQuery is the input shape for ListPublished.
type ListPublishedQuery struct {
	StoreID      string
	CategorySlug string // optional
	Search       string // optional case-insensitive substring match on title
	Page         int
	PageSize     int
}

// VariantDiff carries the add/update/remove set produced elsewhere
// (typically by matrix.DiffMatrix after matching up existing DB rows to
// incoming VariantSpecs). A separate type from matrix.MatrixDiff because
// this layer works in gorm `Variant` rows (real FKs, decimals, etc.) and
// the matrix layer works in `VariantSpec` (option-name tuples only).
type VariantDiff struct {
	Adds    []Variant
	Updates []Variant
	Removes []string // variant IDs
}

// gormRepository is the concrete implementation.
type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository backed by the given *gorm.DB.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

// ---------- Create ----------

// CreateAggregateInTx inserts products, product_options, product_option_values,
// product_variants, variant_option_values, product_media, and product_categories.
// The caller MUST supply an open transaction and own its commit/rollback.
//
// Ordering is strict:
//  1. products (parent row; required for FK)
//  2. product_options (each option must exist before its values)
//  3. product_option_values
//  4. product_variants (composite FK to products(id, store_id))
//  5. variant_option_values (links variants to option values)
//  6. product_media
//  7. product_categories
//
// IDs in the aggregate are either user-supplied (UUIDs) or generated by
// the DB DEFAULT. We assume the caller has pre-assigned UUIDs for any
// rows that need to be referenced by child rows in the same call (it is
// simpler than rebinding IDs after each INSERT).
func (r *gormRepository) CreateAggregateInTx(ctx context.Context, tx *gorm.DB, a *Aggregate) error {
	if a == nil {
		return fmt.Errorf("product: create aggregate: nil aggregate")
	}
	tx = tx.WithContext(ctx)

	// 1. Product row.
	if err := tx.Create(&a.Product).Error; err != nil {
		return translateUniqueViolation(err, a.Product.Handle, firstSKU(a.Variants))
	}

	// 2. Options (each with its nested Values).
	for i := range a.Options {
		a.Options[i].ProductID = a.Product.ID
		if err := tx.Create(&a.Options[i]).Error; err != nil {
			return fmt.Errorf("product: create option %q: %w", a.Options[i].Name, err)
		}
		// Nested values: Option already has Values populated; GORM's
		// Create cascades them because of the HasMany relationship on
		// the model. We do NOT need a second Create call.
	}

	// 3. Variants. Composite FK to products(id, store_id) enforces that
	//    store_id on variant matches store_id on product. We set store_id
	//    from the parent to prevent drift.
	for i := range a.Variants {
		a.Variants[i].ProductID = a.Product.ID
		a.Variants[i].StoreID = a.Product.StoreID
		// Detach OptionValueLinks so GORM's HasMany cascade does not
		// insert them during the parent Create — we insert them
		// explicitly below once VariantID is known.
		links := a.Variants[i].OptionValueLinks
		a.Variants[i].OptionValueLinks = nil
		if err := tx.Create(&a.Variants[i]).Error; err != nil {
			a.Variants[i].OptionValueLinks = links
			return translateUniqueViolation(err, a.Product.Handle, a.Variants[i].SKU)
		}
		for j := range links {
			links[j].VariantID = a.Variants[i].ID
		}
		if len(links) > 0 {
			if err := tx.Create(&links).Error; err != nil {
				return fmt.Errorf("product: link variant %q to option values: %w", a.Variants[i].SKU, err)
			}
		}
		a.Variants[i].OptionValueLinks = links
	}

	// 4. Media.
	for i := range a.Media {
		a.Media[i].ProductID = a.Product.ID
	}
	if len(a.Media) > 0 {
		if err := tx.Create(&a.Media).Error; err != nil {
			return fmt.Errorf("product: create media: %w", err)
		}
	}

	// 5. Category links.
	for i := range a.CategoryLinks {
		a.CategoryLinks[i].ProductID = a.Product.ID
	}
	if len(a.CategoryLinks) > 0 {
		if err := tx.Create(&a.CategoryLinks).Error; err != nil {
			return fmt.Errorf("product: create category links: %w", err)
		}
	}

	return nil
}

// ---------- Get ----------

// GetByIDForStore returns the full aggregate or apperrors.NotFound.
// Tenant isolation is enforced: cross-tenant lookups return NotFound.
func (r *gormRepository) GetByIDForStore(ctx context.Context, id, storeID, tenantID string) (*Aggregate, error) {
	var p Product
	err := r.db.WithContext(ctx).
		Preload("Options").
		Preload("Options.Values").
		Preload("Variants").
		Preload("Variants.OptionValueLinks").
		Preload("Media").
		Where("id = ? AND store_id = ? AND tenant_id = ? AND deleted_at IS NULL", id, storeID, tenantID).
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("product")
	}
	if err != nil {
		return nil, fmt.Errorf("product: get by id: %w", err)
	}

	var links []ProductCategory
	if err := r.db.WithContext(ctx).
		Where("product_id = ?", p.ID).
		Find(&links).Error; err != nil {
		return nil, fmt.Errorf("product: get category links: %w", err)
	}

	return &Aggregate{
		Product:       p,
		Options:       p.Options,
		Variants:      p.Variants,
		Media:         p.Media,
		CategoryLinks: links,
	}, nil
}

// ---------- ListAdmin ----------

// ListAdmin returns paginated products for the admin UI. It runs at most
// 7 SQL statements regardless of how many products are returned:
//  1. COUNT(*) for pagination total
//  2. SELECT products (page slice)
//  3. Preload options (one query, filtered by product_id IN (...))
//  4. Preload option values (one query, filtered by option_id IN (...))
//  5. Preload variants (one query)
//  6. Preload variant_option_values (one query)
//  7. Preload product_media (one query)
//
// Category links are deliberately NOT returned from ListAdmin — the admin
// list DTO doesn't need them. M5 handler composes them from a separate
// batch call if it ever needs them.
func (r *gormRepository) ListAdmin(ctx context.Context, q ListAdminQuery) ([]Aggregate, int64, error) {
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	if q.Page <= 0 {
		q.Page = 1
	}

	base := r.db.WithContext(ctx).
		Model(&Product{}).
		Where("store_id = ? AND tenant_id = ? AND deleted_at IS NULL", q.StoreID, q.TenantID)
	if q.Status != "" {
		base = base.Where("status = ?", q.Status)
	}
	if q.Search != "" {
		base = base.Where(
			"to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(description,'')) @@ plainto_tsquery('simple', ?)",
			q.Search)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("product: list admin count: %w", err)
	}

	var rows []Product
	err := base.
		Preload("Options").
		Preload("Options.Values").
		Preload("Variants").
		Preload("Variants.OptionValueLinks").
		Preload("Media").
		Order("updated_at DESC").
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("product: list admin: %w", err)
	}

	out := make([]Aggregate, 0, len(rows))
	for _, p := range rows {
		out = append(out, Aggregate{
			Product:  p,
			Options:  p.Options,
			Variants: p.Variants,
			Media:    p.Media,
		})
	}
	return out, total, nil
}

// ---------- ListPublished ----------

// ListPublished is the storefront-visible query. It enforces
// status = 'active' AND deleted_at IS NULL AND published_at IS NOT NULL.
// Even though M3 has no storefront handler, shipping this now means M6
// does not need to touch the repository.
//
// If `CategorySlug` is non-empty, the query joins through
// product_categories + categories filtering on slug. Otherwise the join
// is skipped.
func (r *gormRepository) ListPublished(ctx context.Context, q ListPublishedQuery) ([]Aggregate, error) {
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	if q.Page <= 0 {
		q.Page = 1
	}

	// SECURITY: hard-coded storefront visibility filter. No caller can
	// bypass any of these predicates — status, published_at, deleted_at.
	base := r.db.WithContext(ctx).
		Model(&Product{}).
		Where("store_id = ? AND deleted_at IS NULL AND status = ? AND published_at IS NOT NULL AND published_at <= now()",
			q.StoreID, StatusActive)

	if q.CategorySlug != "" {
		base = base.Where(`
			id IN (
				SELECT pc.product_id
				FROM product_categories pc
				JOIN categories c ON c.id = pc.category_id
				WHERE c.slug = ? AND c.store_id = ? AND c.deleted_at IS NULL AND c.is_active = true
			)`, q.CategorySlug, q.StoreID)
	}

	if q.Search != "" {
		// Case-insensitive substring match against the title. The
		// storefront `Search` field is intentionally narrower than the
		// admin's full-text search to avoid matching draft or
		// admin-only fields. Bound by the storefront query's max=200.
		base = base.Where("title ILIKE ?", "%"+q.Search+"%")
	}

	var rows []Product
	err := base.
		Preload("Options").
		Preload("Options.Values").
		Preload("Variants").
		Preload("Media").
		Order("published_at DESC").
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("product: list published: %w", err)
	}

	out := make([]Aggregate, 0, len(rows))
	for _, p := range rows {
		out = append(out, Aggregate{
			Product:  p,
			Options:  p.Options,
			Variants: p.Variants,
			Media:    p.Media,
		})
	}
	return out, nil
}

// ---------- ListPublishedBySlugs ----------

// ListPublishedBySlugs returns published products whose handle matches
// any of the supplied slugs, preserving the input order. Missing slugs
// silently drop (the storefront falls back to "fewer products" rather
// than 404ing the whole homepage). Applies the same hard-coded
// storefront visibility filter as ListPublished.
func (r *gormRepository) ListPublishedBySlugs(ctx context.Context, storeID string, slugs []string) ([]Aggregate, error) {
	if len(slugs) == 0 {
		return []Aggregate{}, nil
	}
	// Dedupe while preserving first-seen order.
	seen := make(map[string]struct{}, len(slugs))
	ordered := make([]string, 0, len(slugs))
	for _, s := range slugs {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		ordered = append(ordered, s)
	}
	if len(ordered) == 0 {
		return []Aggregate{}, nil
	}

	var rows []Product
	err := r.db.WithContext(ctx).
		Preload("Options").
		Preload("Options.Values").
		Preload("Variants").
		Preload("Media").
		Where("store_id = ? AND handle IN ? AND status = ? AND deleted_at IS NULL AND published_at IS NOT NULL AND published_at <= now()",
			storeID, ordered, StatusActive).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("product: list published by slugs: %w", err)
	}

	byHandle := make(map[string]Product, len(rows))
	for _, p := range rows {
		byHandle[p.Handle] = p
	}

	out := make([]Aggregate, 0, len(rows))
	for _, h := range ordered {
		p, ok := byHandle[h]
		if !ok {
			continue
		}
		out = append(out, Aggregate{
			Product:  p,
			Options:  p.Options,
			Variants: p.Variants,
			Media:    p.Media,
		})
	}
	return out, nil
}

// ---------- GetPublishedByHandle ----------

// GetPublishedByHandle is the storefront product-detail lookup. It
// applies the same hard-coded visibility filter as ListPublished —
// status='active', published_at IS NOT NULL AND <= now(),
// deleted_at IS NULL — and returns apperrors.NotFound("product") on
// any miss (existence leaks are explicitly forbidden by spec §13.1.4).
func (r *gormRepository) GetPublishedByHandle(ctx context.Context, storeID, handle string) (*Aggregate, error) {
	var p Product
	err := r.db.WithContext(ctx).
		Preload("Options").
		Preload("Options.Values").
		Preload("Variants").
		Preload("Variants.OptionValueLinks").
		Preload("Media").
		Where("store_id = ? AND handle = ? AND status = ? AND deleted_at IS NULL AND published_at IS NOT NULL AND published_at <= now()",
			storeID, handle, StatusActive).
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("product")
	}
	if err != nil {
		return nil, fmt.Errorf("product: get published by handle: %w", err)
	}

	var links []ProductCategory
	if err := r.db.WithContext(ctx).
		Where("product_id = ?", p.ID).
		Find(&links).Error; err != nil {
		return nil, fmt.Errorf("product: get published by handle category links: %w", err)
	}

	return &Aggregate{
		Product:       p,
		Options:       p.Options,
		Variants:      p.Variants,
		Media:         p.Media,
		CategoryLinks: links,
	}, nil
}

// ---------- UpdateBasics ----------

// UpdateBasicsInTx updates title/description/seo/status/tags/primary
// category. The field map is validated/filtered by the service layer;
// this method simply passes it through with WHERE tenant isolation.
//
// This is also the code path exercised by the §14.3 regression test: if
// a caller tries to clear `deleted_at` on a row while another live row
// with the same (store_id, handle) exists, the partial unique index
// fires during the UPDATE and this method translates that to
// apperrors.HandleTaken.
//
// NOTE: The standard update WHERE clause includes `deleted_at IS NULL`
// so you can't normally un-delete via this method. For the §14.3 test we
// bypass that filter with an explicit "allow deleted" mode controlled by
// the presence of a deleted_at key in the fields map. In production code
// the service layer never clears deleted_at; un-delete is not a
// supported user operation. The mode exists solely so tests can exercise
// the translation helper on the update path.
func (r *gormRepository) UpdateBasicsInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string, fields map[string]any) error {
	q := tx.WithContext(ctx).Model(&Product{}).
		Where("id = ? AND store_id = ? AND tenant_id = ?", id, storeID, tenantID)
	if _, touchesDeletedAt := fields["deleted_at"]; !touchesDeletedAt {
		q = q.Where("deleted_at IS NULL")
	}
	res := q.Updates(fields)
	if res.Error != nil {
		handle, _ := fields["handle"].(string)
		return translateUniqueViolation(res.Error, handle, "")
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("product")
	}
	return nil
}

// ---------- SoftDelete ----------

// SoftDeleteInTx sets deleted_at on the product row. Variants carry
// their own deleted_at for future slices; we do NOT cascade here.
func (r *gormRepository) SoftDeleteInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string) error {
	res := tx.WithContext(ctx).Model(&Product{}).
		Where("id = ? AND store_id = ? AND tenant_id = ? AND deleted_at IS NULL", id, storeID, tenantID).
		Update("deleted_at", gorm.Expr("now()"))
	if res.Error != nil {
		return fmt.Errorf("product: soft delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("product")
	}
	return nil
}

// ---------- Unique-violation translation ----------

// translateUniqueViolation inspects a pg error and returns the
// appropriate typed apperrors.Error. When the error is not a unique
// violation it is returned wrapped for debuggability.
//
// Callers pass whatever `attemptedHandle` and `attemptedSKU` they have
// at the call site; empty strings are acceptable. The suggested handle
// is conservatively `attempted + "-2"`; the service layer may rewrite
// with a better candidate.
func translateUniqueViolation(err error, attemptedHandle, attemptedSKU string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return fmt.Errorf("product: db error: %w", err)
	}
	switch pgErr.ConstraintName {
	case "products_handle_per_store_live_unique":
		return apperrors.HandleTaken(attemptedHandle, attemptedHandle+"-2")
	case "variants_sku_per_store_live_unique":
		return apperrors.SKUTaken(attemptedSKU)
	default:
		return fmt.Errorf("product: unique violation on %s: %w", pgErr.ConstraintName, err)
	}
}

// firstSKU returns the SKU of the first variant or "" — used only to
// give the translation helper a reasonable "attempted" value when a
// unique violation happens on the Product INSERT (unlikely, but the
// error could also chain from batch inserts).
func firstSKU(vs []Variant) string {
	if len(vs) == 0 {
		return ""
	}
	return vs[0].SKU
}
