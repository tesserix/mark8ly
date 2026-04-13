package admin

import (
	"time"

	"github.com/mark8ly/marketplace-api/internal/campaign"
)

// --- Request DTOs ---

// CreateCampaignRequest is the JSON body for POST /campaigns.
type CreateCampaignRequest struct {
	Name               string  `json:"name" binding:"required"`
	Type               string  `json:"type"`      // defaults to "email"
	Subject            string  `json:"subject"`
	Content            string  `json:"content"`
	SegmentID          *string `json:"segment_id"`
	CouponID           *string `json:"coupon_id"`
	ShowOnStorefront   bool    `json:"show_on_storefront"`
	StorefrontLabel    *string `json:"storefront_label"`
	StorefrontPriority int     `json:"storefront_priority"`
}

// UpdateCampaignRequest is the JSON body for PATCH /campaigns/:id.
type UpdateCampaignRequest struct {
	Name               *string `json:"name"`
	Subject            *string `json:"subject"`
	Content            *string `json:"content"`
	SegmentID          *string `json:"segment_id"`
	CouponID           *string `json:"coupon_id"`
	ShowOnStorefront   *bool   `json:"show_on_storefront"`
	StorefrontLabel    *string `json:"storefront_label"`
	StorefrontPriority *int    `json:"storefront_priority"`
}

// ScheduleCampaignRequest is the JSON body for POST /campaigns/:id/schedule.
type ScheduleCampaignRequest struct {
	ScheduledAt string `json:"scheduled_at" binding:"required"` // RFC3339
}

// --- Response DTOs ---

// CampaignResponse is the JSON response for a campaign.
type CampaignResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Type            string  `json:"type"`
	Status          string  `json:"status"`
	Subject         *string `json:"subject"`
	Content         *string `json:"content"`
	SegmentID       *string `json:"segment_id"`
	CouponID        *string `json:"coupon_id"`
	ScheduledAt     *string `json:"scheduled_at"`
	SentAt          *string `json:"sent_at"`
	TotalRecipients int     `json:"total_recipients"`
	Delivered       int     `json:"delivered"`
	Opened          int     `json:"opened"`
	Clicked         int     `json:"clicked"`
	Converted       int     `json:"converted"`
	Unsubscribed    int     `json:"unsubscribed"`
	Failed          int     `json:"failed"`
	Revenue            string  `json:"revenue"`
	ShowOnStorefront   bool    `json:"show_on_storefront"`
	StorefrontLabel    *string `json:"storefront_label,omitempty"`
	StorefrontPriority int     `json:"storefront_priority"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

// ToCampaignResponse maps a domain Campaign to its response DTO.
func ToCampaignResponse(c *campaign.Campaign) CampaignResponse {
	resp := CampaignResponse{
		ID:              c.ID.String(),
		Name:            c.Name,
		Type:            c.Type,
		Status:          c.Status,
		Subject:         c.Subject,
		Content:         c.Content,
		TotalRecipients: c.TotalRecipients,
		Delivered:       c.Delivered,
		Opened:          c.Opened,
		Clicked:         c.Clicked,
		Converted:       c.Converted,
		Unsubscribed:    c.Unsubscribed,
		Failed:          c.Failed,
		Revenue:            c.Revenue.StringFixed(2),
		ShowOnStorefront:   c.ShowOnStorefront,
		StorefrontLabel:    c.StorefrontLabel,
		StorefrontPriority: c.StorefrontPriority,
		CreatedAt:          c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          c.UpdatedAt.Format(time.RFC3339),
	}
	if c.SegmentID != nil {
		s := c.SegmentID.String()
		resp.SegmentID = &s
	}
	if c.CouponID != nil {
		s := c.CouponID.String()
		resp.CouponID = &s
	}
	if c.ScheduledAt != nil {
		s := c.ScheduledAt.Format(time.RFC3339)
		resp.ScheduledAt = &s
	}
	if c.SentAt != nil {
		s := c.SentAt.Format(time.RFC3339)
		resp.SentAt = &s
	}
	return resp
}

// ToCampaignListResponse maps a slice of campaigns to response DTOs.
func ToCampaignListResponse(campaigns []campaign.Campaign) []CampaignResponse {
	out := make([]CampaignResponse, len(campaigns))
	for i := range campaigns {
		out[i] = ToCampaignResponse(&campaigns[i])
	}
	return out
}
