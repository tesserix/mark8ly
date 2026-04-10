// Package storefront — customer_account.go: Storefront account handlers.
// GET/PATCH profile, addresses CRUD. All routes require RequireCustomerAuth.
package storefront

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/customer"
)

// CustomerAccountHandler serves the storefront /account/* routes.
type CustomerAccountHandler struct {
	db          *gorm.DB
	repo        customer.Repository
	customerSvc *customer.Service
	logger      *slog.Logger
}

// NewCustomerAccountHandler constructs a CustomerAccountHandler.
func NewCustomerAccountHandler(
	db *gorm.DB,
	repo customer.Repository,
	customerSvc *customer.Service,
	logger *slog.Logger,
) *CustomerAccountHandler {
	return &CustomerAccountHandler{
		db:          db,
		repo:        repo,
		customerSvc: customerSvc,
		logger:      logger,
	}
}

// GetProfile handles GET /storefront/stores/:storeSlug/account.
func (h *CustomerAccountHandler) GetProfile(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": mapProfileToResponse(profile)})
}

// UpdateProfile handles PATCH /storefront/stores/:storeSlug/account.
func (h *CustomerAccountHandler) UpdateProfile(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	var req customer.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	// Build update map — only non-nil fields.
	updates := make(map[string]any)
	if req.FirstName != nil {
		updates["first_name"] = strings.TrimSpace(*req.FirstName)
	}
	if req.LastName != nil {
		updates["last_name"] = strings.TrimSpace(*req.LastName)
	}
	if req.Phone != nil {
		updates["phone"] = strings.TrimSpace(*req.Phone)
	}
	if req.AvatarURL != nil {
		av := strings.TrimSpace(*req.AvatarURL)
		if av != "" && !strings.HasPrefix(av, "https://storage.googleapis.com/") {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "validation_error",
				"message": "avatar_url must be a Google Cloud Storage URL",
			})
			return
		}
		updates["avatar_url"] = av
	}
	if req.MarketingOptIn != nil {
		updates["marketing_opt_in"] = *req.MarketingOptIn
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": mapProfileToResponse(profile)})
		return
	}

	updated, err := h.repo.UpdateProfile(c.Request.Context(), profile.ID, updates)
	if err != nil {
		h.logger.Error("failed to update customer profile",
			"error", err,
			"profile_id", profile.ID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to update profile",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mapProfileToResponse(updated)})
}

// ListAddresses handles GET /storefront/stores/:storeSlug/account/addresses.
func (h *CustomerAccountHandler) ListAddresses(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	addrs, err := h.repo.ListAddresses(c.Request.Context(), profile.ID)
	if err != nil {
		h.logger.Error("failed to list addresses", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to list addresses",
		})
		return
	}

	resp := make([]customer.AddressResponse, 0, len(addrs))
	for _, a := range addrs {
		resp = append(resp, mapAddressToResponse(&a))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// CreateAddress handles POST /storefront/stores/:storeSlug/account/addresses.
func (h *CustomerAccountHandler) CreateAddress(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	var req customer.CreateAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	addr := &customer.CustomerAddress{
		TenantID:    profile.TenantID,
		CustomerID:  profile.ID,
		Label:       req.Label,
		IsDefault:   req.IsDefault,
		Name:        strings.TrimSpace(req.Name),
		Line1:       strings.TrimSpace(req.Line1),
		Line2:       req.Line2,
		City:        strings.TrimSpace(req.City),
		Region:      req.Region,
		PostalCode:  req.PostalCode,
		CountryCode: strings.ToUpper(strings.TrimSpace(req.CountryCode)),
		Phone:       req.Phone,
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if addr.IsDefault {
			if err := h.repo.ClearDefaultAddresses(c.Request.Context(), tx, profile.ID); err != nil {
				return err
			}
		}
		return h.repo.CreateAddress(c.Request.Context(), tx, addr)
	})
	if err != nil {
		h.logger.Error("failed to create address", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to create address",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": mapAddressToResponse(addr)})
}

// UpdateAddress handles PATCH /storefront/stores/:storeSlug/account/addresses/:id.
func (h *CustomerAccountHandler) UpdateAddress(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	addrID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_id",
			"message": "Invalid address ID",
		})
		return
	}

	// Verify ownership.
	if _, err := h.repo.GetAddress(c.Request.Context(), addrID, profile.ID); err != nil {
		if errors.Is(err, customer.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Address not found",
			})
			return
		}
		h.logger.Error("failed to get address", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "internal error",
		})
		return
	}

	var req customer.UpdateAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	updates := make(map[string]any)
	if req.Label != nil {
		updates["label"] = strings.TrimSpace(*req.Label)
	}
	if req.Name != nil {
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Line1 != nil {
		updates["line1"] = strings.TrimSpace(*req.Line1)
	}
	if req.Line2 != nil {
		updates["line2"] = strings.TrimSpace(*req.Line2)
	}
	if req.City != nil {
		updates["city"] = strings.TrimSpace(*req.City)
	}
	if req.Region != nil {
		updates["region"] = strings.TrimSpace(*req.Region)
	}
	if req.PostalCode != nil {
		updates["postal_code"] = strings.TrimSpace(*req.PostalCode)
	}
	if req.CountryCode != nil {
		updates["country_code"] = strings.ToUpper(strings.TrimSpace(*req.CountryCode))
	}
	if req.Phone != nil {
		updates["phone"] = strings.TrimSpace(*req.Phone)
	}

	var updated *customer.CustomerAddress
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if req.IsDefault != nil && *req.IsDefault {
			if clearErr := h.repo.ClearDefaultAddresses(c.Request.Context(), tx, profile.ID); clearErr != nil {
				return clearErr
			}
			updates["is_default"] = true
		} else if req.IsDefault != nil {
			updates["is_default"] = false
		}
		var updateErr error
		updated, updateErr = h.repo.UpdateAddress(c.Request.Context(), tx, addrID, updates)
		return updateErr
	})
	if err != nil {
		h.logger.Error("failed to update address", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to update address",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mapAddressToResponse(updated)})
}

// DeleteAddress handles DELETE /storefront/stores/:storeSlug/account/addresses/:id.
func (h *CustomerAccountHandler) DeleteAddress(c *gin.Context) {
	profile := h.mustGetProfile(c)
	if profile == nil {
		return
	}

	addrID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_id",
			"message": "Invalid address ID",
		})
		return
	}

	if err := h.repo.DeleteAddress(c.Request.Context(), addrID, profile.ID); err != nil {
		if errors.Is(err, customer.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Address not found",
			})
			return
		}
		h.logger.Error("failed to delete address", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to delete address",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Address deleted"})
}

