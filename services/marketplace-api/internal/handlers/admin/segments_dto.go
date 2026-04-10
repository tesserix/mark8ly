package admin

import (
	"time"

	"github.com/mark8ly/marketplace-api/internal/campaign"
)

// CreateSegmentRequest is the JSON body for POST /segments.
type CreateSegmentRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Rules       string `json:"rules" binding:"required"` // JSON array string
}

// SegmentResponse is the JSON response for a customer segment.
type SegmentResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Rules       string  `json:"rules"` // JSON array string
	MemberCount int     `json:"member_count"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// ToSegmentResponse maps a domain CustomerSegment to its response DTO.
func ToSegmentResponse(s *campaign.CustomerSegment) SegmentResponse {
	return SegmentResponse{
		ID:          s.ID.String(),
		Name:        s.Name,
		Description: s.Description,
		Rules:       string(s.Rules),
		MemberCount: s.MemberCount,
		CreatedAt:   s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   s.UpdatedAt.Format(time.RFC3339),
	}
}

// ToSegmentListResponse maps a slice of segments to response DTOs.
func ToSegmentListResponse(segments []campaign.CustomerSegment) []SegmentResponse {
	out := make([]SegmentResponse, len(segments))
	for i := range segments {
		out[i] = ToSegmentResponse(&segments[i])
	}
	return out
}
