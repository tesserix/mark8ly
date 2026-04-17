package storefront

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/review"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// ReviewsHandler serves the storefront review endpoints.
type ReviewsHandler struct {
	reviewSvc   *review.Service
	reviewRepo  review.Repository
	productRepo product.Repository
	notify      *notification.Service // optional — nil-safe
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

// WithNotifier attaches the notification service so review submissions
// fire in-app notifications. Nil-safe.
func (h *ReviewsHandler) WithNotifier(n *notification.Service) *ReviewsHandler {
	h.notify = n
	return h
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

	// If the viewer is signed in, annotate each review with THEIR
	// active reaction so the UI can highlight the selected button.
	// Anonymous callers get empty viewer_reaction fields.
	var viewerReactions map[string]string
	if profileVal, ok := c.Get(CustomerProfileKey); ok {
		if profile, ok := profileVal.(*customer.CustomerProfile); ok && profile != nil {
			ids := make([]string, 0, len(reviews))
			for i := range reviews {
				ids = append(ids, reviews[i].ID)
			}
			if m, err := h.reviewRepo.ListViewerReactions(c.Request.Context(), profile.ID.String(), ids); err == nil {
				viewerReactions = m
			}
		}
	}

	out := make([]storefrontReviewResponse, 0, len(reviews))
	for i := range reviews {
		resp := toStorefrontReviewResponse(&reviews[i])
		if v, ok := viewerReactions[reviews[i].ID]; ok {
			resp.ViewerReaction = v
		}
		out = append(out, resp)
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

	if tenantUUID, err1 := uuid.Parse(result.TenantID); err1 == nil {
		if storeUUID, err2 := uuid.Parse(result.StoreID); err2 == nil {
			reviewMsg := "A customer submitted a product review."
			reviewResource := "review"
			var reviewID *uuid.UUID
			if rid, err3 := uuid.Parse(result.ID); err3 == nil {
				reviewID = &rid
			}
			notification.Emit(c.Request.Context(), h.notify, h.logger, notification.Notification{
				TenantID:     tenantUUID,
				StoreID:      storeUUID,
				Type:         notification.TypeReviewSubmitted,
				Title:        "New product review",
				Message:      &reviewMsg,
				ResourceType: &reviewResource,
				ResourceID:   reviewID,
			})
		}
	}

	c.JSON(http.StatusCreated, gin.H{"data": toStorefrontReviewResponse(result)})
}

// submitGuestReviewRequest — wire body for the public (unauthenticated)
// review endpoint. Same fields as submitReviewRequest plus explicit
// name + email, since there's no customer session to derive them from.
type submitGuestReviewRequest struct {
	Rating        int    `json:"rating"         binding:"required,min=1,max=5"`
	Title         string `json:"title"          binding:"omitempty,max=300"`
	Content       string `json:"content"        binding:"required,max=5000"`
	CustomerName  string `json:"customer_name"  binding:"required,min=1,max=120"`
	CustomerEmail string `json:"customer_email" binding:"required,email,max=320"`
}

// SubmitGuestReview handles POST /storefront/stores/:storeSlug/products/:handle/reviews-guest.
// Anonymous (unauthenticated) customers can submit a review by
// supplying name + email in the body. Same moderation gate as
// authenticated submissions (status = pending → admin approves), and
// the same UNIQUE(store, product, email) invariant prevents
// double-submits.
func (h *ReviewsHandler) SubmitGuestReview(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	handle := c.Param("handle")
	if handle == "" {
		respondNotFound(c)
		return
	}

	agg, err := h.productRepo.GetPublishedByHandle(c.Request.Context(), store.ID, handle)
	if err != nil {
		respondNotFound(c)
		return
	}

	var req submitGuestReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	result, err := h.reviewSvc.SubmitReview(c.Request.Context(), review.SubmitReviewInput{
		TenantID:      store.TenantID,
		StoreID:       store.ID,
		ProductID:     agg.Product.ID,
		CustomerName:  strings.TrimSpace(req.CustomerName),
		CustomerEmail: strings.TrimSpace(req.CustomerEmail),
		Rating:        req.Rating,
		Title:         req.Title,
		Content:       req.Content,
	})
	if err != nil {
		if strings.Contains(err.Error(), "already reviewed") {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "duplicate_review",
				"message": "A review from this email already exists for this product.",
			})
			return
		}
		respondInternal(c, h.logger, err)
		return
	}

	if tenantUUID, err1 := uuid.Parse(result.TenantID); err1 == nil {
		if storeUUID, err2 := uuid.Parse(result.StoreID); err2 == nil {
			reviewMsg := "A guest visitor submitted a product review."
			reviewResource := "review"
			var reviewID *uuid.UUID
			if rid, err3 := uuid.Parse(result.ID); err3 == nil {
				reviewID = &rid
			}
			notification.Emit(c.Request.Context(), h.notify, h.logger, notification.Notification{
				TenantID:     tenantUUID,
				StoreID:      storeUUID,
				Type:         notification.TypeReviewSubmitted,
				Title:        "New product review (guest)",
				Message:      &reviewMsg,
				ResourceType: &reviewResource,
				ResourceID:   reviewID,
			})
		}
	}

	c.JSON(http.StatusCreated, gin.H{"data": toStorefrontReviewResponse(result)})
}

