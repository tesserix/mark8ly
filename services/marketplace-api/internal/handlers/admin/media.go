// Package admin — media.go: HTTP handler for the admin media CRUD
// surface and the /media/upload-url signed URL endpoint. The upload-url
// path type-asserts the uploader to media.SignedURLGenerator so the
// dev fake (which cannot sign) cleanly returns 501.
package admin

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// MediaHandler bundles dependencies for the admin media endpoints.
type MediaHandler struct {
	svc          *product.Service
	uploader     media.Uploader
	logger       *slog.Logger
	signedURLTTL time.Duration
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
func (h *MediaHandler) Create(c *gin.Context) {
	storeID := c.Param("storeId")
	productID := c.Param("id")
	tenantID := c.GetString("tenant_id")

	var req CreateMediaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
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
		ProductID: productID,
		MediaID:   mediaID,
		StoreID:   storeID,
		TenantID:  tenantID,
		Filename:  req.Filename,
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
