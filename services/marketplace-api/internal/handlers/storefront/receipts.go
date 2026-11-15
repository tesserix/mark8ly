package storefront

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/instore"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ReceiptReader reads in-store receipts by token (#1106);
// instore.ReceiptService implements it.
type ReceiptReader interface {
	Load(ctx context.Context, storeID uuid.UUID, token string) (*instore.Receipt, error)
	Resend(ctx context.Context, storeID uuid.UUID, token string) error
}

// ReceiptsHandler serves the receipt page's data. The token is the only
// credential, so a token is good only on the address of the store it
// belongs to.
type ReceiptsHandler struct {
	receipts ReceiptReader
	logger   *slog.Logger
}

func NewReceiptsHandler(r ReceiptReader, logger *slog.Logger) *ReceiptsHandler {
	return &ReceiptsHandler{receipts: r, logger: logger}
}

func (h *ReceiptsHandler) storeID(c *gin.Context) (uuid.UUID, bool) {
	v, _ := c.Get("store")
	s, _ := v.(*stores.Store)
	if s == nil {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s.ID)
	return id, err == nil
}

func (h *ReceiptsHandler) fail(c *gin.Context, err error) {
	if errors.Is(err, apperrors.ErrNotFound) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	if h.logger != nil {
		h.logger.Error("storefront: receipt", "err", err)
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal"})
}

// Get handles GET /storefront/stores/:storeSlug/receipts/:token.
func (h *ReceiptsHandler) Get(c *gin.Context) {
	sid, ok := h.storeID(c)
	if !ok {
		h.fail(c, apperrors.NotFound("store"))
		return
	}
	r, err := h.receipts.Load(c.Request.Context(), sid, c.Param("token"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, r)
}

// Resend handles POST /storefront/stores/:storeSlug/receipts/:token/resend.
// Always 202 for a real token: the caller never learns whether the order
// has an address or whether an email went out.
func (h *ReceiptsHandler) Resend(c *gin.Context) {
	sid, ok := h.storeID(c)
	if !ok {
		h.fail(c, apperrors.NotFound("store"))
		return
	}
	if err := h.receipts.Resend(c.Request.Context(), sid, c.Param("token")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}
