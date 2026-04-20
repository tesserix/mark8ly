// Package product — service_single_media.go: AddMedia, UpdateMedia,
// DeleteMedia operate on ONE product_media row at a time. Each opens
// its own tx and enqueues one product.updated outbox event.
package product

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// MediaSnapshot is the minimal view of a product needed to evaluate a
// per-plan media-count gate. Exposed so handlers can compute
// plangate.ImagesAllowed without coupling product.Service to plangate.
type MediaSnapshot struct {
	CurrentCount     int
	ProductCreatedAt time.Time
}

// GetMediaSnapshot returns the current media count for a product and the
// product's CreatedAt so callers can evaluate per-plan image caps
// (plangate.ImagesAllowed takes product CreatedAt for grandfathering).
// Returns apperrors.NotFound when the product doesn't belong to the
// (tenant, store) scope.
func (s *Service) GetMediaSnapshot(ctx context.Context, productID, storeID, tenantID string) (MediaSnapshot, error) {
	agg, err := s.repo.GetByIDForStore(ctx, productID, storeID, tenantID)
	if err != nil {
		return MediaSnapshot{}, err
	}
	return MediaSnapshot{
		CurrentCount:     len(agg.Media),
		ProductCreatedAt: agg.Product.CreatedAt,
	}, nil
}

// AddMediaRequest is the service-level DTO for inserting a single
// product_media row.
type AddMediaRequest struct {
	ProductID  string
	StoreID    string
	TenantID   string
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

// AddMedia verifies the upload exists (pre-tx) then inserts one
// product_media row and enqueues product.updated.
func (s *Service) AddMedia(ctx context.Context, req AddMediaRequest) (*Media, error) {
	if req.StorageKey == "" {
		return nil, apperrors.ValidationFailed("media.storage_key", "storage_key must not be empty")
	}
	// Verify product belongs to this tenant/store.
	if _, err := s.repo.GetByIDForStore(ctx, req.ProductID, req.StoreID, req.TenantID); err != nil {
		return nil, err
	}
	// Pre-tx upload verification.
	if s.uploader != nil {
		if _, err := s.uploader.Verify(ctx, req.StorageKey); err != nil {
			if errors.Is(err, media.ErrNotFound) {
				return nil, apperrors.UploadNotFound(req.StorageKey)
			}
			return nil, apperrors.UploadNotFound(req.StorageKey)
		}
	}

	// Stamp the immutable Cache-Control on the freshly-uploaded object
	// so any CDN/browser caches it aggressively. Best-effort: log and
	// continue on failure so a misconfigured GCS IAM doesn't break the
	// finalize path entirely.
	if setter, ok := s.uploader.(media.MetadataSetter); ok && s.mediaCacheControl != "" {
		if err := setter.SetCacheControl(ctx, req.StorageKey, s.mediaCacheControl); err != nil {
			s.logger.Warn("media: set cache-control failed",
				"storage_key", req.StorageKey, "err", err)
		}
	}

	// Persist a publicly-resolvable URL on product_media.url so storefront
	// renders it directly. Falls back to whatever the caller passed (the
	// upload client today passes the raw storage key) when no base URL is
	// configured — matches legacy behavior.
	persistedURL := req.URL
	if s.mediaPublicBaseURL != "" {
		persistedURL = s.mediaPublicBaseURL + "/" + req.StorageKey
	}

	row := &Media{
		ID:              uuid.NewString(),
		ProductID:       req.ProductID,
		StorageKey:      req.StorageKey,
		GcsPathOriginal: req.StorageKey,
		URL:        persistedURL,
		Alt:        req.Alt,
		Position:   req.Position,
		MediaType:  defaultMediaType(req.MediaType),
		VariantID:  req.VariantID,
		Width:      req.Width,
		Height:     req.Height,
		Bytes:      req.Bytes,
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.InsertMediaInTx(ctx, tx, row); err != nil {
			return err
		}
		return s.enqueue(ctx, tx, req.TenantID, req.StoreID, req.ProductID, outbox.EventProductUpdated)
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// UpdateMediaRequest carries the non-nil fields to apply to a single
// product_media row.
type UpdateMediaRequest struct {
	ProductID  string
	MediaID    string
	StoreID    string
	TenantID   string
	Alt        *string
	Position   *int
	URL        *string
	VariantID  *string
	StorageKey *string
}

// UpdateMedia applies non-nil fields to the given media row. Tenant/
// store scoping is enforced by the repo via a products subquery.
func (s *Service) UpdateMedia(ctx context.Context, req UpdateMediaRequest) error {
	fields := map[string]any{}
	if req.Alt != nil {
		fields["alt"] = *req.Alt
	}
	if req.Position != nil {
		fields["position"] = *req.Position
	}
	if req.URL != nil {
		fields["url"] = *req.URL
	}
	if req.VariantID != nil {
		fields["variant_id"] = *req.VariantID
	}
	if req.StorageKey != nil {
		// storage_key updates commit a fresh recrop upload. gcs_path_original
		// is intentionally NOT touched so future recrops still read the
		// pristine source.
		fields["storage_key"] = *req.StorageKey
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(fields) > 0 {
			if err := s.repo.UpdateMediaInTx(ctx, tx, req.ProductID, req.MediaID, req.StoreID, req.TenantID, fields); err != nil {
				return err
			}
		}
		return s.enqueue(ctx, tx, req.TenantID, req.StoreID, req.ProductID, outbox.EventProductUpdated)
	})
}

// DeleteMedia removes a single product_media row. The GCS object itself
// is not touched — a later sweep handles orphaned storage keys.
func (s *Service) DeleteMedia(ctx context.Context, productID, mediaID, storeID, tenantID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.DeleteMediaInTx(ctx, tx, productID, mediaID, storeID, tenantID); err != nil {
			return err
		}
		return s.enqueue(ctx, tx, tenantID, storeID, productID, outbox.EventProductUpdated)
	})
}