// addReactionRequest is the wire body for POST /reviews/:id/reactions.
// The three values mirror the DB CHECK constraint added in migration 39
// (helpful / not_helpful / useful); the UNIQUE(review_id,
// customer_profile_id) invariant on review_reactions means switching
// reaction type is a single UPSERT — the user's old reaction is
// replaced, never stacked.
type addReactionRequest struct {
	Reaction string `json:"reaction" binding:"required,oneof=helpful not_helpful useful"`
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

// addReplyRequest is the wire body for POST /reviews/:id/replies —
// customer comment on a review. parent_reply_id is optional; when set,
// the comment is nested under that reply. Must belong to the same
// review (validated server-side).
type addReplyRequest struct {
	Content       string  `json:"content"         binding:"required,min=1,max=5000"`
	ParentReplyID *string `json:"parent_reply_id"`
}

// AddCustomerReply handles POST /storefront/stores/:storeSlug/reviews/:id/replies.
// Requires customer auth. Only approved reviews accept comments — the
// storefront never surfaces pending/rejected reviews anyway but this
// stops a client that remembered a pending id from writing under it.
func (h *ReviewsHandler) AddCustomerReply(c *gin.Context) {
	reviewID := c.Param("id")
	if reviewID == "" {
		respondNotFound(c)
		return
	}

	profile := mustGetCustomerProfile(c)
	if profile == nil {
		return
	}

	var req addReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	rev, err := h.reviewRepo.GetByID(c.Request.Context(), reviewID)
	if err != nil {
		respondNotFound(c)
		return
	}
	if rev.Status != review.StatusApproved {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "not_commentable",
			"message": "This review isn't open for comments yet.",
		})
		return
	}

	// When parent_reply_id is provided, verify it belongs to THIS
	// review. Otherwise a client could anchor comments onto threads
	// from another review (or another store).
	if req.ParentReplyID != nil && *req.ParentReplyID != "" {
		if _, err := h.reviewRepo.GetReply(c.Request.Context(), *req.ParentReplyID, reviewID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_parent",
				"message": "Reply parent does not belong to this review.",
			})
			return
		}
	}

	reply := &review.ReviewReply{
		ReviewID:      reviewID,
		ParentReplyID: req.ParentReplyID,
		AuthorType:    review.AuthorTypeCustomer,
		AuthorName:    buildCustomerName(profile),
		AuthorEmail:   &profile.Email,
		Content:       strings.TrimSpace(req.Content),
	}
	if err := h.reviewRepo.AddReply(c.Request.Context(), reply); err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": map[string]any{
			"id":              reply.ID,
			"review_id":       reply.ReviewID,
			"parent_reply_id": reply.ParentReplyID,
			"author_type":     reply.AuthorType,
			"author_name":     reply.AuthorName,
			"content":         reply.Content,
			"created_at":      reply.CreatedAt,
		},
	})
}

