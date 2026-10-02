// Package storefront — personalisation_uploads.go: the buyer's artwork
// surface (#963).
//
// # Authorisation is the cart token, and only the cart token
//
// These routes are public: most buyers are guests, so there is no
// authenticated customer to check an upload against. The cart token is
// therefore the whole of the authorisation, and every handler resolves it
// the way cart_holds does — body first, then the httpOnly cookie.
//
// Unlike cart_holds, a missing or unparseable token is REFUSED rather than
// minted. Minting is right there, where a first cart write legitimately
// has no token yet; here it would hand the caller a fresh identity that
// owns nothing, and the only thing it could then do is create an orphan.
package storefront

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/personalisationupload"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// PersonalisationUploadsHandler serves the buyer upload surface.
type PersonalisationUploadsHandler struct {
	svc    *personalisationupload.Service
	logger *slog.Logger
}

// NewPersonalisationUploadsHandler constructs the handler.
func NewPersonalisationUploadsHandler(
	svc *personalisationupload.Service, logger *slog.Logger,
) *PersonalisationUploadsHandler {
	return &PersonalisationUploadsHandler{svc: svc, logger: logger}
}

// cartTokenFrom resolves the owning cart, or writes a 400 and returns "".
func (h *PersonalisationUploadsHandler) cartTokenFrom(c *gin.Context, fromBody string) string {
	token := fromBody
	if token == "" {
		if ck, err := c.Cookie(CartTokenCookie); err == nil {
			token = ck
		}
	}
	if _, err := uuid.Parse(token); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "a cart is required before uploading",
		})
		return ""
	}
	return token
}

// respond maps a service error onto a status.
//
// This surface is anonymous, so it follows the storefront's rule from
// errors.go: typed codes are not leaked and anything unrecognised is an
// opaque 500. Two cases are mapped deliberately:
//
//   - ErrUploadsDisabled is 501, not 500. No private bucket is a
//     deployment that has not enabled the feature, not a fault.
//   - A validation failure carries its MESSAGE through, because this is
//     the one place the text is the product: "that image is too large"
//     and "HEIC photos need converting first" are the difference between
//     a shopper fixing it and a shopper giving up. The code stays off the
//     wire; only the sentence the buyer needs goes out.
func (h *PersonalisationUploadsHandler) respond(c *gin.Context, err error) {
	switch {
	case errors.Is(err, personalisationupload.ErrUploadsDisabled):
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "image uploads are not enabled on this store",
		})
	case errors.Is(err, apperrors.ErrNotFound):
		// Deliberately indistinguishable from "belongs to another cart":
		// a separate 403 would turn this into an oracle for whether an
		// upload id exists.
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "upload not found",
		})
	case errors.Is(err, apperrors.ErrValidationFailed):
		msg := "that upload could not be accepted"
		var ae *apperrors.Error
		if errors.As(err, &ae) && ae.Message != "" {
			msg = ae.Message
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_error", "message": msg})
	default:
		respondInternal(c, h.logger, err)
	}
}

type createUploadBody struct {
	ProductID   string `json:"product_id"   binding:"required,uuid"`
	FieldID     string `json:"field_id"     binding:"required,uuid"`
	Filename    string `json:"filename"     binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
	ContentHash string `json:"content_hash"`
	CartToken   string `json:"cart_token"`
}

// CreateUploadURL handles POST /storefront/stores/:storeSlug/personalisation/upload-url.
func (h *PersonalisationUploadsHandler) CreateUploadURL(c *gin.Context) {
	var body createUploadBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "upload request could not be parsed",
		})
		return
	}
	token := h.cartTokenFrom(c, body.CartToken)
	if token == "" {
		return
	}
	store := c.MustGet("store").(*stores.Store)

	res, err := h.svc.CreateUploadURL(c.Request.Context(), personalisationupload.CreateRequest{
		StoreID:     store.ID,
		TenantID:    store.TenantID,
		ProductID:   body.ProductID,
		FieldID:     body.FieldID,
		CartToken:   token,
		Filename:    body.Filename,
		ContentType: body.ContentType,
		ContentHash: body.ContentHash,
	})
	if err != nil {
		h.respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

type confirmBody struct {
	CartToken string `json:"cart_token"`
	WidthPx   *int   `json:"width_px"`
	HeightPx  *int   `json:"height_px"`
}

// Confirm handles POST .../personalisation/uploads/:uploadId/confirm.
func (h *PersonalisationUploadsHandler) Confirm(c *gin.Context) {
	var body confirmBody
	// An empty body is fine: dimensions are advisory and the token may
	// come from the cookie.
	_ = c.ShouldBindJSON(&body)

	token := h.cartTokenFrom(c, body.CartToken)
	if token == "" {
		return
	}
	up, err := h.svc.Confirm(c.Request.Context(), c.Param("uploadId"), token, body.WidthPx, body.HeightPx)
	if err != nil {
		h.respond(c, err)
		return
	}
	c.JSON(http.StatusOK, up)
}

type cropBody struct {
	CartToken string `json:"cart_token"`
	Crop      struct {
		X        int `json:"x"`
		Y        int `json:"y"`
		W        int `json:"w"`
		H        int `json:"h"`
		Rotation int `json:"rotation"`
	} `json:"crop" binding:"required"`
}

// PrepareCrop handles PATCH .../personalisation/uploads/:uploadId/crop.
func (h *PersonalisationUploadsHandler) PrepareCrop(c *gin.Context) {
	var body cropBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "crop request could not be parsed",
		})
		return
	}
	token := h.cartTokenFrom(c, body.CartToken)
	if token == "" {
		return
	}
	res, err := h.svc.PrepareCrop(c.Request.Context(), c.Param("uploadId"), token,
		personalisationupload.CropRect{
			X: body.Crop.X, Y: body.Crop.Y, W: body.Crop.W, H: body.Crop.H,
			Rotation: body.Crop.Rotation,
		})
	if err != nil {
		h.respond(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// Preview handles GET .../personalisation/uploads/:uploadId/preview.
//
// Returns a short-lived signed URL rather than redirecting to it, so the
// storefront can decide how to render and the URL never ends up in a
// browser's history or a referrer header.
func (h *PersonalisationUploadsHandler) Preview(c *gin.Context) {
	token := h.cartTokenFrom(c, c.Query("cart_token"))
	if token == "" {
		return
	}
	url, expiresAt, err := h.svc.PreviewURL(c.Request.Context(), c.Param("uploadId"), token)
	if err != nil {
		h.respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url, "expires_at": expiresAt})
}

// Delete handles DELETE .../personalisation/uploads/:uploadId.
func (h *PersonalisationUploadsHandler) Delete(c *gin.Context) {
	token := h.cartTokenFrom(c, c.Query("cart_token"))
	if token == "" {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), c.Param("uploadId"), token); err != nil {
		h.respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
