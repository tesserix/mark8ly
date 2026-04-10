package admin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/review"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ReviewsHandler bundles dependencies for the admin review endpoints.
type ReviewsHandler struct {
	repo   review.Repository
	logger *slog.Logger
}

// NewReviewsHandler constructs a ReviewsHandler.
func NewReviewsHandler(repo review.Repository, logger *slog.Logger) *ReviewsHandler {
	return &ReviewsHandler{repo: repo, logger: logger}
}

// List handles GET /admin/stores/:storeId/reviews.
func (h *ReviewsHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")

	var q AdminReviewListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		RespondErr(c, apperrors.ValidationFailed("query", err.Error()), h.logger)
		return
	}

	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		q.PageSize = 20
	}

	rows, total, err := h.repo.ListByStore(c.Request.Context(), storeID, q.Status, q.Page, q.PageSize)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	totalPages := int64(0)
	if q.PageSize > 0 {
		totalPages = (total + int64(q.PageSize) - 1) / int64(q.PageSize)
	}

	out := make([]AdminReviewResponse, 0, len(rows))
	for i := range rows {
		out = append(out, toAdminReviewResponse(&rows[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page":        q.Page,
			"page_size":   q.PageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// Get handles GET /admin/stores/:storeId/reviews/:id.
func (h *ReviewsHandler) Get(c *gin.Context) {
	reviewID := c.Param("id")

	rev, err := h.repo.GetByID(c.Request.Context(), reviewID)
	if err != nil {
		if errors.Is(err, review.ErrNotFound) {
			RespondErr(c, apperrors.NotFound("review"), h.logger)
			return
		}
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toAdminReviewResponse(rev))
}

// Approve handles POST /admin/stores/:storeId/reviews/:id/approve.
func (h *ReviewsHandler) Approve(c *gin.Context) {
	reviewID := c.Param("id")

	// Verify review exists.
	existing, err := h.repo.GetByID(c.Request.Context(), reviewID)
	if err != nil {
		if errors.Is(err, review.ErrNotFound) {
			RespondErr(c, apperrors.NotFound("review"), h.logger)
			return
		}
		RespondErr(c, err, h.logger)
		return
	}

	if existing.Status == review.StatusApproved {
		c.JSON(http.StatusOK, toAdminReviewResponse(existing))
		return
	}

	updated, err := h.repo.UpdateStatus(c.Request.Context(), reviewID, review.StatusApproved)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toAdminReviewResponse(updated))
}

// Reject handles POST /admin/stores/:storeId/reviews/:id/reject.
func (h *ReviewsHandler) Reject(c *gin.Context) {
	reviewID := c.Param("id")

	// Verify review exists.
	existing, err := h.repo.GetByID(c.Request.Context(), reviewID)
	if err != nil {
		if errors.Is(err, review.ErrNotFound) {
			RespondErr(c, apperrors.NotFound("review"), h.logger)
			return
		}
		RespondErr(c, err, h.logger)
		return
	}

	if existing.Status == review.StatusRejected {
		c.JSON(http.StatusOK, toAdminReviewResponse(existing))
		return
	}

	updated, err := h.repo.UpdateStatus(c.Request.Context(), reviewID, review.StatusRejected)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toAdminReviewResponse(updated))
}

// ToggleFeatured handles POST /admin/stores/:storeId/reviews/:id/featured.
func (h *ReviewsHandler) ToggleFeatured(c *gin.Context) {
	reviewID := c.Param("id")

	// Verify review exists.
	if _, err := h.repo.GetByID(c.Request.Context(), reviewID); err != nil {
		if errors.Is(err, review.ErrNotFound) {
			RespondErr(c, apperrors.NotFound("review"), h.logger)
			return
		}
		RespondErr(c, err, h.logger)
		return
	}

	var req SetFeaturedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	updated, err := h.repo.SetFeatured(c.Request.Context(), reviewID, req.Featured)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toAdminReviewResponse(updated))
}

// Reply handles POST /admin/stores/:storeId/reviews/:id/reply.
func (h *ReviewsHandler) Reply(c *gin.Context) {
	reviewID := c.Param("id")

	// Verify review exists.
	if _, err := h.repo.GetByID(c.Request.Context(), reviewID); err != nil {
		if errors.Is(err, review.ErrNotFound) {
			RespondErr(c, apperrors.NotFound("review"), h.logger)
			return
		}
		RespondErr(c, err, h.logger)
		return
	}

	var req ReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	// Use the authenticated admin's identity from the gin context.
	userEmail := c.GetString("user_email")
	userName := c.GetString("user_name")
	if userName == "" {
		userName = "Store Owner"
	}

	reply := &review.ReviewReply{
		ReviewID:    reviewID,
		AuthorType:  "merchant",
		AuthorName:  userName,
		AuthorEmail: nilIfEmpty(userEmail),
		Content:     req.Content,
	}

	if err := h.repo.AddReply(c.Request.Context(), reply); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	// Re-fetch to return the updated review with the new reply.
	updated, err := h.repo.GetByID(c.Request.Context(), reviewID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toAdminReviewResponse(updated))
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
