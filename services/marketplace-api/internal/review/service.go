package review

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"
)

// Service provides review business logic.
type Service struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewService constructs a Service.
func NewService(db *gorm.DB, repo Repository, logger *slog.Logger) *Service {
	return &Service{db: db, repo: repo, logger: logger}
}

// SubmitReview creates a new review after validating no duplicate exists.
func (s *Service) SubmitReview(ctx context.Context, input SubmitReviewInput) (*Review, error) {
	email := strings.TrimSpace(strings.ToLower(input.CustomerEmail))
	if email == "" {
		return nil, fmt.Errorf("review: customer email is required")
	}

	// Check for duplicate review.
	exists, err := s.repo.HasReviewed(ctx, input.StoreID, input.ProductID, email)
	if err != nil {
		return nil, fmt.Errorf("review: duplicate check: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("review: you have already reviewed this product")
	}

	// Sanitize content — strip leading/trailing whitespace.
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, fmt.Errorf("review: content is required")
	}

	var title *string
	if t := strings.TrimSpace(input.Title); t != "" {
		title = &t
	}

	var customerProfileID *string
	if input.CustomerProfileID != "" {
		customerProfileID = &input.CustomerProfileID
	}

	rev := &Review{
		TenantID:          input.TenantID,
		StoreID:           input.StoreID,
		ProductID:         input.ProductID,
		CustomerProfileID: customerProfileID,
		CustomerName:      strings.TrimSpace(input.CustomerName),
		CustomerEmail:     email,
		Rating:            input.Rating,
		Title:             title,
		Content:           content,
		Status:            StatusPending,
		VerifiedPurchase:  false, // stub — verified purchase check not implemented yet
		Media:             []ReviewMedia{},
		Replies:           []ReviewReply{},
	}

	if err := s.repo.Create(ctx, rev); err != nil {
		return nil, fmt.Errorf("review: submit: %w", err)
	}

	s.logger.Info("review submitted",
		"store_id", input.StoreID,
		"product_id", input.ProductID,
		"review_id", rev.ID,
		"email", email,
	)

	return rev, nil
}

// GetProductReviews returns approved reviews for a product along with the
// average rating and total count.
func (s *Service) GetProductReviews(ctx context.Context, productID string, page, pageSize int) ([]Review, int64, float64, error) {
	reviews, total, err := s.repo.ListByProduct(ctx, productID, StatusApproved, page, pageSize)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("review: get product reviews: %w", err)
	}

	avgRating, _, err := s.repo.GetAverageRating(ctx, productID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("review: get average rating: %w", err)
	}

	return reviews, total, avgRating, nil
}
