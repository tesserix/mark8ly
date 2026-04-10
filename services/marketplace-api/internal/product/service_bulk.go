// Package product — service_bulk.go: per-id bulk actions with FGA
// enforcement. Each action iterates the requested product IDs, checks
// FGA permission per id, applies the mutation, and accumulates per-id
// results. Partial success is the norm — the caller always receives
// HTTP 200 with a results array.
//
// Locked decisions (M7d plan):
//   - archive  = set status "archived"
//   - unarchive = set status "draft"
//   - publish   = set status "active" (+ published_at)
//   - unpublish = set status "draft"
//   - delete    = soft-delete (existing Service.Delete)
//   - assign_category = replace category links for each product
//
// FGA check runs BEFORE the mutation for each id. Unauthorized ids
// return {status: "error", error: "forbidden"} in the results array.
package product

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// BulkAction enumerates the supported bulk operations.
type BulkAction string

const (
	BulkActionArchive        BulkAction = "archive"
	BulkActionUnarchive      BulkAction = "unarchive"
	BulkActionPublish        BulkAction = "publish"
	BulkActionUnpublish      BulkAction = "unpublish"
	BulkActionDelete         BulkAction = "delete"
	BulkActionAssignCategory BulkAction = "assign_category"
)

// ValidBulkActions is the set of recognized action strings.
var ValidBulkActions = map[BulkAction]bool{
	BulkActionArchive:        true,
	BulkActionUnarchive:      true,
	BulkActionPublish:        true,
	BulkActionUnpublish:      true,
	BulkActionDelete:         true,
	BulkActionAssignCategory: true,
}

// BulkRequest is the service-level DTO for bulk product actions.
type BulkRequest struct {
	StoreID     string
	TenantID    string
	UserID      string
	Action      BulkAction
	ProductIDs  []string
	CategoryIDs []string // only used for assign_category
}

// BulkResult is the per-id outcome of a bulk action.
type BulkResult struct {
	ID     string `json:"id"`
	Status string `json:"status"` // "ok" or "error"
	Error  string `json:"error,omitempty"`
}

// BulkCheckFunc is the FGA check function injected by the handler so
// the service layer does not depend on the authz package directly.
type BulkCheckFunc func(ctx context.Context, userID, productID string) (bool, error)

// MaxBulkIDs is the hard cap on product IDs per bulk request (spec §3).
const MaxBulkIDs = 100

// Bulk applies the requested action to each product ID, checking FGA
// permissions per id via the provided check function. Returns a result
// for every requested ID.
func (s *Service) Bulk(ctx context.Context, req BulkRequest, fgaCheck BulkCheckFunc) []BulkResult {
	results := make([]BulkResult, 0, len(req.ProductIDs))

	for _, pid := range req.ProductIDs {
		r := s.bulkOne(ctx, req, pid, fgaCheck)
		results = append(results, r)
	}

	return results
}

func (s *Service) bulkOne(ctx context.Context, req BulkRequest, productID string, fgaCheck BulkCheckFunc) BulkResult {
	// FGA check per id.
	allowed, err := fgaCheck(ctx, req.UserID, productID)
	if err != nil {
		s.logger.Error("bulk fga check", "product_id", productID, "err", err)
		return BulkResult{ID: productID, Status: "error", Error: "authorization_error"}
	}
	if !allowed {
		return BulkResult{ID: productID, Status: "error", Error: "forbidden"}
	}

	switch req.Action {
	case BulkActionArchive:
		err = s.bulkSetStatus(ctx, productID, req.StoreID, req.TenantID, StatusArchived)
	case BulkActionUnarchive:
		err = s.bulkSetStatus(ctx, productID, req.StoreID, req.TenantID, StatusDraft)
	case BulkActionPublish:
		err = s.bulkPublish(ctx, productID, req.StoreID, req.TenantID)
	case BulkActionUnpublish:
		err = s.bulkSetStatus(ctx, productID, req.StoreID, req.TenantID, StatusDraft)
	case BulkActionDelete:
		err = s.Delete(ctx, productID, req.StoreID, req.TenantID)
	case BulkActionAssignCategory:
		err = s.UpdateCategoryLinks(ctx, productID, req.StoreID, req.TenantID, req.CategoryIDs)
	default:
		return BulkResult{ID: productID, Status: "error", Error: "unsupported_action"}
	}

	if err != nil {
		var ae *apperrors.Error
		if errors.As(err, &ae) {
			return BulkResult{ID: productID, Status: "error", Error: string(ae.Code)}
		}
		s.logger.Error("bulk action", "action", req.Action, "product_id", productID, "err", err)
		return BulkResult{ID: productID, Status: "error", Error: "internal"}
	}
	return BulkResult{ID: productID, Status: "ok"}
}

// bulkSetStatus sets the product status inside a transaction and enqueues
// product.updated. Does NOT set published_at — use bulkPublish for that.
func (s *Service) bulkSetStatus(ctx context.Context, id, storeID, tenantID, status string) error {
	if _, err := s.repo.GetByIDForStore(ctx, id, storeID, tenantID); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fields := map[string]any{"status": status}
		if err := s.repo.UpdateBasicsInTx(ctx, tx, id, storeID, tenantID, fields); err != nil {
			return err
		}
		return s.enqueue(ctx, tx, tenantID, storeID, id, outbox.EventProductUpdated)
	})
}

// bulkPublish sets status=active and published_at=now (if not already set)
// inside a transaction and enqueues product.updated.
func (s *Service) bulkPublish(ctx context.Context, id, storeID, tenantID string) error {
	if _, err := s.repo.GetByIDForStore(ctx, id, storeID, tenantID); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		fields := map[string]any{
			"status":       StatusActive,
			"published_at": gorm.Expr("coalesce(published_at, ?)", now),
		}
		if err := s.repo.UpdateBasicsInTx(ctx, tx, id, storeID, tenantID, fields); err != nil {
			return err
		}
		return s.enqueue(ctx, tx, tenantID, storeID, id, outbox.EventProductUpdated)
	})
}
