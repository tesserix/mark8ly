// Package product — service.go: orchestration layer for the product
// aggregate. Every mutation runs inside a single gorm transaction and
// enqueues a matching outbox_events row in the same tx so M3's outbox
// publisher (and M6's Pub/Sub fan-out) sees domain events atomically
// with the mutation that produced them.
//
// Payload envelopes always include a top-level "store_id" key — the
// publisher's watermark upsert depends on it.
//
// Design notes:
//   - Rich-text Description is sanitized at write time via Sanitize().
//     Stored HTML is never re-sanitized on read (spec §14.14).
//   - Options/Variants shape is validated by matrix.ValidateMatrix before
//     any DB writes — failures never reach the repository layer.
//   - Every variant is forced to the store's currency. Cross-currency
//     variants return apperrors.CurrencyMismatch; currency change on
//     update returns apperrors.CurrencyChangeForbidden (slice 1).
//   - Media uploads are verified against the media.Uploader interface
//     pre-transaction so we fail fast without rolling back writes.
//   - Handle generation uses GenerateHandle + SuggestNextHandle; a lost-
//     write race still lands on the repo's "<handle>-2" fallback via
//     translateUniqueViolation.
package product

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// DefaultLocationID is the slice-1 single-warehouse location used for
// variant_stock seeding. Multi-warehouse lands in a later slice.
const DefaultLocationID = "00000000-0000-0000-0000-000000000001"

// Service orchestrates product mutations and their outbox events.
type Service struct {
	db         *gorm.DB
	repo       Repository
	storesRepo stores.Repository
	outboxRepo outbox.Repository
	uploader   media.Uploader
	logger     *slog.Logger
}

// Config bundles Service dependencies.
type Config struct {
	DB         *gorm.DB
	Repo       Repository
	StoresRepo stores.Repository
	OutboxRepo outbox.Repository
	Uploader   media.Uploader
	Logger     *slog.Logger
}

// NewService constructs a Service from cfg.
func NewService(cfg Config) *Service {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		db:         cfg.DB,
		repo:       cfg.Repo,
		storesRepo: cfg.StoresRepo,
		outboxRepo: cfg.OutboxRepo,
		uploader:   cfg.Uploader,
		logger:     logger,
	}
}

// ---------- DTOs ----------

// CreateRequest is the service-level shape of a new product. Handles
// are generated from Title when Handle is empty. Variant matrix rules
// are asserted by matrix.ValidateMatrix before any DB writes.
type CreateRequest struct {
	StoreID           string
	TenantID          string
	Handle            string // optional
	Title             string
	Description       string // rich text; sanitized on write
	Status            string // draft | active | archived (default draft)
	Tags              []string
	SEOTitle          *string
	SEODescription    *string
	PrimaryCategoryID *string
	VendorID          *string
	CategoryIDs       []string
	Options           []OptionSpec
	Variants          []VariantInput
	Media             []MediaInput
	CreatedBy         *string
}

// VariantInput is the service-layer shape of a variant on Create/Update.
// Matrix validation matches VariantSpec; money lives here.
type VariantInput struct {
	ID                string // empty = new
	SKU               string
	Barcode           *string
	Price             decimal.Decimal
	CompareAtPrice    *decimal.Decimal
	CostPrice         *decimal.Decimal
	CurrencyCode      string // must equal store currency
	WeightGrams       *int
	InitialStock      int
	InventoryPolicy   string
	LowStockThreshold *int
	Position          int
	OptionValues      []OptionValueRef
}

// MediaInput is the service-layer shape of a product media item. The
// StorageKey must correspond to a verified uploaded object.
type MediaInput struct {
	StorageKey string
	URL        string
	Alt        *string
	Position   int
	MediaType  string
	VariantID  *string
	Width      *int
	Height     *int
	Bytes      *int64
}

// UpdateBasicsRequest carries partial updates to title/description/seo
// and similar scalar fields. Variants/media/categories are handled by
// separate methods so each path can be reasoned about on its own.
type UpdateBasicsRequest struct {
	ID                string
	StoreID           string
	TenantID          string
	Title             *string
	Handle            *string
	Description       *string
	Status            *string
	Tags              *[]string
	SEOTitle          *string
	SEODescription    *string
	PrimaryCategoryID *string
	UpdatedBy         *string
}

// UpdateVariantsRequest carries a full desired variant matrix. The
// service computes the diff against existing variants and dispatches
// Adds/Updates/Removes through the repository.
type UpdateVariantsRequest struct {
	ID       string
	StoreID  string
	TenantID string
	Options  []OptionSpec
	Variants []VariantInput
}

// ---------- Create ----------

