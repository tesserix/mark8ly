// Package storefront — wishlist.go: Storefront wishlist handlers (C4).
// Add, remove, list, and check wishlisted products. All routes require
// RequireCustomerAuth middleware.
package storefront

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/wishlist"
)

// WishlistHandler bundles dependencies for storefront wishlist endpoints.
type WishlistHandler struct {
	repo   wishlist.Repository
	logger *slog.Logger
}

// NewWishlistHandler constructs a WishlistHandler.
func NewWishlistHandler(repo wishlist.Repository, logger *slog.Logger) *WishlistHandler {
	return &WishlistHandler{repo: repo, logger: logger}
}

// --- Request DTOs ---

type addToWishlistRequest struct {
	ProductID string `json:"product_id" binding:"required,uuid"`
}

// --- Handlers ---

// List handles GET /account/wishlist — paginated wishlist with product details.
func (h *WishlistHandler) List(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	items, total, err := h.repo.List(c.Request.Context(), profile.ID.String(), page, limit)
	if err != nil {
		h.logger.Error("failed to list wishlist", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to list wishlist",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  items,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// Add handles POST /account/wishlist — add a product to the wishlist.
func (h *WishlistHandler) Add(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}
	store := h.resolveStore(c)
	if store == nil {
		return
	}

	var req addToWishlistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "product_id is required and must be a valid UUID",
		})
		return
	}

	item := &wishlist.Wishlist{
		TenantID:   store.TenantID,
		StoreID:    store.ID,
		CustomerID: profile.ID.String(),
		ProductID:  req.ProductID,
	}

	if err := h.repo.Add(c.Request.Context(), item); err != nil {
		h.logger.Error("failed to add to wishlist", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to add to wishlist",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": gin.H{
			"product_id": req.ProductID,
			"wishlisted": true,
		},
	})
}

// Remove handles DELETE /account/wishlist/:productId — remove from wishlist.
func (h *WishlistHandler) Remove(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	productID := c.Param("productId")
	if productID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "Product ID is required",
		})
		return
	}

	if err := h.repo.Remove(c.Request.Context(), profile.ID.String(), productID); err != nil {
		h.logger.Error("failed to remove from wishlist", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to remove from wishlist",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"product_id": productID,
			"wishlisted": false,
		},
	})
}

// Check handles GET /account/wishlist/check/:productId — check if wishlisted.
func (h *WishlistHandler) Check(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	productID := c.Param("productId")
	if productID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "Product ID is required",
		})
		return
	}

	wishlisted, err := h.repo.Check(c.Request.Context(), profile.ID.String(), productID)
	if err != nil {
		h.logger.Error("failed to check wishlist", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to check wishlist",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"product_id": productID,
			"wishlisted": wishlisted,
		},
	})
}

// --- helpers ---

func (h *WishlistHandler) mustGetProfile(c *gin.Context) *customer.CustomerProfile {
	val, exists := c.Get(CustomerProfileKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "Authentication required",
		})
		return nil
	}
	profile, ok := val.(*customer.CustomerProfile)
	if !ok || profile == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "Authentication required",
		})
		return nil
	}
	return profile
}

func (h *WishlistHandler) resolveStore(c *gin.Context) *stores.Store {
	storeVal, ok := c.Get("store")
	if !ok {
		respondNotFound(c)
		return nil
	}
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		respondNotFound(c)
		return nil
	}
	return store
}
