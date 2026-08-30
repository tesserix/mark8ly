package storefront

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// CartTokenCookie is the first-party cookie carrying the server-minted cart
// identity.
//
// httpOnly: the token identifies a stock reservation, so script access buys
// the storefront nothing and costs XSS exposure. The storefront learns its
// token from the response body when it needs it; the cookie is what makes
// the identity survive a reload.
const CartTokenCookie = "mk_cart_token"

// HoldTTL is how long a cart keeps its units.
//
// Placed AT CART-ADD, not at checkout-start — decided 2026-08-30 as the
// epic's literal intent: the shopper who adds first keeps the unit. The cost
// is real and accepted: an abandoned tab parks the last unit for fifteen
// minutes, and abandoned carts are the norm. If that proves too expensive on
// scarce variants, the lever is this constant plus a shorter hold at
// cart-add, not a change to the mechanism.
const HoldTTL = 15 * time.Minute

// CartHoldsHandler serves the storefront's stock-hold surface (#232).
type CartHoldsHandler struct {
	db     *gorm.DB
	holds  *stockhold.Repository
	logger *slog.Logger
}

func NewCartHoldsHandler(db *gorm.DB, holds *stockhold.Repository, logger *slog.Logger) *CartHoldsHandler {
	return &CartHoldsHandler{db: db, holds: holds, logger: logger}
}

type cartHoldItem struct {
	VariantID string `json:"variant_id" binding:"required,uuid"`
	Quantity  int    `json:"quantity"   binding:"required,min=1"`
}

type cartHoldsRequest struct {
	CartToken string         `json:"cart_token"`
	Items     []cartHoldItem `json:"items" binding:"required,min=1,dive"`
}

type cartHoldItemResult struct {
	VariantID string `json:"variant_id"`
	// Status is "held" or "insufficient".
	Status string `json:"status"`
	// Available is what the shopper can actually have right now. Reported
	// so the storefront can say "only 2 left" rather than a bare failure.
	Available int `json:"available"`
}

// Place creates or refreshes holds for a cart.
//
// # Per-item status rather than all-or-nothing
//
// A cart of five items where one is short must tell the shopper WHICH one.
// Failing the whole request would lose the other four holds to whoever asks
// next, and would give the storefront nothing to render.
//
// # One transaction for the whole cart
//
// Each item's hold takes a row lock on its own variant_stock row, and they
// commit together. A partial commit would leave a cart holding some of what
// the shopper believes they have.
func (h *CartHoldsHandler) Place(c *gin.Context) {
	var req cartHoldsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "cart hold request could not be parsed"})
		return
	}

	cartToken := req.CartToken
	if cartToken == "" {
		if ck, err := c.Cookie(CartTokenCookie); err == nil {
			cartToken = ck
		}
	}
	if _, err := uuid.Parse(cartToken); err != nil {
		// Mint rather than reject: a first cart write has no token yet, and
		// an unparseable one from a stale client is not worth an error the
		// shopper cannot act on.
		cartToken = uuid.NewString()
	}

	store := c.MustGet("store").(*stores.Store)
	storeID := store.ID
	expiresAt := time.Now().Add(HoldTTL)
	results := make([]cartHoldItemResult, 0, len(req.Items))

	err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range req.Items {
			// Ownership check FIRST. This surface is unauthenticated beyond
			// the shared storefront key, so without it a caller could hold
			// — and probe the stock of — any variant in the estate through
			// any store's slug.
			var owned int64
			if err := tx.Raw(
				`SELECT count(*) FROM product_variants WHERE id = ? AND store_id = ?`,
				item.VariantID, storeID).Scan(&owned).Error; err != nil {
				return err
			}
			if owned == 0 {
				return errVariantNotInStore
			}

			err := h.holds.Hold(c.Request.Context(), tx, cartToken, item.VariantID,
				product.DefaultLocationID, item.Quantity, HoldTTL)
			switch {
			case err == nil:
				results = append(results, cartHoldItemResult{
					VariantID: item.VariantID, Status: "held", Available: item.Quantity,
				})
			case errors.Is(err, stockhold.ErrInsufficientStock):
				avail, aerr := h.holds.Available(c.Request.Context(), tx, item.VariantID, product.DefaultLocationID, cartToken)
				if aerr != nil {
					return aerr
				}
				results = append(results, cartHoldItemResult{
					VariantID: item.VariantID, Status: "insufficient", Available: avail,
				})
			default:
				return err
			}
		}
		return nil
	})

	switch {
	case errors.Is(err, errVariantNotInStore):
		c.JSON(http.StatusNotFound, gin.H{"error": "variant_not_found", "message": "variant does not belong to this store"})
		return
	case err != nil:
		if h.logger != nil {
			h.logger.Error("cart holds: place failed", "err", err, "store_id", storeID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "could not place stock holds"})
		return
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(CartTokenCookie, cartToken, int(HoldTTL.Seconds()), "/", "", c.Request.TLS != nil, true)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"cart_token": cartToken,
		"expires_at": expiresAt.UTC(),
		"items":      results,
	}})
}

// Release drops a cart's holds, for an explicit cart clear.
//
// Idempotent and deliberately incurious: releasing a cart that holds nothing
// is a success, because the caller's intent — "this cart holds nothing" — is
// satisfied either way.
func (h *CartHoldsHandler) Release(c *gin.Context) {
	cartToken := c.Param("cartToken")
	if _, err := uuid.Parse(cartToken); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_cart_token", "message": "cart_token must be a UUID"})
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		return h.holds.Release(c.Request.Context(), tx, cartToken)
	}); err != nil {
		if h.logger != nil {
			h.logger.Error("cart holds: release failed", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "could not release stock holds"})
		return
	}

	c.SetCookie(CartTokenCookie, "", -1, "/", "", c.Request.TLS != nil, true)
	c.Status(http.StatusNoContent)
}

var errVariantNotInStore = errors.New("storefront: variant does not belong to this store")