// Create inserts a new product aggregate + enqueues product.created.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Aggregate, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, apperrors.ValidationFailed("title", "title must not be empty")
	}

	// Resolve store + currency.
	store, err := s.storesRepo.GetByIDForTenant(ctx, req.StoreID, req.TenantID)
	if err != nil {
		return nil, apperrors.NotFound("store")
	}

	// Validate matrix shape before any DB writes.
	specs := variantSpecsFromInputs(req.Variants)
	if err := ValidateMatrix(req.Options, specs); err != nil {
		return nil, err
	}

	// Enforce currency on every variant.
	for i, v := range req.Variants {
		if v.CurrencyCode == "" {
			req.Variants[i].CurrencyCode = store.CurrencyCode
			continue
		}
		if v.CurrencyCode != store.CurrencyCode {
			return nil, apperrors.CurrencyMismatch(v.CurrencyCode, store.CurrencyCode)
		}
	}

	// Verify every media upload pre-tx.
	if err := s.verifyMedia(ctx, req.Media); err != nil {
		return nil, err
	}

	handle := strings.TrimSpace(req.Handle)
	if handle == "" {
		handle = GenerateHandle(title)
	}
	if handle == "" {
		return nil, apperrors.ValidationFailed("handle", "handle could not be generated from title")
	}

	status := req.Status
	if status == "" {
		status = StatusDraft
	}

	// Build aggregate with pre-assigned UUIDs so child rows can reference
	// parents without a roundtrip.
	productID := uuid.NewString()
	optSpecToID, optValSpecToID, options := buildOptions(productID, req.Options)
	variants, err := buildVariants(req.StoreID, req.Variants, optValSpecToID, req.Options)
	if err != nil {
		return nil, err
	}
	_ = optSpecToID // reserved for potential reverse lookup

	mediaRows := make([]Media, 0, len(req.Media))
	for _, m := range req.Media {
		mediaRows = append(mediaRows, Media{
			ID:         uuid.NewString(),
			StorageKey: m.StorageKey,
			URL:        m.URL,
			Alt:        m.Alt,
			Position:   m.Position,
			MediaType:  defaultMediaType(m.MediaType),
			VariantID:  m.VariantID,
			Width:      m.Width,
			Height:     m.Height,
			Bytes:      m.Bytes,
		})
	}

	catLinks := make([]ProductCategory, 0, len(req.CategoryIDs))
	for _, cid := range req.CategoryIDs {
		catLinks = append(catLinks, ProductCategory{CategoryID: cid})
	}

	description := Sanitize(req.Description)
	var descPtr *string
	if description != "" {
		descPtr = &description
	}

	agg := &Aggregate{
		Product: Product{
			ID:                productID,
			TenantID:          req.TenantID,
			StoreID:           req.StoreID,
			Handle:            handle,
			Title:             title,
			Description:       descPtr,
			Status:            status,
			Tags:              req.Tags,
			SEOTitle:          req.SEOTitle,
			SEODescription:    req.SEODescription,
			PrimaryCategoryID: req.PrimaryCategoryID,
			VendorID:          req.VendorID,
			CreatedBy:         req.CreatedBy,
			UpdatedBy:         req.CreatedBy,
		},
		Options:       options,
		Variants:      variants,
		Media:         mediaRows,
		CategoryLinks: catLinks,
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.CreateAggregateInTx(ctx, tx, agg); err != nil {
			return err
		}
		// Seed per-variant stock at the default location.
		for i := range agg.Variants {
			if err := s.repo.UpdateVariantStockInTx(ctx, tx,
				agg.Variants[i].ID, DefaultLocationID, req.Variants[i].InitialStock); err != nil {
				return err
			}
		}
		return s.enqueue(ctx, tx, req.TenantID, req.StoreID, productID, outbox.EventProductCreated)
	})
	if err != nil {
		return nil, err
	}

	return s.repo.GetByIDForStore(ctx, productID, req.StoreID, req.TenantID)
}

// ---------- UpdateBasics ----------

