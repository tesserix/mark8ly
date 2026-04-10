package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound is returned when a review does not exist.
var ErrNotFound = errors.New("review: not found")

// Repository is the data-access interface for reviews.
type Repository interface {
	Create(ctx context.Context, review *Review) error
	GetByID(ctx context.Context, reviewID string) (*Review, error)
	ListByProduct(ctx context.Context, productID, status string, page, pageSize int) ([]Review, int64, error)
	ListByStore(ctx context.Context, storeID, status string, page, pageSize int) ([]Review, int64, error)
	UpdateStatus(ctx context.Context, reviewID, status string) (*Review, error)
	SetFeatured(ctx context.Context, reviewID string, featured bool) (*Review, error)
	AddReply(ctx context.Context, reply *ReviewReply) error
	AddMedia(ctx context.Context, media *ReviewMedia) error
	AddReaction(ctx context.Context, reaction *ReviewReaction) error
	RemoveReaction(ctx context.Context, reviewID, customerProfileID string) error
	UpdateHelpfulCounts(ctx context.Context, reviewID string) error
	GetAverageRating(ctx context.Context, productID string) (float64, int64, error)
	HasReviewed(ctx context.Context, storeID, productID, customerEmail string) (bool, error)
	ListByCustomer(ctx context.Context, storeID, customerProfileID string, page, pageSize int) ([]Review, int64, error)
}

type gormRepo struct {
	db *gorm.DB
}

// NewRepository constructs a Repository backed by GORM.
func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) Create(ctx context.Context, review *Review) error {
	if err := r.db.WithContext(ctx).Create(review).Error; err != nil {
		return fmt.Errorf("review: create: %w", err)
	}
	return nil
}

func (r *gormRepo) GetByID(ctx context.Context, reviewID string) (*Review, error) {
	var rev Review
	err := r.db.WithContext(ctx).
		Preload("Media", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Where("id = ?", reviewID).
		First(&rev).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("review: get by id: %w", err)
	}
	if rev.Media == nil {
		rev.Media = []ReviewMedia{}
	}
	if rev.Replies == nil {
		rev.Replies = []ReviewReply{}
	}
	return &rev, nil
}

func (r *gormRepo) ListByProduct(ctx context.Context, productID, status string, page, pageSize int) ([]Review, int64, error) {
	page, pageSize = defaultPagination(page, pageSize)

	base := r.db.WithContext(ctx).Model(&Review{}).Where("product_id = ?", productID)
	if status != "" {
		base = base.Where("status = ?", status)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("review: list by product count: %w", err)
	}

	var rows []Review
	err := base.
		Preload("Media", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("review: list by product: %w", err)
	}
	if rows == nil {
		rows = []Review{}
	}
	return rows, total, nil
}

func (r *gormRepo) ListByStore(ctx context.Context, storeID, status string, page, pageSize int) ([]Review, int64, error) {
	page, pageSize = defaultPagination(page, pageSize)

	base := r.db.WithContext(ctx).Model(&Review{}).Where("store_id = ?", storeID)
	if status != "" {
		base = base.Where("status = ?", status)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("review: list by store count: %w", err)
	}

	var rows []Review
	err := base.
		Preload("Media", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("review: list by store: %w", err)
	}
	if rows == nil {
		rows = []Review{}
	}
	return rows, total, nil
}

func (r *gormRepo) UpdateStatus(ctx context.Context, reviewID, status string) (*Review, error) {
	updates := map[string]any{
		"status":     status,
		"updated_at": time.Now(),
	}
	if status == StatusApproved {
		now := time.Now()
		updates["published_at"] = now
	}

	err := r.db.WithContext(ctx).
		Model(&Review{}).
		Where("id = ?", reviewID).
		Updates(updates).Error
	if err != nil {
		return nil, fmt.Errorf("review: update status: %w", err)
	}
	return r.GetByID(ctx, reviewID)
}

func (r *gormRepo) SetFeatured(ctx context.Context, reviewID string, featured bool) (*Review, error) {
	err := r.db.WithContext(ctx).
		Model(&Review{}).
		Where("id = ?", reviewID).
		Updates(map[string]any{
			"featured":   featured,
			"updated_at": time.Now(),
		}).Error
	if err != nil {
		return nil, fmt.Errorf("review: set featured: %w", err)
	}
	return r.GetByID(ctx, reviewID)
}

