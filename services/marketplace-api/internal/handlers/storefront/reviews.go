package storefront

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/review"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// ReviewsHandler serves the storefront review endpoints.
type ReviewsHandler struct {
	reviewSvc   *review.Service
	reviewRepo  review.Repository
	productRepo product.Repository
	logger      *slog.Logger
}

// NewReviewsHandler constructs a ReviewsHandler.
func NewReviewsHandler(
	reviewSvc *review.Service,
	reviewRepo review.Repository,
	productRepo product.Repository,
	logger *slog.Logger,
) *ReviewsHandler {
	return &ReviewsHandler{
		reviewSvc:   reviewSvc,
		reviewRepo:  reviewRepo,
		productRepo: productRepo,
		logger:      logger,
	}
}

// listReviewsQuery is the pagination input for listing reviews.
type listReviewsQuery struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}

func (q *listReviewsQuery) defaults() {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
}

// ListProductReviews handles GET /storefront/stores/:storeSlug/products/:handle/reviews.
// Public endpoint — returns only approved reviews.
func (h *ReviewsHandler) ListProductReviews(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	handle := c.Param("handle")
	if handle == "" {
		respondNotFound(c)
		return
	}

	// Resolve product by handle.
	agg, err := h.productRepo.GetPublishedByHandle(c.Request.Context(), store.ID, handle)
	if err != nil {
		respondNotFound(c)
		return
	}

	var q listReviewsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		respondNotFound(c)
		return
	}
	q.defaults()

	reviews, total, avgRating, err := h.reviewSvc.GetProductReviews(
		c.Request.Context(), agg.Product.ID, q.Page, q.PageSize,
	)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	out := make([]storefrontReviewResponse, 0, len(reviews))
	for i := range reviews {
		out = append(out, toStorefrontReviewResponse(&reviews[i]))
	}

	totalPages := int64(0)
	if q.PageSize > 0 {
		totalPages = (total + int64(q.PageSize) - 1) / int64(q.PageSize)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page":           q.Page,
			"page_size":      q.PageSize,
			"total":          total,
			"total_pages":    totalPages,
			"average_rating": avgRating,
		},
	})
}

// submitReviewRequest is the wire body for POST /products/:handle/reviews.
type submitReviewRequest struct {
	Rating  int    `json:"rating" binding:"required,min=1,max=5"`
	Title   string `json:"title" binding:"omitempty,max=300"`
	Content string `json:"content" binding:"required,max=5000"`
}

// SubmitReview handles POST /storefront/stores/:storeSlug/products/:handle/reviews.
// Requires customer auth.
func (h *ReviewsHandler) SubmitReview(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	handle := c.Param("handle")
	if handle == "" {
		respondNotFound(c)
		return
	}

	// Require customer auth.
	profile := mustGetCustomerProfile(c)
	if profile == nil {
		return
	}

	// Resolve product by handle.
	agg, err := h.productRepo.GetPublishedByHandle(c.Request.Context(), store.ID, handle)
	if err != nil {
		respondNotFound(c)
		return
	}

	var req submitReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	// Build customer name from profile.
	customerName := buildCustomerName(profile)

	result, err := h.reviewSvc.SubmitReview(c.Request.Context(), review.SubmitReviewInput{
		TenantID:          store.TenantID,
		StoreID:           store.ID,
		ProductID:         agg.Product.ID,
		CustomerProfileID: profile.ID.String(),
		CustomerName:      customerName,
		CustomerEmail:     profile.Email,
		Rating:            req.Rating,
		Title:             req.Title,
		Content:           req.Content,
	})
	if err != nil {
		if strings.Contains(err.Error(), "already reviewed") {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "duplicate_review",
				"message": "You have already reviewed this product",
			})
			return
		}
		respondInternal(c, h.logger, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": toStorefrontReviewResponse(result)})
}

// addReactionRequest is the wire body for POST /reviews/:id/reactions.
type addReactionRequest struct {
	Reaction string `json:"reaction" binding:"required,oneof=helpful not_helpful"`
}