// UpdateBasics applies partial scalar updates + enqueues product.updated.
func (s *Service) UpdateBasics(ctx context.Context, req UpdateBasicsRequest) (*Aggregate, error) {
	// Pre-check existence so callers get a clean NotFound.
	if _, err := s.repo.GetByIDForStore(ctx, req.ID, req.StoreID, req.TenantID); err != nil {
		return nil, err
	}

	fields := map[string]any{}
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t == "" {
			return nil, apperrors.ValidationFailed("title", "title must not be empty")
		}
		fields["title"] = t
	}
	if req.Handle != nil {
		h := strings.TrimSpace(*req.Handle)
		if h == "" {
			return nil, apperrors.ValidationFailed("handle", "handle must not be empty")
		}
		fields["handle"] = h
	}
	if req.Description != nil {
		fields["description"] = Sanitize(*req.Description)
	}
	if req.Status != nil {
		if err := validateStatus(*req.Status); err != nil {
			return nil, err
		}
		fields["status"] = *req.Status
		if *req.Status == StatusActive {
			fields["published_at"] = gorm.Expr("coalesce(published_at, now())")
		}
	}
	if req.Tags != nil {
		fields["tags"] = *req.Tags
	}
	if req.SEOTitle != nil {
		fields["seo_title"] = *req.SEOTitle
	}
	if req.SEODescription != nil {
		fields["seo_description"] = *req.SEODescription
	}
	if req.PrimaryCategoryID != nil {
		fields["primary_category_id"] = *req.PrimaryCategoryID
	}
	if req.UpdatedBy != nil {
		fields["updated_by"] = *req.UpdatedBy
	}
	if len(fields) > 0 {
		fields["updated_at"] = gorm.Expr("now()")
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(fields) > 0 {
			if err := s.repo.UpdateBasicsInTx(ctx, tx, req.ID, req.StoreID, req.TenantID, fields); err != nil {
				return err
			}
		}
		return s.enqueue(ctx, tx, req.TenantID, req.StoreID, req.ID, outbox.EventProductUpdated)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetByIDForStore(ctx, req.ID, req.StoreID, req.TenantID)
}

// ---------- UpdateCategoryLinks ----------

// UpdateCategoryLinks replaces the category link set for a product and
// enqueues a product.updated outbox event.
func (s *Service) UpdateCategoryLinks(ctx context.Context, productID, storeID, tenantID string, categoryIDs []string) error {
	if _, err := s.repo.GetByIDForStore(ctx, productID, storeID, tenantID); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.ReplaceCategoryLinksInTx(ctx, tx, productID, categoryIDs); err != nil {
			return err
		}
		return s.enqueue(ctx, tx, tenantID, storeID, productID, outbox.EventProductUpdated)
	})
}

// ---------- UpdateMedia ----------

// UpdateMedia replaces the media set for a product. Every StorageKey is
// verified against the Uploader before the transaction opens.
func (s *Service) UpdateMedia(ctx context.Context, productID, storeID, tenantID string, mediaInputs []MediaInput) error {
	if _, err := s.repo.GetByIDForStore(ctx, productID, storeID, tenantID); err != nil {
		return err
	}
	if err := s.verifyMedia(ctx, mediaInputs); err != nil {
		return err
	}
	rows := make([]Media, 0, len(mediaInputs))
	for _, m := range mediaInputs {
		rows = append(rows, Media{
			ID:         uuid.NewString(),
			ProductID:  productID,
			StorageKey: m.StorageKey,
			URL:        m.URL,
			Alt:        m.Alt,
			Position:   m.Position,
			MediaType:  defaultMediaType(m.MediaType),
			VariantID:  m.VariantID,
			Width:      m.Width,
			Height:     m.Height,
			Bytes:      m.Bytes,
		})
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.ReplaceMediaInTx(ctx, tx, productID, rows); err != nil {
			return err
		}
		return s.enqueue(ctx, tx, tenantID, storeID, productID, outbox.EventProductUpdated)
	})
}

// ---------- Delete ----------

// Delete soft-deletes a product and enqueues product.deleted.
func (s *Service) Delete(ctx context.Context, id, storeID, tenantID string) error {
	if _, err := s.repo.GetByIDForStore(ctx, id, storeID, tenantID); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.SoftDeleteInTx(ctx, tx, id, storeID, tenantID); err != nil {
			return err
		}
		return s.enqueue(ctx, tx, tenantID, storeID, id, outbox.EventProductDeleted)
	})
}

// ---------- handler-facing convenience wrappers ----------
//
// List and Get are thin pass-throughs to the repository. They exist so
// HTTP handlers depend only on *Service and don't need to import or hold
// a repo reference for the read paths. Mutations already go through the
// service, so exposing reads here keeps the handler surface uniform.

// List wraps repo.ListAdmin.
func (s *Service) List(ctx context.Context, q ListAdminQuery) ([]Aggregate, int64, error) {
	return s.repo.ListAdmin(ctx, q)
}

// Get wraps repo.GetByIDForStore.
func (s *Service) Get(ctx context.Context, id, storeID, tenantID string) (*Aggregate, error) {
	return s.repo.GetByIDForStore(ctx, id, storeID, tenantID)
}

// ---------- helpers ----------