func (r *gormRepo) AddReply(ctx context.Context, reply *ReviewReply) error {
	if err := r.db.WithContext(ctx).Create(reply).Error; err != nil {
		return fmt.Errorf("review: add reply: %w", err)
	}
	return nil
}

func (r *gormRepo) AddMedia(ctx context.Context, media *ReviewMedia) error {
	if err := r.db.WithContext(ctx).Create(media).Error; err != nil {
		return fmt.Errorf("review: add media: %w", err)
	}
	return nil
}

func (r *gormRepo) AddReaction(ctx context.Context, reaction *ReviewReaction) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "review_id"}, {Name: "customer_profile_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"reaction", "created_at"}),
		}).
		Create(reaction).Error
	if err != nil {
		return fmt.Errorf("review: add reaction: %w", err)
	}
	return nil
}

func (r *gormRepo) RemoveReaction(ctx context.Context, reviewID, customerProfileID string) error {
	result := r.db.WithContext(ctx).
		Where("review_id = ? AND customer_profile_id = ?", reviewID, customerProfileID).
		Delete(&ReviewReaction{})
	if result.Error != nil {
		return fmt.Errorf("review: remove reaction: %w", result.Error)
	}
	return nil
}

func (r *gormRepo) UpdateHelpfulCounts(ctx context.Context, reviewID string) error {
	err := r.db.WithContext(ctx).Exec(`
		UPDATE reviews SET
			helpful_count     = (SELECT COUNT(*) FROM review_reactions WHERE review_id = ? AND reaction = 'helpful'),
			not_helpful_count = (SELECT COUNT(*) FROM review_reactions WHERE review_id = ? AND reaction = 'not_helpful'),
			updated_at = now()
		WHERE id = ?
	`, reviewID, reviewID, reviewID).Error
	if err != nil {
		return fmt.Errorf("review: update helpful counts: %w", err)
	}
	return nil
}

func (r *gormRepo) GetAverageRating(ctx context.Context, productID string) (float64, int64, error) {
	var result struct {
		Avg   float64
		Count int64
	}
	err := r.db.WithContext(ctx).
		Model(&Review{}).
		Select("COALESCE(AVG(rating), 0) as avg, COUNT(*) as count").
		Where("product_id = ? AND status = ?", productID, StatusApproved).
		Scan(&result).Error
	if err != nil {
		return 0, 0, fmt.Errorf("review: get average rating: %w", err)
	}
	return result.Avg, result.Count, nil
}

func (r *gormRepo) HasReviewed(ctx context.Context, storeID, productID, customerEmail string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Review{}).
		Where("store_id = ? AND product_id = ? AND customer_email = ?", storeID, productID, customerEmail).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("review: has reviewed: %w", err)
	}
	return count > 0, nil
}

func (r *gormRepo) ListByCustomer(ctx context.Context, storeID, customerProfileID string, page, pageSize int) ([]Review, int64, error) {
	page, pageSize = defaultPagination(page, pageSize)

	base := r.db.WithContext(ctx).Model(&Review{}).
		Where("store_id = ? AND customer_profile_id = ?", storeID, customerProfileID)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("review: list by customer count: %w", err)
	}

	var rows []Review
	err := base.
		Preload("Media", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("review: list by customer: %w", err)
	}
	if rows == nil {
		rows = []Review{}
	}
	return rows, total, nil
}

// defaultPagination normalises page/pageSize values.
func defaultPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

// sanitizeSortColumn maps user-supplied sort columns to safe SQL identifiers.
func sanitizeSortColumn(col string) string {
	switch col {
	case "rating":
		return "rating"
	case "helpful_count":
		return "helpful_count"
	default:
		return "created_at"
	}
}

// sanitizeSortDir normalises the sort direction to ASC or DESC.
func sanitizeSortDir(dir string) string {
	if strings.EqualFold(dir, "asc") {
		return "ASC"
	}
	return "DESC"
}
