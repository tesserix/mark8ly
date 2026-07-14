// Package admin — media.go: HTTP handler for the admin media CRUD
// surface and the /media/upload-url signed URL endpoint. The upload-url
// path type-asserts the uploader to media.SignedURLGenerator so the
// dev fake (which cannot sign) cleanly returns 501.
package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// MediaHandler bundles dependencies for the admin media endpoints.
type MediaHandler struct {
	svc          *product.Service
	uploader     media.Uploader
	logger       *slog.Logger
	signedURLTTL time.Duration
	// resolver + subRepo + db are optional. When all three are wired, the
	// Create path enforces the per-plan images-per-product cap via
	// plangate.ImagesAllowed (applies grandfathering for existing products).
	// Production must always wire them; tests may leave them nil.
	resolver *plangate.PlanResolver
	subRepo  subscription.Repository
	subDB    *gorm.DB
}

// NewMediaHandler constructs a MediaHandler with the default 15-minute
// signed URL TTL. main.go may override via the exported field.
func NewMediaHandler(svc *product.Service, uploader media.Uploader, logger *slog.Logger) *MediaHandler {
	return &MediaHandler{
		svc:          svc,
		uploader:     uploader,
		logger:       logger,
		signedURLTTL: 15 * time.Minute,
	}
}

// SetPlanGate wires the plan resolver and subscription repository used to
// enforce the images-per-product cap at Create time. Pass the same *gorm.DB
// the subscription repository was constructed against. Without these, the
// cap is not enforced — production main.go must always wire this.
func (h *MediaHandler) SetPlanGate(resolver *plangate.PlanResolver, subRepo subscription.Repository, db *gorm.DB) {
	h.resolver = resolver
	h.subRepo = subRepo
	h.subDB = db
}

// UploadURL handles POST /admin/stores/:storeId/products/:id/media/upload-url.
// When the wired uploader does not implement SignedURLGenerator (e.g. the
// dev FakeUploader), the endpoint returns 501 Not Implemented.
func (h *MediaHandler) UploadURL(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	var req UploadURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	signer, ok := h.uploader.(media.SignedURLGenerator)
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "signed upload URLs require a real GCS bucket",
		})
		return
	}

	key := media.BuildStorageKey(tenantID, req.ContentHash, req.Filename)
	url, expiresAt, err := signer.SignedUploadURL(c.Request.Context(), key, req.ContentType, h.signedURLTTL)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, UploadURLResponse{
		URL:        url,
		StorageKey: key,
		ExpiresAt:  expiresAt,
	})
}

// Create handles POST /admin/stores/:storeId/products/:id/media.
// When the plan gate is wired, the per-plan images-per-product cap is
// enforced before the DB write. plangate.ImagesAllowed applies the
// grandfathering rule (existing products keep their cap after a
// downgrade) so a cap breach here means "this NEW media would push the
// product above the effective cap on the current plan".
func (h *MediaHandler) Create(c *gin.Context) {
	storeID := c.Param("storeId")
	productID := c.Param("id")
	tenantID := c.GetString("tenant_id")

	var req CreateMediaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	if err := h.enforceImageCap(c, tenantID, storeID, productID); err != nil {
		return
	}

	svcReq := toServiceAddMedia(req, productID, storeID, tenantID)
	m, err := h.svc.AddMedia(c.Request.Context(), svcReq)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusCreated, ToAdminMediaResponse(m))
}