func (s *Service) enqueue(ctx context.Context, tx *gorm.DB, tenantID, storeID, productID, eventType string) error {
	payload, err := json.Marshal(map[string]any{
		"store_id":   storeID,
		"product_id": productID,
	})
	if err != nil {
		return fmt.Errorf("product: marshal %s payload: %w", eventType, err)
	}
	evt := &outbox.OutboxEvent{
		TenantID:    tenantID,
		Aggregate:   outbox.AggregateProduct,
		AggregateID: productID,
		EventType:   eventType,
		Payload:     datatypes.JSON(payload),
	}
	return s.outboxRepo.EnqueueInTx(ctx, tx, evt)
}

func (s *Service) verifyMedia(ctx context.Context, inputs []MediaInput) error {
	if s.uploader == nil || len(inputs) == 0 {
		return nil
	}
	for _, m := range inputs {
		if m.StorageKey == "" {
			return apperrors.ValidationFailed("media.storage_key", "storage_key must not be empty")
		}
		if _, err := s.uploader.Verify(ctx, m.StorageKey); err != nil {
			return apperrors.UploadNotFound(m.StorageKey)
		}
	}
	return nil
}

func validateStatus(s string) error {
	switch s {
	case StatusDraft, StatusActive, StatusArchived:
		return nil
	default:
		return apperrors.ValidationFailed("status", fmt.Sprintf("status %q is not valid", s))
	}
}

func defaultMediaType(t string) string {
	if t == "" {
		return MediaTypeImage
	}
	return t
}

func variantSpecsFromInputs(inputs []VariantInput) []VariantSpec {
	out := make([]VariantSpec, 0, len(inputs))
	for _, v := range inputs {
		out = append(out, VariantSpec{
			ID:           v.ID,
			SKU:          v.SKU,
			OptionValues: v.OptionValues,
		})
	}
	return out
}

// buildOptions turns OptionSpecs into gorm Option+OptionValue rows with
// pre-assigned IDs, and returns lookup maps keyed by option name and
// (option-name, value) tuple.
func buildOptions(productID string, specs []OptionSpec) (
	optByName map[string]string,
	valueByTuple map[string]string,
	rows []Option,
) {
	optByName = make(map[string]string, len(specs))
	valueByTuple = make(map[string]string)
	rows = make([]Option, 0, len(specs))
	for i, sp := range specs {
		optID := uuid.NewString()
		optByName[sp.Name] = optID
		vals := make([]OptionValue, 0, len(sp.Values))
		for j, v := range sp.Values {
			valID := v.ID
			if valID == "" {
				valID = uuid.NewString()
			}
			valueByTuple[sp.Name+"="+v.Value] = valID
			vals = append(vals, OptionValue{
				ID:       valID,
				OptionID: optID,
				Value:    v.Value,
				Position: j,
			})
		}
		rows = append(rows, Option{
			ID:        optID,
			ProductID: productID,
			Name:      sp.Name,
			Position:  i,
			Values:    vals,
		})
	}
	return
}

// buildVariants turns VariantInputs into gorm Variant rows with
// pre-assigned IDs + OptionValueLinks resolved against valueByTuple.
// Returns an error if any variant references an unknown option value.
func buildVariants(
	storeID string,
	inputs []VariantInput,
	valueByTuple map[string]string,
	optSpecs []OptionSpec,
) ([]Variant, error) {
	out := make([]Variant, 0, len(inputs))
	for _, in := range inputs {
		vid := in.ID
		if vid == "" {
			vid = uuid.NewString()
		}
		links := make([]VariantOptionValue, 0, len(in.OptionValues))
		for _, ref := range in.OptionValues {
			key := ref.OptionName + "=" + ref.Value
			valID, ok := valueByTuple[key]
			if !ok {
				return nil, apperrors.ValidationFailed("variants",
					fmt.Sprintf("variant references unknown option value %s", key))
			}
			// VariantID is populated by CreateAggregateInTx after the
			// variant row exists; leaving it empty prevents a duplicate
			// cascade insert from gorm's HasMany.
			links = append(links, VariantOptionValue{
				OptionValueID: valID,
			})
		}
		policy := in.InventoryPolicy
		if policy == "" {
			policy = InventoryPolicyDeny
		}
		out = append(out, Variant{
			ID:                vid,
			StoreID:           storeID,
			SKU:               in.SKU,
			Barcode:           in.Barcode,
			Price:             in.Price,
			CompareAtPrice:    in.CompareAtPrice,
			CostPrice:         in.CostPrice,
			CurrencyCode:      in.CurrencyCode,
			WeightGrams:       in.WeightGrams,
			InventoryPolicy:   policy,
			LowStockThreshold: in.LowStockThreshold,
			Position:          in.Position,
			OptionValueLinks:  links,
		})
	}
	_ = optSpecs
	return out, nil
}
