package admin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CustomersHandler bundles dependencies for the admin customer endpoints.
type CustomersHandler struct {
	repo   customer.Repository
	logger *slog.Logger
}

// NewCustomersHandler constructs a CustomersHandler.
func NewCustomersHandler(repo customer.Repository, logger *slog.Logger) *CustomersHandler {
	return &CustomersHandler{repo: repo, logger: logger}
}

// List handles GET /admin/stores/:storeId/customers.
func (h *CustomersHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	var q AdminCustomerListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		RespondErr(c, apperrors.ValidationFailed("query", err.Error()), h.logger)
		return
	}

	listQ := customer.ListCustomersQuery{
		Search:   q.Search,
		Status:   q.Status,
		Tag:      q.Tag,
		SortBy:   q.SortBy,
		SortDir:  q.SortDir,
		Page:     q.Page,
		PageSize: q.PageSize,
	}

	rows, total, err := h.repo.ListForStore(c.Request.Context(), storeID, tenantID, listQ)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	listQ.Defaults()
	totalPages := int64(0)
	if listQ.PageSize > 0 {
		totalPages = (total + int64(listQ.PageSize) - 1) / int64(listQ.PageSize)
	}

	out := make([]AdminCustomerResponse, 0, len(rows))
	for i := range rows {
		out = append(out, toAdminCustomerResponse(&rows[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page":        listQ.Page,
			"page_size":   listQ.PageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// Get handles GET /admin/stores/:storeId/customers/:id.
func (h *CustomersHandler) Get(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	customerID := c.Param("id")

	profile, err := h.repo.GetByIDForAdmin(c.Request.Context(), storeID, tenantID, customerID)
	if err != nil {
		if errors.Is(err, customer.ErrNotFound) {
			RespondErr(c, apperrors.NotFound("customer"), h.logger)
			return
		}
		RespondErr(c, err, h.logger)
		return
	}

	addresses, err := h.repo.ListAddressesByCustomer(c.Request.Context(), customerID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	addrOut := make([]AdminCustomerAddressResponse, 0, len(addresses))
	for i := range addresses {
		addrOut = append(addrOut, toAdminAddressResponse(&addresses[i]))
	}

	resp := AdminCustomerDetailResponse{
		AdminCustomerResponse: toAdminCustomerProfileResponse(profile),
		Addresses:             addrOut,
	}
	c.JSON(http.StatusOK, resp)
}

// UpdateTags handles PATCH /admin/stores/:storeId/customers/:id/tags.
func (h *CustomersHandler) UpdateTags(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	customerID := c.Param("id")

	var req UpdateTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	profile, err := h.repo.UpdateTags(c.Request.Context(), storeID, tenantID, customerID, req.Tags)
	if err != nil {
		if errors.Is(err, customer.ErrNotFound) {
			RespondErr(c, apperrors.NotFound("customer"), h.logger)
			return
		}
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toAdminCustomerProfileResponse(profile))
}

// UpdateNotes handles PATCH /admin/stores/:storeId/customers/:id/notes.
func (h *CustomersHandler) UpdateNotes(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	customerID := c.Param("id")

	var req UpdateNotesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	profile, err := h.repo.UpdateNotes(c.Request.Context(), storeID, tenantID, customerID, req.Notes)
	if err != nil {
		if errors.Is(err, customer.ErrNotFound) {
			RespondErr(c, apperrors.NotFound("customer"), h.logger)
			return
		}
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toAdminCustomerProfileResponse(profile))
}

// Block handles POST /admin/stores/:storeId/customers/:id/block.
func (h *CustomersHandler) Block(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	customerID := c.Param("id")

	var req BlockCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	profile, err := h.repo.SetStatus(c.Request.Context(), storeID, tenantID, customerID, customer.StatusBlocked, req.Reason)
	if err != nil {
		if errors.Is(err, customer.ErrNotFound) {
			RespondErr(c, apperrors.NotFound("customer"), h.logger)
			return
		}
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toAdminCustomerProfileResponse(profile))
}

// Unblock handles POST /admin/stores/:storeId/customers/:id/unblock.
func (h *CustomersHandler) Unblock(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	customerID := c.Param("id")

	profile, err := h.repo.SetStatus(c.Request.Context(), storeID, tenantID, customerID, customer.StatusActive, "")
	if err != nil {
		if errors.Is(err, customer.ErrNotFound) {
			RespondErr(c, apperrors.NotFound("customer"), h.logger)
			return
		}
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toAdminCustomerProfileResponse(profile))
}