// AddReaction handles POST /storefront/stores/:storeSlug/reviews/:id/reactions.
// Requires customer auth.
func (h *ReviewsHandler) AddReaction(c *gin.Context) {
	reviewID := c.Param("id")
	if reviewID == "" {
		respondNotFound(c)
		return
	}

	profile := mustGetCustomerProfile(c)
	if profile == nil {
		return
	}

	var req addReactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	// Verify review exists.
	if _, err := h.reviewRepo.GetByID(c.Request.Context(), reviewID); err != nil {
		respondNotFound(c)
		return
	}

	reaction := &review.ReviewReaction{
		ReviewID:          reviewID,
		CustomerProfileID: profile.ID.String(),
		Reaction:          req.Reaction,
	}

	if err := h.reviewRepo.AddReaction(c.Request.Context(), reaction); err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	// Recalculate helpful counts.
	if err := h.reviewRepo.UpdateHelpfulCounts(c.Request.Context(), reviewID); err != nil {
		h.logger.Error("failed to update helpful counts", "error", err, "review_id", reviewID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Reaction recorded"})
}

// --- helpers ---

func mustGetCustomerProfile(c *gin.Context) *customer.CustomerProfile {
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

func buildCustomerName(p *customer.CustomerProfile) string {
	parts := make([]string, 0, 2)
	if p.FirstName != nil && *p.FirstName != "" {
		parts = append(parts, *p.FirstName)
	}
	if p.LastName != nil && *p.LastName != "" {
		parts = append(parts, *p.LastName)
	}
	if len(parts) == 0 {
		// Fallback to email prefix.
		email := p.Email
		if idx := strings.Index(email, "@"); idx > 0 {
			return email[:idx]
		}
		return "Customer"
	}
	return strings.Join(parts, " ")
}

// --- DTOs ---

type storefrontReviewResponse struct {
	ID               string                        `json:"id"`
	Rating           int                           `json:"rating"`
	Title            string                        `json:"title,omitempty"`
	Content          string                        `json:"content"`
	CustomerName     string                        `json:"customer_name"`
	VerifiedPurchase bool                          `json:"verified_purchase"`
	Featured         bool                          `json:"featured"`
	HelpfulCount     int                           `json:"helpful_count"`
	NotHelpfulCount  int                           `json:"not_helpful_count"`
	PublishedAt      string                        `json:"published_at,omitempty"`
	CreatedAt        string                        `json:"created_at"`
	Media            []storefrontReviewMediaDTO    `json:"media"`
	Replies          []storefrontReviewReplyDTO    `json:"replies"`
}

type storefrontReviewMediaDTO struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Alt       string `json:"alt,omitempty"`
	Position  int    `json:"position"`
	MediaType string `json:"media_type"`
}

type storefrontReviewReplyDTO struct {
	AuthorType string `json:"author_type"`
	AuthorName string `json:"author_name"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

func toStorefrontReviewResponse(r *review.Review) storefrontReviewResponse {
	resp := storefrontReviewResponse{
		ID:               r.ID,
		Rating:           r.Rating,
		Content:          r.Content,
		CustomerName:     r.CustomerName,
		VerifiedPurchase: r.VerifiedPurchase,
		Featured:         r.Featured,
		HelpfulCount:     r.HelpfulCount,
		NotHelpfulCount:  r.NotHelpfulCount,
		CreatedAt:        r.CreatedAt.UTC().Format(time.RFC3339),
	}
	if r.Title != nil {
		resp.Title = *r.Title
	}
	if r.PublishedAt != nil {
		resp.PublishedAt = r.PublishedAt.UTC().Format(time.RFC3339)
	}

	resp.Media = make([]storefrontReviewMediaDTO, 0, len(r.Media))
	for _, m := range r.Media {
		alt := ""
		if m.Alt != nil {
			alt = *m.Alt
		}
		resp.Media = append(resp.Media, storefrontReviewMediaDTO{
			ID:        m.ID,
			URL:       m.URL,
			Alt:       alt,
			Position:  m.Position,
			MediaType: m.MediaType,
		})
	}

	resp.Replies = make([]storefrontReviewReplyDTO, 0, len(r.Replies))
	for _, rp := range r.Replies {
		resp.Replies = append(resp.Replies, storefrontReviewReplyDTO{
			AuthorType: rp.AuthorType,
			AuthorName: rp.AuthorName,
			Content:    rp.Content,
			CreatedAt:  rp.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	return resp
}