// enforceImageCap returns non-nil after it has already written the 403
// response and the handler should abort. Returns nil to continue. When
// the gate dependencies are not wired (tests, dev), it is a no-op.
func (h *MediaHandler) enforceImageCap(c *gin.Context, tenantID, storeID, productID string) error {
	if h.resolver == nil || h.subRepo == nil || h.subDB == nil {
		return nil
	}
	tenantUUID, terr := uuid.Parse(tenantID)
	storeUUID, serr := uuid.Parse(storeID)
	if terr != nil || serr != nil {
		return nil // scope errors surface later from service layer
	}

	snap, err := h.svc.GetMediaSnapshot(c.Request.Context(), productID, storeID, tenantID)
	if err != nil {
		// Let the service layer's AddMedia return the real error (NotFound,
		// etc.) so we don't duplicate error-shape logic here.
		return nil
	}

	sub, err := h.subRepo.GetByStoreID(c.Request.Context(), h.subDB, tenantUUID, storeUUID)
	if err != nil {
		// Fail-closed: without a subscription row we can't evaluate the gate.
		// Apply the trial cap as the conservative default.
		cap := plangate.Limit(subscription.PlanTrial, plangate.FeatureImagesPerProduct)
		if cap != plangate.Unlimited && snap.CurrentCount >= cap {
			respondImageCap(c, subscription.PlanTrial, cap, snap.CurrentCount)
			return fmt.Errorf("plan gate: trial-fallback cap reached")
		}
		return nil
	}

	limit := plangate.ImagesAllowed(sub.Plan, snap.ProductCreatedAt, sub.LastPlanChangeAt)
	if limit == plangate.Unlimited {
		return nil
	}
	if snap.CurrentCount >= limit {
		respondImageCap(c, sub.Plan, limit, snap.CurrentCount)
		return fmt.Errorf("plan gate: images-per-product cap reached")
	}
	return nil
}

func respondImageCap(c *gin.Context, plan subscription.SubscriptionPlan, limit, current int) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error":   "plan_limit_exceeded",
		"message": fmt.Sprintf("This product is at its images cap (%d). Upgrade your plan to add more images.", limit),
		"feature": string(plangate.FeatureImagesPerProduct),
		"limit":   limit,
		"current": current,
		"plan":    string(plan),
	})
}

// Patch handles PATCH /admin/stores/:storeId/products/:id/media/:mediaId.
func (h *MediaHandler) Patch(c *gin.Context) {
	storeID := c.Param("storeId")
	productID := c.Param("id")
	mediaID := c.Param("mediaId")
	tenantID := c.GetString("tenant_id")

	var req UpdateMediaWireRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	svcReq := toServiceUpdateMedia(req, productID, mediaID, storeID, tenantID)
	if err := h.svc.UpdateMedia(c.Request.Context(), svcReq); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.Status(http.StatusNoContent)
}

// Recrop handles POST /admin/stores/:storeId/products/:id/media/:mediaId/recrop.
// It loads the target media row, generates a new content-addressed
// storage key for the cropped result, and returns signed URLs the
// client uses to download the pristine original and upload the cropped
// blob directly to GCS. The row is not mutated; the client commits via
// PATCH /media/:mediaId with the returned new_storage_key.
//
// Returns 501 Not Implemented when the wired uploader does not
// implement the signing interfaces (dev FakeUploader).
func (h *MediaHandler) Recrop(c *gin.Context) {
	storeID := c.Param("storeId")
	productID := c.Param("id")
	mediaID := c.Param("mediaId")
	tenantID := c.GetString("tenant_id")

	var req RecropMediaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	putSigner, okPut := h.uploader.(media.SignedURLGenerator)
	getSigner, okGet := h.uploader.(media.SignedReadURLGenerator)
	if !okPut || !okGet {
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "signed recrop URLs require a real GCS bucket",
		})
		return
	}

	svcReq := product.RecropMediaRequest{
		ProductID:   productID,
		MediaID:     mediaID,
		StoreID:     storeID,
		TenantID:    tenantID,
		Filename:    req.Filename,
		ContentType: req.ContentType,
	}
	resp, err := h.svc.RecropMedia(c.Request.Context(), svcReq, putSigner, getSigner, h.signedURLTTL)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, RecropMediaResponse{
		SourceOriginalURL: resp.SourceOriginalURL,
		UploadURL:         resp.UploadURL,
		NewStorageKey:     resp.NewStorageKey,
		ExpiresAt:         resp.ExpiresAt,
	})
}

// Delete handles DELETE /admin/stores/:storeId/products/:id/media/:mediaId.
func (h *MediaHandler) Delete(c *gin.Context) {
	storeID := c.Param("storeId")
	productID := c.Param("id")
	mediaID := c.Param("mediaId")
	tenantID := c.GetString("tenant_id")

	if err := h.svc.DeleteMedia(c.Request.Context(), productID, mediaID, storeID, tenantID); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.Status(http.StatusNoContent)
}
