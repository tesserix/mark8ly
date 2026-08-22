package admin

import (
	"time"

	"github.com/mark8ly/marketplace-api/internal/review"
)

// AdminReviewListQuery is the parsed query string for GET /admin/stores/:storeId/reviews.
type AdminReviewListQuery struct {
	Status   string `form:"status"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// AdminReviewResponse is the wire shape for a review in the admin list.
type AdminReviewResponse struct {
	ID                string                `json:"id"`
	ProductID         string                `json:"product_id"`
	CustomerProfileID string                `json:"customer_profile_id,omitempty"`
	CustomerName      string                `json:"customer_name"`
	CustomerEmail     string                `json:"customer_email"`
	Rating            int                   `json:"rating"`
	Title             string                `json:"title,omitempty"`
	Content           string                `json:"content"`
	Status            string                `json:"status"`
	VerifiedPurchase  bool                  `json:"verified_purchase"`
	Featured          bool                  `json:"featured"`
	HelpfulCount      int                   `json:"helpful_count"`
	NotHelpfulCount   int                   `json:"not_helpful_count"`
	PublishedAt       string                `json:"published_at,omitempty"`
	CreatedAt         string                `json:"created_at"`
	UpdatedAt         string                `json:"updated_at"`
	Media             []AdminReviewMediaDTO `json:"media"`
	Replies           []AdminReviewReplyDTO `json:"replies"`
}

// AdminReviewMediaDTO is the wire shape for review media.
type AdminReviewMediaDTO struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Alt       string `json:"alt,omitempty"`
	Position  int    `json:"position"`
	MediaType string `json:"media_type"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	FileSize  int    `json:"file_size,omitempty"`
}

// AdminReviewReplyDTO is the wire shape for a review reply.
type AdminReviewReplyDTO struct {
	ID          string `json:"id"`
	AuthorType  string `json:"author_type"`
	AuthorName  string `json:"author_name"`
	AuthorEmail string `json:"author_email,omitempty"`
	Content     string `json:"content"`
	CreatedAt   string `json:"created_at"`
}

// ReplyRequest is the wire body for POST /reviews/:id/reply.
type ReplyRequest struct {
	Content string `json:"content" binding:"required,max=5000"`
}

// SetFeaturedRequest is the wire body for POST /reviews/:id/featured.
type SetFeaturedRequest struct {
	Featured bool `json:"featured"`
}

func toAdminReviewResponse(r *review.Review) AdminReviewResponse {
	resp := AdminReviewResponse{
		ID:               r.ID,
		ProductID:        r.ProductID,
		CustomerName:     r.CustomerName,
		CustomerEmail:    r.CustomerEmail,
		Rating:           r.Rating,
		Content:          r.Content,
		Status:           r.Status,
		VerifiedPurchase: r.VerifiedPurchase,
		Featured:         r.Featured,
		HelpfulCount:     r.HelpfulCount,
		NotHelpfulCount:  r.NotHelpfulCount,
		CreatedAt:        r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        r.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if r.CustomerProfileID != nil {
		resp.CustomerProfileID = *r.CustomerProfileID
	}
	if r.Title != nil {
		resp.Title = *r.Title
	}
	if r.PublishedAt != nil {
		resp.PublishedAt = r.PublishedAt.UTC().Format(time.RFC3339)
	}

	resp.Media = make([]AdminReviewMediaDTO, 0, len(r.Media))
	for _, m := range r.Media {
		dto := AdminReviewMediaDTO{
			ID:        m.ID,
			URL:       m.URL,
			Position:  m.Position,
			MediaType: m.MediaType,
		}
		if m.Alt != nil {
			dto.Alt = *m.Alt
		}
		if m.Width != nil {
			dto.Width = *m.Width
		}
		if m.Height != nil {
			dto.Height = *m.Height
		}
		if m.FileSize != nil {
			dto.FileSize = *m.FileSize
		}
		resp.Media = append(resp.Media, dto)
	}

	resp.Replies = make([]AdminReviewReplyDTO, 0, len(r.Replies))
	for _, rp := range r.Replies {
		dto := AdminReviewReplyDTO{
			ID:         rp.ID,
			AuthorType: rp.AuthorType,
			AuthorName: rp.AuthorName,
			Content:    rp.Content,
			CreatedAt:  rp.CreatedAt.UTC().Format(time.RFC3339),
		}
		if rp.AuthorEmail != nil {
			dto.AuthorEmail = *rp.AuthorEmail
		}
		resp.Replies = append(resp.Replies, dto)
	}

	return resp
}
