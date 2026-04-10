package admin

import (
	"time"

	"github.com/mark8ly/marketplace-api/internal/customer"
)

// AdminCustomerListQuery is the parsed query string for GET /admin/stores/:storeId/customers.
type AdminCustomerListQuery struct {
	Search   string `form:"search"`
	Status   string `form:"status"`
	Tag      string `form:"tag"`
	SortBy   string `form:"sort_by"`
	SortDir  string `form:"sort_dir"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// AdminCustomerResponse is the wire shape for a customer in the admin list.
type AdminCustomerResponse struct {
	ID             string   `json:"id"`
	Email          string   `json:"email"`
	FirstName      string   `json:"first_name,omitempty"`
	LastName       string   `json:"last_name,omitempty"`
	Phone          string   `json:"phone,omitempty"`
	AvatarURL      string   `json:"avatar_url,omitempty"`
	Tags           []string `json:"tags"`
	Status         string   `json:"status"`
	BlockReason    string   `json:"block_reason,omitempty"`
	Notes          string   `json:"notes,omitempty"`
	MarketingOptIn bool     `json:"marketing_opt_in"`
	OrderCount     int64    `json:"order_count"`
	TotalSpent     float64  `json:"total_spent"`
	LastOrderAt    string   `json:"last_order_at,omitempty"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// AdminCustomerDetailResponse is the wire shape for the customer detail endpoint.
type AdminCustomerDetailResponse struct {
	AdminCustomerResponse
	Addresses []AdminCustomerAddressResponse `json:"addresses"`
}

// AdminCustomerAddressResponse is the wire shape for a customer address.
type AdminCustomerAddressResponse struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	IsDefault   bool   `json:"is_default"`
	Name        string `json:"name"`
	Line1       string `json:"line1"`
	Line2       string `json:"line2,omitempty"`
	City        string `json:"city"`
	Region      string `json:"region,omitempty"`
	PostalCode  string `json:"postal_code,omitempty"`
	CountryCode string `json:"country_code"`
	Phone       string `json:"phone,omitempty"`
}

// UpdateTagsRequest is the wire body for PATCH /customers/:id/tags.
type UpdateTagsRequest struct {
	Tags []string `json:"tags" binding:"required"`
}

// UpdateNotesRequest is the wire body for PATCH /customers/:id/notes.
type UpdateNotesRequest struct {
	Notes string `json:"notes"`
}

// BlockCustomerRequest is the wire body for POST /customers/:id/block.
type BlockCustomerRequest struct {
	Reason string `json:"reason" binding:"required"`
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func toAdminCustomerResponse(c *customer.CustomerWithStats) AdminCustomerResponse {
	tags := make([]string, len(c.Tags))
	copy(tags, c.Tags)
	return AdminCustomerResponse{
		ID:             c.ID.String(),
		Email:          c.Email,
		FirstName:      derefStr(c.FirstName),
		LastName:       derefStr(c.LastName),
		Phone:          derefStr(c.Phone),
		AvatarURL:      derefStr(c.AvatarURL),
		Tags:           tags,
		Status:         c.Status,
		BlockReason:    derefStr(c.BlockReason),
		Notes:          derefStr(c.Notes),
		MarketingOptIn: c.MarketingOptIn,
		OrderCount:     c.OrderCount,
		TotalSpent:     c.TotalSpent,
		LastOrderAt:    formatTimePtr(c.LastOrderAt),
		CreatedAt:      formatTime(c.CreatedAt),
		UpdatedAt:      formatTime(c.UpdatedAt),
	}
}

func toAdminCustomerProfileResponse(p *customer.CustomerProfile) AdminCustomerResponse {
	tags := make([]string, len(p.Tags))
	copy(tags, p.Tags)
	return AdminCustomerResponse{
		ID:             p.ID.String(),
		Email:          p.Email,
		FirstName:      derefStr(p.FirstName),
		LastName:       derefStr(p.LastName),
		Phone:          derefStr(p.Phone),
		AvatarURL:      derefStr(p.AvatarURL),
		Tags:           tags,
		Status:         p.Status,
		BlockReason:    derefStr(p.BlockReason),
		Notes:          derefStr(p.Notes),
		MarketingOptIn: p.MarketingOptIn,
		CreatedAt:      formatTime(p.CreatedAt),
		UpdatedAt:      formatTime(p.UpdatedAt),
	}
}

func toAdminAddressResponse(a *customer.CustomerAddress) AdminCustomerAddressResponse {
	return AdminCustomerAddressResponse{
		ID:          a.ID.String(),
		Label:       derefStr(a.Label),
		IsDefault:   a.IsDefault,
		Name:        a.Name,
		Line1:       a.Line1,
		Line2:       derefStr(a.Line2),
		City:        a.City,
		Region:      derefStr(a.Region),
		PostalCode:  derefStr(a.PostalCode),
		CountryCode: a.CountryCode,
		Phone:       derefStr(a.Phone),
	}
}