// --- helpers ---

func (h *CustomerAccountHandler) mustGetProfile(c *gin.Context) *customer.CustomerProfile {
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

func mapProfileToResponse(p *customer.CustomerProfile) customer.StorefrontProfileResponse {
	resp := customer.StorefrontProfileResponse{
		ID:             p.ID.String(),
		Email:          p.Email,
		MarketingOptIn: p.MarketingOptIn,
		CreatedAt:      p.CreatedAt.Format(time.RFC3339),
	}
	if p.FirstName != nil {
		resp.FirstName = *p.FirstName
	}
	if p.LastName != nil {
		resp.LastName = *p.LastName
	}
	if p.Phone != nil {
		resp.Phone = *p.Phone
	}
	if p.AvatarURL != nil {
		resp.AvatarURL = *p.AvatarURL
	}
	return resp
}

func mapAddressToResponse(a *customer.CustomerAddress) customer.AddressResponse {
	resp := customer.AddressResponse{
		ID:          a.ID.String(),
		IsDefault:   a.IsDefault,
		Name:        a.Name,
		Line1:       a.Line1,
		City:        a.City,
		CountryCode: a.CountryCode,
	}
	if a.Label != nil {
		resp.Label = *a.Label
	}
	if a.Line2 != nil {
		resp.Line2 = *a.Line2
	}
	if a.Region != nil {
		resp.Region = *a.Region
	}
	if a.PostalCode != nil {
		resp.PostalCode = *a.PostalCode
	}
	if a.Phone != nil {
		resp.Phone = *a.Phone
	}
	return resp
}