// addGuestReplyRequest — anonymous comment body. Same fields as the
// authenticated one plus name/email so we can attribute the comment
// without a customer session.
type addGuestReplyRequest struct {
	Content       string  `json:"content"         binding:"required,min=1,max=5000"`
	ParentReplyID *string `json:"parent_reply_id"`
	CustomerName  string  `json:"customer_name"   binding:"required,min=1,max=120"`
	CustomerEmail string  `json:"customer_email"  binding:"required,email,max=320"`
}

// AddGuestReply handles POST /storefront/stores/:storeSlug/reviews/:id/replies-guest.
// Anonymous visitors can add a comment by supplying name + email.
// Same rules as AddCustomerReply: review must be approved, nested
// parent must belong to the review, trimmed content ≤ 5000 chars.
func (h *ReviewsHandler) AddGuestReply(c *gin.Context) {
	reviewID := c.Param("id")
	if reviewID == "" {
		respondNotFound(c)
		return
	}

	var req addGuestReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	rev, err := h.reviewRepo.GetByID(c.Request.Context(), reviewID)
	if err != nil {
		respondNotFound(c)
		return
	}
	if rev.Status != review.StatusApproved {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "not_commentable",
			"message": "This review isn't open for comments yet.",
		})
		return
	}

	if req.ParentReplyID != nil && *req.ParentReplyID != "" {
		if _, err := h.reviewRepo.GetReply(c.Request.Context(), *req.ParentReplyID, reviewID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_parent",
				"message": "Reply parent does not belong to this review.",
			})
			return
		}
	}

	email := strings.TrimSpace(req.CustomerEmail)
	reply := &review.ReviewReply{
		ReviewID:      reviewID,
		ParentReplyID: req.ParentReplyID,
		AuthorType:    review.AuthorTypeCustomer,
		AuthorName:    strings.TrimSpace(req.CustomerName),
		AuthorEmail:   &email,
		Content:       strings.TrimSpace(req.Content),
	}
	if err := h.reviewRepo.AddReply(c.Request.Context(), reply); err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": map[string]any{
			"id":              reply.ID,
			"review_id":       reply.ReviewID,
			"parent_reply_id": reply.ParentReplyID,
			"author_type":     reply.AuthorType,
			"author_name":     reply.AuthorName,
			"content":         reply.Content,
			"created_at":      reply.CreatedAt,
		},
	})
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
	UsefulCount      int                           `json:"useful_count"`
	// ViewerReaction: the reaction the caller themselves picked on this
	// review ("" = none). Lets the UI highlight the active button
	// without a separate round-trip. Only populated when we have a
	// customer profile in context (authenticated path).
	ViewerReaction   string                        `json:"viewer_reaction,omitempty"`
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
	ID            string  `json:"id"`
	ParentReplyID *string `json:"parent_reply_id,omitempty"`
	AuthorType    string  `json:"author_type"`
	AuthorName    string  `json:"author_name"`
	Content       string  `json:"content"`
	CreatedAt     string  `json:"created_at"`
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
		UsefulCount:      r.UsefulCount,
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
			ID:            rp.ID,
			ParentReplyID: rp.ParentReplyID,
			AuthorType:    rp.AuthorType,
			// PII guard — never leak real staff names to the public
			// storefront. Admins have their real name preserved in the
			// DB (and the admin-side DTO shows it), but shoppers see a
			// generic "Store Team" label.
			AuthorName: publicReplyName(rp.AuthorType, rp.AuthorName),
			Content:    rp.Content,
			CreatedAt:  rp.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	return resp
}

// publicReplyName returns a safe author label for the storefront DTO.
// Merchant replies always render as "Store Team" — staff PII never
// leaves the admin boundary. Customer replies keep whatever the
// customer supplied (their own name is not PII to themselves).
func publicReplyName(authorType, authorName string) string {
	if authorType == "merchant" {
		return "Store Team"
	}
	return authorName
}
