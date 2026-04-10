# Customers C3 — Reviews Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship product reviews with 1-5 star rating, text + photos (max 3), approval workflow, verified purchase detection, helpful reactions, featured flag, merchant replies. Admin moderation page + storefront display and submission.

**Architecture:** New `internal/review/` package (models, repository, service). Migration 000014. Photo upload via existing GCS uploader. Star ratings use moss-700 filled / ink-900 opacity-15 empty. One review per product per customer (UNIQUE constraint). Reactions keyed on customer_profile_id.

**Tech Stack:** Go 1.26, Gin, GORM, bluemonday (sanitizer). Next.js 16, React 19, Tailwind.

**Prerequisite:** C1 (storefront auth) + C2 (customer profiles) must be on main.

---

## Task 1 — Migration 000014 (reviews schema)

**Files:**
- `services/marketplace-api/migrations/000014_reviews.up.sql` (NEW)
- `services/marketplace-api/migrations/000014_reviews.down.sql` (NEW)
- `services/marketplace-api/migrations.go` (EDIT — bump `ExpectedSchemaVersion`)

### Steps

- [ ] **1.1** Create `000014_reviews.up.sql` wrapped in `BEGIN; ... COMMIT;`:

```sql
-- 000014_reviews: Product reviews with media, replies, and reactions.
BEGIN;

CREATE TABLE reviews (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    product_id          UUID          NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    customer_profile_id UUID          REFERENCES customer_profiles(id) ON DELETE SET NULL,
    customer_name       VARCHAR(200)  NOT NULL,
    customer_email      VARCHAR(300)  NOT NULL,
    rating              INT           NOT NULL CHECK (rating >= 1 AND rating <= 5),
    title               VARCHAR(300),
    content             TEXT          NOT NULL,
    status              VARCHAR(20)   NOT NULL DEFAULT 'pending',
    verified_purchase   BOOLEAN       NOT NULL DEFAULT false,
    featured            BOOLEAN       NOT NULL DEFAULT false,
    helpful_count       INT           NOT NULL DEFAULT 0,
    not_helpful_count   INT           NOT NULL DEFAULT 0,
    published_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, product_id, customer_email)
);
CREATE INDEX reviews_product_status_idx ON reviews (product_id, status);
CREATE INDEX reviews_store_status_idx ON reviews (store_id, status);
CREATE INDEX reviews_customer_idx ON reviews (customer_email, store_id);

CREATE TABLE review_media (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id       UUID          NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    url             TEXT          NOT NULL,
    alt             VARCHAR(300),
    position        INT           NOT NULL DEFAULT 0,
    media_type      VARCHAR(20)   NOT NULL DEFAULT 'image',
    width           INT,
    height          INT,
    file_size       INT,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX rm_review_idx ON review_media (review_id);

CREATE TABLE review_replies (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id       UUID          NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    author_type     VARCHAR(20)   NOT NULL,
    author_name     VARCHAR(200)  NOT NULL,
    author_email    VARCHAR(300),
    content         TEXT          NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX rr_review_idx ON review_replies (review_id);

CREATE TABLE review_reactions (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id           UUID          NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    customer_profile_id UUID          NOT NULL REFERENCES customer_profiles(id) ON DELETE CASCADE,
    reaction            VARCHAR(20)   NOT NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (review_id, customer_profile_id)
);

COMMIT;
```

Note: `review_reactions` uses `customer_profile_id` (not `customer_email`) per spec section 8.1 — email is public and spoofable.

- [ ] **1.2** Create `000014_reviews.down.sql`:

```sql
BEGIN;
DROP TABLE IF EXISTS review_reactions;
DROP TABLE IF EXISTS review_replies;
DROP TABLE IF EXISTS review_media;
DROP TABLE IF EXISTS reviews;
COMMIT;
```

- [ ] **1.3** Bump `ExpectedSchemaVersion` in `services/marketplace-api/migrations.go`. The current value is `1` (it covers migrations 000001-000013 via a single assertion — bump to the migration number matching your schema state; follow whatever pattern C1/C2 set for 000013).

- [ ] **1.4** Run `make mp-migrate-up` and verify all four tables exist with correct constraints. Verify the UNIQUE constraints:
  - `reviews (store_id, product_id, customer_email)` — one review per product per customer
  - `review_reactions (review_id, customer_profile_id)` — one reaction per review per customer

---

## Task 2 — `internal/review/` models + repository

**Files:**
- `services/marketplace-api/internal/review/models.go` (NEW)
- `services/marketplace-api/internal/review/repository.go` (NEW)

### Steps

- [ ] **2.1** Create `internal/review/models.go` with GORM structs matching the migration schema:

```go
package review

import (
	"time"

	"github.com/google/uuid"
)

// ReviewStatus enumerates the moderation states.
type ReviewStatus string

const (
	StatusPending  ReviewStatus = "pending"
	StatusApproved ReviewStatus = "approved"
	StatusRejected ReviewStatus = "rejected"
)

// Review is the GORM model for the reviews table.
type Review struct {
	ID                string       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID          string       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID           string       `gorm:"column:store_id;type:uuid;not null"`
	ProductID         string       `gorm:"column:product_id;type:uuid;not null"`
	CustomerProfileID *string      `gorm:"column:customer_profile_id;type:uuid"`
	CustomerName      string       `gorm:"column:customer_name;type:varchar(200);not null"`
	CustomerEmail     string       `gorm:"column:customer_email;type:varchar(300);not null"`
	Rating            int          `gorm:"column:rating;not null"`
	Title             *string      `gorm:"column:title;type:varchar(300)"`
	Content           string       `gorm:"column:content;type:text;not null"`
	Status            ReviewStatus `gorm:"column:status;type:varchar(20);not null;default:'pending'"`
	VerifiedPurchase  bool         `gorm:"column:verified_purchase;not null;default:false"`
	Featured          bool         `gorm:"column:featured;not null;default:false"`
	HelpfulCount      int          `gorm:"column:helpful_count;not null;default:0"`
	NotHelpfulCount   int          `gorm:"column:not_helpful_count;not null;default:0"`
	PublishedAt       *time.Time   `gorm:"column:published_at"`
	CreatedAt         time.Time    `gorm:"column:created_at;not null"`
	UpdatedAt         time.Time    `gorm:"column:updated_at;not null"`

	// Associations — loaded via Preload, never saved.
	Media   []ReviewMedia `gorm:"foreignKey:ReviewID"`
	Replies []ReviewReply `gorm:"foreignKey:ReviewID"`
}

func (Review) TableName() string { return "reviews" }

// ReviewMedia is one photo attached to a review.
type ReviewMedia struct {
	ID        string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ReviewID  string    `gorm:"column:review_id;type:uuid;not null"`
	URL       string    `gorm:"column:url;type:text;not null"`
	Alt       *string   `gorm:"column:alt;type:varchar(300)"`
	Position  int       `gorm:"column:position;not null;default:0"`
	MediaType string    `gorm:"column:media_type;type:varchar(20);not null;default:'image'"`
	Width     *int      `gorm:"column:width"`
	Height    *int      `gorm:"column:height"`
	FileSize  *int      `gorm:"column:file_size"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

func (ReviewMedia) TableName() string { return "review_media" }

// ReviewReply is a merchant or customer reply on a review.
type ReviewReply struct {
	ID          string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ReviewID    string    `gorm:"column:review_id;type:uuid;not null"`
	AuthorType  string    `gorm:"column:author_type;type:varchar(20);not null"`
	AuthorName  string    `gorm:"column:author_name;type:varchar(200);not null"`
	AuthorEmail *string   `gorm:"column:author_email;type:varchar(300)"`
	Content     string    `gorm:"column:content;type:text;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;not null"`
}

func (ReviewReply) TableName() string { return "review_replies" }

// ReviewReaction tracks a customer's helpful/not_helpful vote.
type ReviewReaction struct {
	ID                string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ReviewID          string    `gorm:"column:review_id;type:uuid;not null"`
	CustomerProfileID string    `gorm:"column:customer_profile_id;type:uuid;not null"`
	Reaction          string    `gorm:"column:reaction;type:varchar(20);not null"`
	CreatedAt         time.Time `gorm:"column:created_at;not null"`
}

func (ReviewReaction) TableName() string { return "review_reactions" }

// ReviewSummary holds the computed aggregate stats for a product's reviews.
type ReviewSummary struct {
	AverageRating float64          `json:"average_rating"`
	TotalCount    int              `json:"total_count"`
	Distribution  map[int]int      `json:"distribution"` // {1: 3, 2: 0, 3: 5, 4: 12, 5: 8}
}
```

- [ ] **2.2** Create `internal/review/repository.go` with the `Repository` interface and GORM implementation:

```go
package review

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ListFilter specifies the filter and pagination params for review listing.
type ListFilter struct {
	StoreID   string
	TenantID  string
	ProductID string       // optional — filters by product
	Status    ReviewStatus // optional — filters by status
	Search    string       // optional — searches customer_name, title, content
	Page      int
	PageSize  int
}

// Repository is the data-access contract for the review domain.
type Repository interface {
	// Admin operations
	ListByStore(ctx context.Context, f ListFilter) ([]Review, int64, error)
	GetByID(ctx context.Context, id, storeID, tenantID string) (*Review, error)
	Create(ctx context.Context, r *Review) error
	Update(ctx context.Context, r *Review) error
	Delete(ctx context.Context, id, storeID, tenantID string) error

	// Media — with FOR UPDATE lock on the parent review
	CountMediaForUpdate(ctx context.Context, tx *gorm.DB, reviewID string) (int, error)
	CreateMedia(ctx context.Context, m *ReviewMedia) error

	// Replies
	CreateReply(ctx context.Context, r *ReviewReply) error

	// Reactions — atomic toggle
	UpsertReaction(ctx context.Context, reaction *ReviewReaction) error
	DeleteReaction(ctx context.Context, reviewID, customerProfileID string) error

	// Storefront reads
	ListApprovedByProduct(ctx context.Context, productID, storeID string, page, pageSize int) ([]Review, int64, error)
	GetSummary(ctx context.Context, productID, storeID string) (*ReviewSummary, error)

	// Verified purchase check
	HasPurchasedProduct(ctx context.Context, storeID, customerEmail, productID string) (bool, error)

	// Duplicate check
	ExistsByCustomer(ctx context.Context, storeID, productID, customerEmail string) (bool, error)
}

// GORMRepository implements Repository using GORM.
type GORMRepository struct {
	db *gorm.DB
}

// NewRepository constructs a GORMRepository.
func NewRepository(db *gorm.DB) *GORMRepository {
	return &GORMRepository{db: db}
}
```

- [ ] **2.3** Implement `ListByStore` with status filter, product filter, search, pagination, and preloaded media + replies:

```go
func (r *GORMRepository) ListByStore(ctx context.Context, f ListFilter) ([]Review, int64, error) {
	q := r.db.WithContext(ctx).Where("store_id = ? AND tenant_id = ?", f.StoreID, f.TenantID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.ProductID != "" {
		q = q.Where("product_id = ?", f.ProductID)
	}
	if f.Search != "" {
		like := "%" + f.Search + "%"
		q = q.Where("(customer_name ILIKE ? OR title ILIKE ? OR content ILIKE ?)", like, like, like)
	}

	var total int64
	if err := q.Model(&Review{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("review: count: %w", err)
	}

	var reviews []Review
	err := q.
		Preload("Media", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).
		Preload("Replies", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		Order("created_at DESC").
		Offset((f.Page - 1) * f.PageSize).
		Limit(f.PageSize).
		Find(&reviews).Error
	if err != nil {
		return nil, 0, fmt.Errorf("review: list: %w", err)
	}
	return reviews, total, nil
}
```

- [ ] **2.4** Implement `GetByID` with store+tenant scoping and full preloads:

```go
func (r *GORMRepository) GetByID(ctx context.Context, id, storeID, tenantID string) (*Review, error) {
	var rev Review
	err := r.db.WithContext(ctx).
		Where("id = ? AND store_id = ? AND tenant_id = ?", id, storeID, tenantID).
		Preload("Media", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).
		Preload("Replies", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		First(&rev).Error
	if err != nil {
		return nil, fmt.Errorf("review: get: %w", err)
	}
	return &rev, nil
}
```

- [ ] **2.5** Implement `CountMediaForUpdate` with `SELECT ... FOR UPDATE` on the review row to prevent concurrent photo uploads exceeding max 3:

```go
func (r *GORMRepository) CountMediaForUpdate(ctx context.Context, tx *gorm.DB, reviewID string) (int, error) {
	// Lock the review row to serialize concurrent media uploads.
	var rev Review
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").
		Where("id = ?", reviewID).
		First(&rev).Error; err != nil {
		return 0, fmt.Errorf("review: lock for media count: %w", err)
	}

	var count int64
	if err := tx.WithContext(ctx).
		Model(&ReviewMedia{}).
		Where("review_id = ?", reviewID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("review: count media: %w", err)
	}
	return int(count), nil
}
```

- [ ] **2.6** Implement `UpsertReaction` with atomic helpful_count toggle. Uses `INSERT ON CONFLICT DO NOTHING` + manual count adjustment:

```go
func (r *GORMRepository) UpsertReaction(ctx context.Context, reaction *ReviewReaction) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check for existing reaction by this customer on this review.
		var existing ReviewReaction
		err := tx.Where("review_id = ? AND customer_profile_id = ?",
			reaction.ReviewID, reaction.CustomerProfileID).First(&existing).Error

		if err == nil {
			// Existing reaction found.
			if existing.Reaction == reaction.Reaction {
				// Same reaction — remove it (toggle off).
				if err := tx.Delete(&existing).Error; err != nil {
					return fmt.Errorf("review: delete reaction: %w", err)
				}
				// Decrement the matching counter atomically.
				col := "helpful_count"
				if existing.Reaction == "not_helpful" {
					col = "not_helpful_count"
				}
				return tx.Model(&Review{}).Where("id = ?", reaction.ReviewID).
					Update(col, gorm.Expr(col+" - 1")).Error
			}
			// Different reaction — swap. Decrement old, increment new.
			oldCol := "helpful_count"
			newCol := "not_helpful_count"
			if existing.Reaction == "not_helpful" {
				oldCol, newCol = newCol, oldCol
			}
			if err := tx.Model(&Review{}).Where("id = ?", reaction.ReviewID).
				Updates(map[string]interface{}{
					oldCol: gorm.Expr(oldCol + " - 1"),
					newCol: gorm.Expr(newCol + " + 1"),
				}).Error; err != nil {
				return fmt.Errorf("review: swap reaction counts: %w", err)
			}
			existing.Reaction = reaction.Reaction
			return tx.Save(&existing).Error
		}

		// No existing reaction — insert and increment.
		if err := tx.Create(reaction).Error; err != nil {
			return fmt.Errorf("review: create reaction: %w", err)
		}
		col := "helpful_count"
		if reaction.Reaction == "not_helpful" {
			col = "not_helpful_count"
		}
		return tx.Model(&Review{}).Where("id = ?", reaction.ReviewID).
			Update(col, gorm.Expr(col+" + 1")).Error
	})
}
```

- [ ] **2.7** Implement `HasPurchasedProduct` — read-only join query on `orders` + `order_items`:

```go
func (r *GORMRepository) HasPurchasedProduct(ctx context.Context, storeID, customerEmail, productID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("orders").
		Joins("JOIN order_items ON order_items.order_id = orders.id").
		Where("orders.store_id = ? AND orders.customer_email = ? AND order_items.product_id = ?",
			storeID, customerEmail, productID).
		Where("orders.status NOT IN ('cancelled', 'refunded')").
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("review: verified purchase check: %w", err)
	}
	return count > 0, nil
}
```

- [ ] **2.8** Implement `ExistsByCustomer`:

```go
func (r *GORMRepository) ExistsByCustomer(ctx context.Context, storeID, productID, customerEmail string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Review{}).
		Where("store_id = ? AND product_id = ? AND customer_email = ?",
			storeID, productID, customerEmail).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("review: exists check: %w", err)
	}
	return count > 0, nil
}
```

- [ ] **2.9** Implement `ListApprovedByProduct` (storefront — approved only, newest first, with media + replies):

```go
func (r *GORMRepository) ListApprovedByProduct(ctx context.Context, productID, storeID string, page, pageSize int) ([]Review, int64, error) {
	q := r.db.WithContext(ctx).
		Where("product_id = ? AND store_id = ? AND status = ?", productID, storeID, StatusApproved)

	var total int64
	if err := q.Model(&Review{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("review: storefront count: %w", err)
	}

	var reviews []Review
	err := q.
		Preload("Media", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).
		Preload("Replies", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		Order("featured DESC, created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&reviews).Error
	if err != nil {
		return nil, 0, fmt.Errorf("review: storefront list: %w", err)
	}
	return reviews, total, nil
}
```

- [ ] **2.10** Implement `GetSummary` — single query with `AVG`, `COUNT`, and rating distribution:

```go
func (r *GORMRepository) GetSummary(ctx context.Context, productID, storeID string) (*ReviewSummary, error) {
	type row struct {
		Rating int
		Cnt    int
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Model(&Review{}).
		Select("rating, COUNT(*) as cnt").
		Where("product_id = ? AND store_id = ? AND status = ?", productID, storeID, StatusApproved).
		Group("rating").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("review: summary: %w", err)
	}

	dist := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	total := 0
	sum := 0
	for _, r := range rows {
		dist[r.Rating] = r.Cnt
		total += r.Cnt
		sum += r.Rating * r.Cnt
	}
	avg := 0.0
	if total > 0 {
		avg = float64(sum) / float64(total)
	}
	return &ReviewSummary{
		AverageRating: avg,
		TotalCount:    total,
		Distribution:  dist,
	}, nil
}
```

- [ ] **2.11** Implement remaining CRUD methods (`Create`, `Update`, `Delete`, `CreateMedia`, `CreateReply`, `DeleteReaction`). Follow the patterns above — simple GORM calls with store+tenant scoping.

- [ ] **2.12** Verify: `go build ./internal/review/...` compiles cleanly.

---

## Task 3 — Review service (business logic)

**Files:**
- `services/marketplace-api/internal/review/service.go` (NEW)
- `services/marketplace-api/internal/review/sanitizer.go` (NEW)

### Steps

- [ ] **3.1** Create `internal/review/sanitizer.go` — a plain-text-only bluemonday policy for review content. Reviews are plain text (not rich HTML like product descriptions):

```go
package review

import "github.com/microcosm-cc/bluemonday"

var textPolicy = bluemonday.StrictPolicy()

// SanitizeText strips all HTML tags — reviews are plain text only.
// The result is safe to render with CSS white-space:pre-wrap.
func SanitizeText(in string) string {
	if in == "" {
		return ""
	}
	return textPolicy.Sanitize(in)
}
```

- [ ] **3.2** Create `internal/review/service.go` with `Config` and `Service`:

```go
package review

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Config groups the service dependencies.
type Config struct {
	DB       *gorm.DB
	Repo     Repository
	Uploader media.Uploader
	Logger   *slog.Logger
}

// Service owns the review business logic.
type Service struct {
	db       *gorm.DB
	repo     Repository
	uploader media.Uploader
	logger   *slog.Logger
}

// NewService constructs a review Service.
func NewService(cfg Config) *Service {
	return &Service{
		db:       cfg.DB,
		repo:     cfg.Repo,
		uploader: cfg.Uploader,
		logger:   cfg.Logger,
	}
}
```

- [ ] **3.3** Implement `CreateReview` with all safety checks:

```go
// CreateReviewInput holds the validated input for submitting a review.
type CreateReviewInput struct {
	TenantID          string
	StoreID           string
	ProductID         string
	CustomerProfileID string
	CustomerName      string
	CustomerEmail     string
	Rating            int
	Title             string
	Content           string
	StoreOwnerEmail   string // for self-review prevention
	CustomerStatus    string // from customer_profiles.status
}

// CreateReview validates business rules and persists a new review.
func (s *Service) CreateReview(ctx context.Context, in CreateReviewInput) (*Review, error) {
	// 1. Blocked customer check.
	if in.CustomerStatus == "blocked" {
		return nil, apperrors.New(apperrors.CodeForbidden,
			"your account has been restricted", nil)
	}

	// 2. Self-review prevention — merchant cannot review own products.
	if in.CustomerEmail == in.StoreOwnerEmail {
		return nil, apperrors.New(apperrors.CodeForbidden,
			"store owners cannot review their own products", nil)
	}

	// 3. One review per product per customer (pre-check before hitting UNIQUE).
	exists, err := s.repo.ExistsByCustomer(ctx, in.StoreID, in.ProductID, in.CustomerEmail)
	if err != nil {
		return nil, fmt.Errorf("review: exists check: %w", err)
	}
	if exists {
		return nil, apperrors.New(apperrors.CodeDuplicateReview,
			"you have already reviewed this product", nil)
	}

	// 4. Sanitize content (strip all HTML).
	sanitizedContent := SanitizeText(in.Content)
	var sanitizedTitle *string
	if in.Title != "" {
		t := SanitizeText(in.Title)
		sanitizedTitle = &t
	}

	// 5. Verified purchase check.
	verified, err := s.repo.HasPurchasedProduct(ctx, in.StoreID, in.CustomerEmail, in.ProductID)
	if err != nil {
		s.logger.Warn("review: verified purchase check failed, defaulting to false",
			"err", err, "customer_email", in.CustomerEmail, "product_id", in.ProductID)
		verified = false
	}

	now := time.Now()
	rev := &Review{
		TenantID:          in.TenantID,
		StoreID:           in.StoreID,
		ProductID:         in.ProductID,
		CustomerProfileID: &in.CustomerProfileID,
		CustomerName:      in.CustomerName,
		CustomerEmail:     in.CustomerEmail,
		Rating:            in.Rating,
		Title:             sanitizedTitle,
		Content:           sanitizedContent,
		Status:            StatusPending,
		VerifiedPurchase:  verified,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.repo.Create(ctx, rev); err != nil {
		return nil, fmt.Errorf("review: create: %w", err)
	}
	return rev, nil
}
```

Note: `apperrors.CodeDuplicateReview` needs to be added to `pkg/apperrors/codes.go` and `internal/handlers/admin/errors.go` (maps to 409 Conflict).

- [ ] **3.4** Implement `ApproveReview` and `RejectReview`:

```go
// ApproveReview sets status=approved and published_at=now.
func (s *Service) ApproveReview(ctx context.Context, id, storeID, tenantID string) (*Review, error) {
	rev, err := s.repo.GetByID(ctx, id, storeID, tenantID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	rev.Status = StatusApproved
	rev.PublishedAt = &now
	rev.UpdatedAt = now
	if err := s.repo.Update(ctx, rev); err != nil {
		return nil, fmt.Errorf("review: approve: %w", err)
	}
	return rev, nil
}

// RejectReview sets status=rejected.
func (s *Service) RejectReview(ctx context.Context, id, storeID, tenantID string) (*Review, error) {
	rev, err := s.repo.GetByID(ctx, id, storeID, tenantID)
	if err != nil {
		return nil, err
	}
	rev.Status = StatusRejected
	rev.PublishedAt = nil
	rev.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, rev); err != nil {
		return nil, fmt.Errorf("review: reject: %w", err)
	}
	return rev, nil
}
```

- [ ] **3.5** Implement `ToggleFeatured`:

```go
// ToggleFeatured flips the featured flag on a review.
func (s *Service) ToggleFeatured(ctx context.Context, id, storeID, tenantID string, featured bool) (*Review, error) {
	rev, err := s.repo.GetByID(ctx, id, storeID, tenantID)
	if err != nil {
		return nil, err
	}
	rev.Featured = featured
	rev.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, rev); err != nil {
		return nil, fmt.Errorf("review: toggle featured: %w", err)
	}
	return rev, nil
}
```

- [ ] **3.6** Implement `AddMerchantReply`:

```go
// AddMerchantReply adds a merchant reply to a review.
func (s *Service) AddMerchantReply(ctx context.Context, reviewID, storeID, tenantID, authorName, authorEmail, content string) (*ReviewReply, error) {
	// Verify the review exists and belongs to this store.
	if _, err := s.repo.GetByID(ctx, reviewID, storeID, tenantID); err != nil {
		return nil, err
	}

	reply := &ReviewReply{
		ReviewID:    reviewID,
		AuthorType:  "merchant",
		AuthorName:  authorName,
		AuthorEmail: &authorEmail,
		Content:     SanitizeText(content),
		CreatedAt:   time.Now(),
	}
	if err := s.repo.CreateReply(ctx, reply); err != nil {
		return nil, fmt.Errorf("review: add reply: %w", err)
	}
	return reply, nil
}
```

- [ ] **3.7** Implement `UploadMedia` with the FOR UPDATE race protection:

```go
const MaxReviewMedia = 3

// UploadMedia attaches a photo to a review. Max 3 per review.
// Uses SELECT FOR UPDATE to prevent concurrent uploads exceeding the limit.
func (s *Service) UploadMedia(ctx context.Context, reviewID, storeID, tenantID string, storageKey string) (*ReviewMedia, error) {
	// Verify the review exists.
	rev, err := s.repo.GetByID(ctx, reviewID, storeID, tenantID)
	if err != nil {
		return nil, err
	}

	// Verify the uploaded object exists in GCS.
	attrs, err := s.uploader.Verify(ctx, storageKey)
	if err != nil {
		if errors.Is(err, media.ErrNotFound) {
			return nil, apperrors.New(apperrors.CodeUploadNotFound,
				"uploaded file not found", nil)
		}
		return nil, fmt.Errorf("review: verify upload: %w", err)
	}

	// Content-type validation — images only.
	if len(attrs.ContentType) < 6 || attrs.ContentType[:6] != "image/" {
		return nil, apperrors.ValidationFailed("media_type",
			"only image files are allowed for review photos")
	}

	// Max file size: 5MB.
	if attrs.Size > 5*1024*1024 {
		return nil, apperrors.New(apperrors.CodePayloadTooLarge,
			"review photos must be under 5MB", nil)
	}

	var m *ReviewMedia
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		count, err := s.repo.CountMediaForUpdate(ctx, tx, reviewID)
		if err != nil {
			return err
		}
		if count >= MaxReviewMedia {
			return apperrors.ValidationFailed("media",
				fmt.Sprintf("reviews can have at most %d photos", MaxReviewMedia))
		}

		m = &ReviewMedia{
			ReviewID:  reviewID,
			URL:       storageKey, // frontend resolves to full CDN URL
			Position:  count,
			MediaType: "image",
			FileSize:  intPtr(int(attrs.Size)),
			CreatedAt: time.Now(),
		}
		return tx.Create(m).Error
	})
	if err != nil {
		return nil, fmt.Errorf("review: upload media: %w", err)
	}
	_ = rev // used for ownership check above
	return m, nil
}

func intPtr(v int) *int { return &v }
```

- [ ] **3.8** Implement `React` (helpful/not_helpful toggle):

```go
// React records or toggles a helpful/not_helpful reaction.
func (s *Service) React(ctx context.Context, reviewID, customerProfileID, reaction string) error {
	if reaction != "helpful" && reaction != "not_helpful" {
		return apperrors.ValidationFailed("reaction", "must be 'helpful' or 'not_helpful'")
	}
	r := &ReviewReaction{
		ReviewID:          reviewID,
		CustomerProfileID: customerProfileID,
		Reaction:          reaction,
	}
	return s.repo.UpsertReaction(ctx, r)
}
```

- [ ] **3.9** Add `CodeDuplicateReview` to `pkg/apperrors/codes.go`:

```go
CodeDuplicateReview Code = "duplicate_review"
```

And map it in `internal/handlers/admin/errors.go`:

```go
apperrors.CodeDuplicateReview: http.StatusConflict,
```

- [ ] **3.10** Verify: `go build ./internal/review/...` compiles cleanly.

---

## Task 4 — Admin review handler

**Files:**
- `services/marketplace-api/internal/handlers/admin/reviews.go` (NEW)
- `services/marketplace-api/internal/handlers/admin/review_dto.go` (NEW)

### Steps

- [ ] **4.1** Create `internal/handlers/admin/review_dto.go` with request/response types:

```go
package admin

import (
	"time"

	"github.com/mark8ly/marketplace-api/internal/review"
)

// AdminReviewResponse is the wire DTO for a review in admin context.
type AdminReviewResponse struct {
	ID                string                    `json:"id"`
	StoreID           string                    `json:"store_id"`
	ProductID         string                    `json:"product_id"`
	CustomerProfileID *string                   `json:"customer_profile_id,omitempty"`
	CustomerName      string                    `json:"customer_name"`
	CustomerEmail     string                    `json:"customer_email"`
	Rating            int                       `json:"rating"`
	Title             *string                   `json:"title,omitempty"`
	Content           string                    `json:"content"`
	Status            string                    `json:"status"`
	VerifiedPurchase  bool                      `json:"verified_purchase"`
	Featured          bool                      `json:"featured"`
	HelpfulCount      int                       `json:"helpful_count"`
	NotHelpfulCount   int                       `json:"not_helpful_count"`
	PublishedAt       *time.Time                `json:"published_at,omitempty"`
	CreatedAt         time.Time                 `json:"created_at"`
	UpdatedAt         time.Time                 `json:"updated_at"`
	Media             []AdminReviewMediaResponse `json:"media"`
	Replies           []AdminReviewReplyResponse `json:"replies"`
}

type AdminReviewMediaResponse struct {
	ID        string  `json:"id"`
	URL       string  `json:"url"`
	Alt       *string `json:"alt,omitempty"`
	Position  int     `json:"position"`
	MediaType string  `json:"media_type"`
	Width     *int    `json:"width,omitempty"`
	Height    *int    `json:"height,omitempty"`
	FileSize  *int    `json:"file_size,omitempty"`
}

type AdminReviewReplyResponse struct {
	ID          string    `json:"id"`
	AuthorType  string    `json:"author_type"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail *string   `json:"author_email,omitempty"` // visible to admin only
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
}

// ModerationRequest is the body for PATCH /reviews/:id.
type ModerationRequest struct {
	Status   *string `json:"status" binding:"omitempty,oneof=approved rejected"`
	Featured *bool   `json:"featured"`
}

// MerchantReplyRequest is the body for POST /reviews/:id/reply.
type MerchantReplyRequest struct {
	Content string `json:"content" binding:"required,min=1,max=2000"`
}

// ToAdminReviewResponse converts domain Review to wire DTO.
func ToAdminReviewResponse(r *review.Review) AdminReviewResponse {
	resp := AdminReviewResponse{
		ID:                r.ID,
		StoreID:           r.StoreID,
		ProductID:         r.ProductID,
		CustomerProfileID: r.CustomerProfileID,
		CustomerName:      r.CustomerName,
		CustomerEmail:     r.CustomerEmail,
		Rating:            r.Rating,
		Title:             r.Title,
		Content:           r.Content,
		Status:            string(r.Status),
		VerifiedPurchase:  r.VerifiedPurchase,
		Featured:          r.Featured,
		HelpfulCount:      r.HelpfulCount,
		NotHelpfulCount:   r.NotHelpfulCount,
		PublishedAt:        r.PublishedAt,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		Media:             make([]AdminReviewMediaResponse, 0, len(r.Media)),
		Replies:           make([]AdminReviewReplyResponse, 0, len(r.Replies)),
	}
	for i := range r.Media {
		resp.Media = append(resp.Media, AdminReviewMediaResponse{
			ID:        r.Media[i].ID,
			URL:       r.Media[i].URL,
			Alt:       r.Media[i].Alt,
			Position:  r.Media[i].Position,
			MediaType: r.Media[i].MediaType,
			Width:     r.Media[i].Width,
			Height:    r.Media[i].Height,
			FileSize:  r.Media[i].FileSize,
		})
	}
	for i := range r.Replies {
		resp.Replies = append(resp.Replies, AdminReviewReplyResponse{
			ID:          r.Replies[i].ID,
			AuthorType:  r.Replies[i].AuthorType,
			AuthorName:  r.Replies[i].AuthorName,
			AuthorEmail: r.Replies[i].AuthorEmail,
			Content:     r.Replies[i].Content,
			CreatedAt:   r.Replies[i].CreatedAt,
		})
	}
	return resp
}
```

- [ ] **4.2** Create `internal/handlers/admin/reviews.go` — the handler struct following the `CategoryHandler` pattern:

```go
package admin

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/review"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ReviewHandler bundles dependencies for admin review moderation.
type ReviewHandler struct {
	svc    *review.Service
	repo   review.Repository
	logger *slog.Logger
}

// NewReviewHandler constructs a ReviewHandler.
func NewReviewHandler(svc *review.Service, repo review.Repository, logger *slog.Logger) *ReviewHandler {
	return &ReviewHandler{svc: svc, repo: repo, logger: logger}
}
```

- [ ] **4.3** Implement `List` (moderation list with status filter):

```go
// List handles GET /admin/stores/:storeId/reviews.
func (h *ReviewHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 { page = 1 }
	if pageSize < 1 || pageSize > 100 { pageSize = 20 }

	f := review.ListFilter{
		StoreID:   storeID,
		TenantID:  tenantID,
		Status:    review.ReviewStatus(c.Query("status")),
		ProductID: c.Query("product_id"),
		Search:    c.Query("search"),
		Page:      page,
		PageSize:  pageSize,
	}

	reviews, total, err := h.repo.ListByStore(c.Request.Context(), f)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]AdminReviewResponse, 0, len(reviews))
	for i := range reviews {
		out = append(out, ToAdminReviewResponse(&reviews[i]))
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 { totalPages++ }

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}
```

- [ ] **4.4** Implement `Get`, `Moderate` (approve/reject/feature), `Reply`, `Delete`:

```go
// Get handles GET /admin/stores/:storeId/reviews/:id.
func (h *ReviewHandler) Get(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	rev, err := h.repo.GetByID(c.Request.Context(), id, storeID, tenantID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminReviewResponse(rev))
}

// Moderate handles PATCH /admin/stores/:storeId/reviews/:id.
// Accepts status (approved/rejected) and/or featured toggle.
func (h *ReviewHandler) Moderate(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	var req ModerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	var rev *review.Review
	var err error

	if req.Status != nil {
		switch review.ReviewStatus(*req.Status) {
		case review.StatusApproved:
			rev, err = h.svc.ApproveReview(c.Request.Context(), id, storeID, tenantID)
		case review.StatusRejected:
			rev, err = h.svc.RejectReview(c.Request.Context(), id, storeID, tenantID)
		}
		if err != nil {
			RespondErr(c, err, h.logger)
			return
		}
	}

	if req.Featured != nil {
		rev, err = h.svc.ToggleFeatured(c.Request.Context(), id, storeID, tenantID, *req.Featured)
		if err != nil {
			RespondErr(c, err, h.logger)
			return
		}
	}

	if rev == nil {
		rev, err = h.repo.GetByID(c.Request.Context(), id, storeID, tenantID)
		if err != nil {
			RespondErr(c, err, h.logger)
			return
		}
	}
	c.JSON(http.StatusOK, ToAdminReviewResponse(rev))
}

// Reply handles POST /admin/stores/:storeId/reviews/:id/reply.
func (h *ReviewHandler) Reply(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	userEmail := c.GetString("user_email")
	userName := c.GetString("user_name")
	if userName == "" { userName = "Store" }

	var req MerchantReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	reply, err := h.svc.AddMerchantReply(c.Request.Context(), id, storeID, tenantID, userName, userEmail, req.Content)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusCreated, AdminReviewReplyResponse{
		ID:          reply.ID,
		AuthorType:  reply.AuthorType,
		AuthorName:  reply.AuthorName,
		AuthorEmail: reply.AuthorEmail,
		Content:     reply.Content,
		CreatedAt:   reply.CreatedAt,
	})
}

// Delete handles DELETE /admin/stores/:storeId/reviews/:id.
func (h *ReviewHandler) Delete(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	if err := h.repo.Delete(c.Request.Context(), id, storeID, tenantID); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.Status(http.StatusNoContent)
}
```

- [ ] **4.5** Verify: `go build ./internal/handlers/admin/...` compiles cleanly.

---

## Task 5 — Storefront review handlers

**Files:**
- `services/marketplace-api/internal/handlers/storefront/reviews.go` (NEW)
- `services/marketplace-api/internal/handlers/storefront/review_dto.go` (NEW)

### Steps

- [ ] **5.1** Create `internal/handlers/storefront/review_dto.go` — storefront DTOs that NEVER expose `author_email`, `customer_email`, or admin fields:

```go
package storefront

import (
	"time"

	"github.com/mark8ly/marketplace-api/internal/review"
)

// StorefrontReviewResponse is the public wire DTO. No customer_email, no author_email.
type StorefrontReviewResponse struct {
	ID               string                          `json:"id"`
	Rating           int                             `json:"rating"`
	Title            *string                         `json:"title,omitempty"`
	Content          string                          `json:"content"`
	CustomerName     string                          `json:"customer_name"`
	VerifiedPurchase bool                            `json:"verified_purchase"`
	Featured         bool                            `json:"featured"`
	HelpfulCount     int                             `json:"helpful_count"`
	NotHelpfulCount  int                             `json:"not_helpful_count"`
	PublishedAt      *time.Time                      `json:"published_at,omitempty"`
	CreatedAt        time.Time                       `json:"created_at"`
	Media            []StorefrontReviewMediaResponse  `json:"media"`
	Replies          []StorefrontReviewReplyResponse  `json:"replies"`
}

type StorefrontReviewMediaResponse struct {
	URL       string  `json:"url"`
	Alt       *string `json:"alt,omitempty"`
	Position  int     `json:"position"`
	Width     *int    `json:"width,omitempty"`
	Height    *int    `json:"height,omitempty"`
}

type StorefrontReviewReplyResponse struct {
	AuthorType string    `json:"author_type"`
	AuthorName string    `json:"author_name"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

// SubmitReviewRequest is the body for POST /storefront/stores/:slug/products/:handle/reviews.
type SubmitReviewRequest struct {
	Rating  int    `json:"rating" binding:"required,min=1,max=5"`
	Title   string `json:"title" binding:"omitempty,max=300"`
	Content string `json:"content" binding:"required,min=20,max=5000"`
}

// ReactionRequest is the body for POST /storefront/stores/:slug/reviews/:id/reaction.
type ReactionRequest struct {
	Reaction string `json:"reaction" binding:"required,oneof=helpful not_helpful"`
}

// ToStorefrontReviewResponse converts domain Review to the public DTO.
func ToStorefrontReviewResponse(r *review.Review) StorefrontReviewResponse {
	resp := StorefrontReviewResponse{
		ID:               r.ID,
		Rating:           r.Rating,
		Title:            r.Title,
		Content:          r.Content,
		CustomerName:     r.CustomerName,
		VerifiedPurchase: r.VerifiedPurchase,
		Featured:         r.Featured,
		HelpfulCount:     r.HelpfulCount,
		NotHelpfulCount:  r.NotHelpfulCount,
		PublishedAt:      r.PublishedAt,
		CreatedAt:        r.CreatedAt,
		Media:            make([]StorefrontReviewMediaResponse, 0, len(r.Media)),
		Replies:          make([]StorefrontReviewReplyResponse, 0, len(r.Replies)),
	}
	for i := range r.Media {
		resp.Media = append(resp.Media, StorefrontReviewMediaResponse{
			URL:      r.Media[i].URL,
			Alt:      r.Media[i].Alt,
			Position: r.Media[i].Position,
			Width:    r.Media[i].Width,
			Height:   r.Media[i].Height,
		})
	}
	for i := range r.Replies {
		// SECURITY: Never return author_email in storefront responses.
		resp.Replies = append(resp.Replies, StorefrontReviewReplyResponse{
			AuthorType: r.Replies[i].AuthorType,
			AuthorName: r.Replies[i].AuthorName,
			Content:    r.Replies[i].Content,
			CreatedAt:  r.Replies[i].CreatedAt,
		})
	}
	return resp
}
```

- [ ] **5.2** Create `internal/handlers/storefront/reviews.go`:

```go
package storefront

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/review"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ReviewsHandler serves the public storefront review endpoints.
type ReviewsHandler struct {
	svc         *review.Service
	repo        review.Repository
	productRepo product.Repository
	logger      *slog.Logger
}

// NewReviewsHandler constructs a ReviewsHandler.
func NewReviewsHandler(
	svc *review.Service,
	repo review.Repository,
	productRepo product.Repository,
	logger *slog.Logger,
) *ReviewsHandler {
	return &ReviewsHandler{svc: svc, repo: repo, productRepo: productRepo, logger: logger}
}
```

- [ ] **5.3** Implement `ListProductReviews` — approved reviews + summary:

```go
// ListProductReviews handles GET /storefront/stores/:slug/products/:handle/reviews.
func (h *ReviewsHandler) ListProductReviews(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	handle := c.Param("handle")

	// Resolve handle to product ID.
	p, err := h.productRepo.GetByHandle(c.Request.Context(), handle, store.ID, store.TenantID)
	if err != nil {
		respondNotFound(c)
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 { page = 1 }
	if pageSize < 1 || pageSize > 50 { pageSize = 10 }

	reviews, total, err := h.repo.ListApprovedByProduct(c.Request.Context(), p.ID, store.ID, page, pageSize)
	if err != nil {
		h.logger.Error("review: list approved", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			map[string]any{"error": "internal", "message": "internal server error"})
		return
	}

	summary, err := h.repo.GetSummary(c.Request.Context(), p.ID, store.ID)
	if err != nil {
		h.logger.Error("review: get summary", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			map[string]any{"error": "internal", "message": "internal server error"})
		return
	}

	out := make([]StorefrontReviewResponse, 0, len(reviews))
	for i := range reviews {
		out = append(out, ToStorefrontReviewResponse(&reviews[i]))
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 { totalPages++ }

	c.JSON(http.StatusOK, gin.H{
		"data":    out,
		"summary": summary,
		"meta": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}
```

- [ ] **5.4** Implement `SubmitReview` — auth required, delegates to service:

```go
// SubmitReview handles POST /storefront/stores/:slug/products/:handle/reviews.
// Requires customer auth (RequireCustomerAuth middleware).
func (h *ReviewsHandler) SubmitReview(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	handle := c.Param("handle")

	customerProfileID := c.GetString("customer_profile_id")
	customerEmail := c.GetString("customer_email")
	customerName := c.GetString("customer_name")
	customerStatus := c.GetString("customer_status")

	// Resolve product by handle.
	p, err := h.productRepo.GetByHandle(c.Request.Context(), handle, store.ID, store.TenantID)
	if err != nil {
		respondNotFound(c)
		return
	}

	var req SubmitReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			map[string]any{"error": "validation_failed", "message": err.Error()})
		return
	}

	rev, err := h.svc.CreateReview(c.Request.Context(), review.CreateReviewInput{
		TenantID:          store.TenantID,
		StoreID:           store.ID,
		ProductID:         p.ID,
		CustomerProfileID: customerProfileID,
		CustomerName:      customerName,
		CustomerEmail:     customerEmail,
		Rating:            req.Rating,
		Title:             req.Title,
		Content:           req.Content,
		StoreOwnerEmail:   store.OwnerEmail, // for self-review prevention
		CustomerStatus:    customerStatus,
	})
	if err != nil {
		// Use storefront error handling — typed errors return proper codes.
		var ae *apperrors.Error
		if errors.As(err, &ae) {
			status := http.StatusBadRequest
			switch ae.Code {
			case apperrors.CodeForbidden:
				status = http.StatusForbidden
			case apperrors.CodeDuplicateReview:
				status = http.StatusConflict
			}
			c.AbortWithStatusJSON(status, map[string]any{
				"error": string(ae.Code), "message": ae.Message,
			})
			return
		}
		h.logger.Error("review: submit", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			map[string]any{"error": "internal", "message": "internal server error"})
		return
	}

	c.JSON(http.StatusCreated, ToStorefrontReviewResponse(rev))
}
```

Note: Add `import "errors"` to the imports.

- [ ] **5.5** Implement `React` and `UploadMedia`:

```go
// React handles POST /storefront/stores/:slug/reviews/:id/reaction.
// Requires customer auth.
func (h *ReviewsHandler) React(c *gin.Context) {
	reviewID := c.Param("id")
	customerProfileID := c.GetString("customer_profile_id")

	var req ReactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			map[string]any{"error": "validation_failed", "message": err.Error()})
		return
	}

	if err := h.svc.React(c.Request.Context(), reviewID, customerProfileID, req.Reaction); err != nil {
		h.logger.Error("review: react", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			map[string]any{"error": "internal", "message": "internal server error"})
		return
	}
	c.Status(http.StatusNoContent)
}

// UploadReviewMedia handles POST /storefront/stores/:slug/reviews/:id/media.
// Requires customer auth. Body: { "storage_key": "tenants/.../file.jpg" }.
func (h *ReviewsHandler) UploadReviewMedia(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	reviewID := c.Param("id")

	var req struct {
		StorageKey string `json:"storage_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			map[string]any{"error": "validation_failed", "message": err.Error()})
		return
	}

	m, err := h.svc.UploadMedia(c.Request.Context(), reviewID, store.ID, store.TenantID, req.StorageKey)
	if err != nil {
		var ae *apperrors.Error
		if errors.As(err, &ae) {
			status := http.StatusBadRequest
			if ae.Code == apperrors.CodePayloadTooLarge {
				status = http.StatusRequestEntityTooLarge
			}
			c.AbortWithStatusJSON(status, map[string]any{
				"error": string(ae.Code), "message": ae.Message,
			})
			return
		}
		h.logger.Error("review: upload media", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			map[string]any{"error": "internal", "message": "internal server error"})
		return
	}

	c.JSON(http.StatusCreated, StorefrontReviewMediaResponse{
		URL:      m.URL,
		Alt:      m.Alt,
		Position: m.Position,
		Width:    m.Width,
		Height:   m.Height,
	})
}
```

- [ ] **5.6** Verify: `go build ./internal/handlers/storefront/...` compiles cleanly.

---

## Task 6 — Wire routes + main.go

**Files:**
- `services/marketplace-api/internal/handlers/admin/routes.go` (EDIT)
- `services/marketplace-api/internal/handlers/storefront/routes.go` (EDIT)
- `services/marketplace-api/cmd/marketplace-api/main.go` (EDIT)

### Steps

- [ ] **6.1** Add `ReviewHandler *ReviewHandler` to `admin.Deps` in `internal/handlers/admin/routes.go`.

- [ ] **6.2** Register admin review routes in `RegisterAdmin()` (inside the `storeRoute` block, after abandoned carts):

```go
// Reviews — moderation.
if deps.ReviewHandler != nil {
    reviews := storeRoute.Group("/reviews")
    {
        reviews.GET("",
            deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
            deps.ReviewHandler.List)
        reviews.GET("/:id",
            deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
            deps.ReviewHandler.Get)
        reviews.PATCH("/:id",
            deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
            deps.ReviewHandler.Moderate)
        reviews.POST("/:id/reply",
            deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
            deps.ReviewHandler.Reply)
        reviews.DELETE("/:id",
            deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner),
            deps.ReviewHandler.Delete)
    }
}
```

- [ ] **6.3** Add `ReviewsHandler *ReviewsHandler` to `storefront.Deps` in `internal/handlers/storefront/routes.go`.

- [ ] **6.4** Register storefront review routes in `RegisterStorefront()`. Public list (no auth), submit/react/media (auth required):

```go
// Reviews — public list (no auth).
if deps.ReviewsHandler != nil {
    group.GET("/products/:handle/reviews", deps.ReviewsHandler.ListProductReviews)
}

// Reviews — authenticated actions.
// These require the customer auth middleware (OptionalCustomerAuth must be
// in the chain, and these specific routes use RequireCustomerAuth).
if deps.ReviewsHandler != nil {
    authGroup := group.Group("", RequireCustomerAuth())
    {
        authGroup.POST("/products/:handle/reviews", deps.ReviewsHandler.SubmitReview)
        authGroup.POST("/reviews/:id/reaction", deps.ReviewsHandler.React)
        authGroup.POST("/reviews/:id/media", deps.ReviewsHandler.UploadReviewMedia)
    }
}
```

Note: `RequireCustomerAuth` should have been implemented in C1 and be available from the storefront middleware. If the `OptionalCustomerAuth` middleware isn't yet in the chain at the `group` level, add it.

- [ ] **6.5** Wire review dependencies in `main.go` — admin block:

```go
// Reviews wiring (C3).
reviewRepo := review.NewRepository(conn)
reviewSvc := review.NewService(review.Config{
    DB:       conn,
    Repo:     reviewRepo,
    Uploader: uploader,
    Logger:   log,
})
reviewHandler := admin.NewReviewHandler(reviewSvc, reviewRepo, log)
```

Add `ReviewHandler: reviewHandler` to the `adminDeps` struct literal.

- [ ] **6.6** Wire review dependencies in `main.go` — storefront block:

```go
// Reviews wiring (C3).
reviewRepoSF := review.NewRepository(conn)
reviewSvcSF := review.NewService(review.Config{
    DB:       conn,
    Repo:     reviewRepoSF,
    Uploader: uploader, // reuse the same uploader or create storefront-mode one
    Logger:   log,
})
storefrontReviewsHandler := storefront.NewReviewsHandler(reviewSvcSF, reviewRepoSF, productRepoSF, log)
```

Add `ReviewsHandler: storefrontReviewsHandler` to the `storefrontDeps` struct literal.

Note: The storefront block currently has no `uploader` variable. If GCS is needed in storefront mode (for review photo uploads), initialize the uploader in storefront mode too, or move uploader init to a shared scope.

- [ ] **6.7** Add `import "github.com/mark8ly/marketplace-api/internal/review"` to `main.go`.

- [ ] **6.8** Verify: `go build ./cmd/marketplace-api/...` compiles and the server starts with `make dev`.

---

## Task 7 — Admin UI: API client + moderation list page

**Files:**
- `apps/admin/lib/api/marketplace-api.ts` (EDIT — add review API functions)
- `apps/admin/app/reviews/page.tsx` (NEW)
- `apps/admin/components/reviews/ReviewsListHeader.tsx` (NEW)
- `apps/admin/components/reviews/ReviewStatusTabs.tsx` (NEW)
- `apps/admin/components/reviews/ReviewsList.tsx` (NEW)
- `apps/admin/components/reviews/ReviewRow.tsx` (NEW)
- `apps/admin/components/reviews/ReviewExpandedDetail.tsx` (NEW)
- `apps/admin/components/reviews/MerchantReplyForm.tsx` (NEW)
- `apps/admin/components/reviews/ReviewsListEmpty.tsx` (NEW)

### Steps

- [ ] **7.1** Add review API functions to `apps/admin/lib/api/marketplace-api.ts`:

```typescript
// --- Reviews API (C3) ---

export interface AdminReview {
  id: string;
  store_id: string;
  product_id: string;
  customer_profile_id: string | null;
  customer_name: string;
  customer_email: string;
  rating: number;
  title: string | null;
  content: string;
  status: "pending" | "approved" | "rejected";
  verified_purchase: boolean;
  featured: boolean;
  helpful_count: number;
  not_helpful_count: number;
  published_at: string | null;
  created_at: string;
  updated_at: string;
  media: AdminReviewMedia[];
  replies: AdminReviewReply[];
}

export interface AdminReviewMedia {
  id: string;
  url: string;
  alt: string | null;
  position: number;
  media_type: string;
  width: number | null;
  height: number | null;
  file_size: number | null;
}

export interface AdminReviewReply {
  id: string;
  author_type: "merchant" | "customer";
  author_name: string;
  author_email: string | null;
  content: string;
  created_at: string;
}

export interface ListReviewsQuery {
  status?: "pending" | "approved" | "rejected";
  product_id?: string;
  search?: string;
  page?: number;
  pageSize?: number;
}

export async function listReviews(
  storeId: string,
  query: ListReviewsQuery,
  session: SessionHeaders,
): Promise<{ data: AdminReview[]; meta: PaginationMeta } | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.product_id) params.set("product_id", query.product_id);
  if (query.search) params.set("search", query.search);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/reviews${qs ? `?${qs}` : ""}`;
  const res = await fetch(url, { headers: authHeaders(session), next: { revalidate: 0 } });
  if (!res.ok) return null;
  return res.json();
}

export async function moderateReview(
  storeId: string,
  reviewId: string,
  body: { status?: "approved" | "rejected"; featured?: boolean },
  session: SessionHeaders,
): Promise<AdminReview | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/reviews/${reviewId}`;
  const res = await fetch(url, {
    method: "PATCH",
    headers: { ...authHeaders(session), "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) return null;
  return res.json();
}

export async function replyToReview(
  storeId: string,
  reviewId: string,
  content: string,
  session: SessionHeaders,
): Promise<AdminReviewReply | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/reviews/${reviewId}/reply`;
  const res = await fetch(url, {
    method: "POST",
    headers: { ...authHeaders(session), "Content-Type": "application/json" },
    body: JSON.stringify({ content }),
  });
  if (!res.ok) return null;
  return res.json();
}

export async function deleteReview(
  storeId: string,
  reviewId: string,
  session: SessionHeaders,
): Promise<boolean> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/reviews/${reviewId}`;
  const res = await fetch(url, {
    method: "DELETE",
    headers: authHeaders(session),
  });
  return res.ok;
}
```

- [ ] **7.2** Create `apps/admin/app/reviews/page.tsx` — server component following the `ProductsPage` pattern:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listReviews, type ListReviewsQuery } from "@/lib/api/marketplace-api";

import { ReviewsListHeader } from "@/components/reviews/ReviewsListHeader";
import { ReviewStatusTabs } from "@/components/reviews/ReviewStatusTabs";
import { ReviewsList } from "@/components/reviews/ReviewsList";
import { ReviewsListPagination } from "@/components/reviews/ReviewsListPagination";
import { ReviewsListEmpty } from "@/components/reviews/ReviewsListEmpty";

interface ReviewsPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function ReviewsPage({ searchParams }: ReviewsPageProps) {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;
  const params = await searchParams;

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="flex flex-col gap-6 px-8 py-6">
          <ReviewsListHeader />
          <ReviewsListEmpty variant="no-store" />
        </main>
      </AdminShell>
    );
  }

  const query = parseSearchParams(params);
  const response = await listReviews(currentStore.id, query, { userId, tenantId });
  const reviews = response?.data ?? [];
  const meta = response?.meta ?? { page: 1, page_size: 20, total: 0, total_pages: 0 };

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="flex flex-col gap-6 px-8 py-6" aria-labelledby="reviews-heading">
        <ReviewsListHeader />
        <ReviewStatusTabs currentStatus={query.status ?? null} />
        {reviews.length === 0 ? (
          <ReviewsListEmpty variant={query.status ? `no-${query.status}` : "no-reviews"} />
        ) : (
          <>
            <ReviewsList reviews={reviews} storeId={currentStore.id} role={role} />
            {/* Pagination component — same pattern as ProductsListPagination */}
          </>
        )}
      </main>
    </AdminShell>
  );
}

function parseSearchParams(raw: Record<string, string | string[] | undefined>): ListReviewsQuery {
  const status = typeof raw.status === "string" ? raw.status : undefined;
  const search = typeof raw.search === "string" ? raw.search : undefined;
  const page = typeof raw.page === "string" ? parseInt(raw.page, 10) : undefined;
  const validStatus = status === "pending" || status === "approved" || status === "rejected" ? status : undefined;
  return { status: validStatus, search: search || undefined, page: page && page > 0 ? page : undefined };
}
```

- [ ] **7.3** Create `ReviewStatusTabs` component — status tabs (Pending | Approved | Rejected) using Link-based navigation (no client state), styled with moss-700 active indicator:

```tsx
import Link from "next/link";

interface ReviewStatusTabsProps {
  currentStatus: "pending" | "approved" | "rejected" | null;
}

const tabs = [
  { label: "Pending", value: "pending" },
  { label: "Approved", value: "approved" },
  { label: "Rejected", value: "rejected" },
] as const;

export function ReviewStatusTabs({ currentStatus }: ReviewStatusTabsProps) {
  return (
    <nav aria-label="Review status filter" className="flex gap-6 border-b border-[color:var(--ink-900)]/10">
      <Link
        href="/reviews"
        className={`pb-2 text-sm font-medium transition-colors ${
          !currentStatus
            ? "border-b-2 border-[color:var(--moss-700)] text-[color:var(--ink-900)]"
            : "text-[color:var(--ink-900)]/50 hover:text-[color:var(--ink-900)]"
        }`}
      >
        All
      </Link>
      {tabs.map((tab) => (
        <Link
          key={tab.value}
          href={`/reviews?status=${tab.value}`}
          className={`pb-2 text-sm font-medium transition-colors ${
            currentStatus === tab.value
              ? "border-b-2 border-[color:var(--moss-700)] text-[color:var(--ink-900)]"
              : "text-[color:var(--ink-900)]/50 hover:text-[color:var(--ink-900)]"
          }`}
        >
          {tab.label}
        </Link>
      ))}
    </nav>
  );
}
```

- [ ] **7.4** Create `ReviewRow` — each row shows product name, customer, rating stars, excerpt, date, quick approve/reject buttons. Expandable inline to show full content + photos + reply form. Per spec: reply form stays mounted after approve/reject:

```tsx
"use client";

import { useState } from "react";
import { StarRating } from "@/components/reviews/StarRating";
import { MerchantReplyForm } from "@/components/reviews/MerchantReplyForm";
import type { AdminReview } from "@/lib/api/marketplace-api";

interface ReviewRowProps {
  review: AdminReview;
  storeId: string;
  role: string;
}

export function ReviewRow({ review, storeId, role }: ReviewRowProps) {
  const [expanded, setExpanded] = useState(false);
  const canModerate = role === "owner" || role === "admin";

  return (
    <div className="border-b border-[color:var(--ink-900)]/10 py-4">
      <div className="flex items-start justify-between gap-4">
        <button
          type="button"
          onClick={() => setExpanded((prev) => !prev)}
          className="flex-1 text-left"
        >
          <div className="flex items-center gap-3">
            <StarRating rating={review.rating} size="sm" />
            <span className="text-sm font-medium text-[color:var(--ink-900)]">
              {review.customer_name}
            </span>
            {review.verified_purchase && (
              <span className="rounded bg-[color:var(--moss-700)]/10 px-1.5 py-0.5 text-xs font-medium text-[color:var(--moss-700)]">
                Verified
              </span>
            )}
          </div>
          <p className="mt-1 line-clamp-2 text-sm text-[color:var(--ink-900)]/70">
            {review.title ? `${review.title} — ` : ""}
            {review.content}
          </p>
        </button>
        {/* Quick approve/reject buttons */}
        {canModerate && review.status === "pending" && (
          <div className="flex shrink-0 gap-2">
            {/* Server actions or client-side fetch — implementation detail */}
          </div>
        )}
      </div>
      {expanded && (
        <div className="mt-4 pl-4 border-l-2 border-[color:var(--ink-900)]/10">
          {/* Full content, photos grid, reply form */}
          <p className="whitespace-pre-wrap text-sm text-[color:var(--ink-900)]">
            {review.content}
          </p>
          {review.media.length > 0 && (
            <div className="mt-3 flex gap-2">
              {review.media.map((m) => (
                <img key={m.id} src={m.url} alt={m.alt ?? ""} className="h-20 w-20 rounded object-cover" />
              ))}
            </div>
          )}
          {review.featured && (
            <div className="mt-2 border-l-2 border-[color:var(--moss-700)] pl-2">
              <span className="text-xs font-medium text-[color:var(--moss-700)]">Featured</span>
            </div>
          )}
          <MerchantReplyForm
            reviewId={review.id}
            storeId={storeId}
            existingReplies={review.replies}
          />
        </div>
      )}
    </div>
  );
}
```

- [ ] **7.5** Create `StarRating` component — reusable star display using moss-700 filled / ink-900 opacity-15 empty:

```tsx
interface StarRatingProps {
  rating: number;
  size?: "sm" | "md" | "lg";
}

const sizeClasses = { sm: "h-3.5 w-3.5", md: "h-4 w-4", lg: "h-5 w-5" };

export function StarRating({ rating, size = "md" }: StarRatingProps) {
  return (
    <div className="flex gap-0.5" aria-label={`${rating} out of 5 stars`}>
      {Array.from({ length: 5 }, (_, i) => (
        <svg
          key={i}
          viewBox="0 0 20 20"
          fill="currentColor"
          className={`${sizeClasses[size]} ${
            i < rating
              ? "text-[color:var(--moss-700)]"
              : "text-[color:var(--ink-900)] opacity-15"
          }`}
        >
          <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
        </svg>
      ))}
    </div>
  );
}
```

- [ ] **7.6** Create `MerchantReplyForm` — client component with textarea, submit button. Uses server action or client-side `replyToReview`. Form stays mounted after use per spec 10.4.

- [ ] **7.7** Create `ReviewsListEmpty` with per-status empty states:
  - `"no-reviews"` — "No reviews yet. They'll appear here when customers submit feedback."
  - `"no-pending"` — "No pending reviews."
  - `"no-approved"` — "No approved reviews yet."
  - `"no-rejected"` — "No rejected reviews."

- [ ] **7.8** Add review routes to the admin sidebar navigation. Reviews should appear under the "Customers" section.

- [ ] **7.9** Verify: `npm run build` in `apps/admin/` compiles successfully.

---

## Task 8 — Storefront UI: review section on product detail page

**Files:**
- `apps/storefront/lib/api/marketplace-api.ts` (EDIT — add review fetch function)
- `apps/storefront/app/products/[handle]/page.tsx` (EDIT — add ReviewSection)
- `apps/storefront/components/ReviewSection.tsx` (NEW)
- `apps/storefront/components/ReviewSummaryBar.tsx` (NEW)
- `apps/storefront/components/ReviewCard.tsx` (NEW)
- `apps/storefront/components/StarRating.tsx` (NEW)
- `apps/storefront/components/RatingDistribution.tsx` (NEW)

### Steps

- [ ] **8.1** Add review types and fetch function to `apps/storefront/lib/api/marketplace-api.ts`:

```typescript
// --- Reviews (C3) ---

export interface StorefrontReview {
  id: string;
  rating: number;
  title: string | null;
  content: string;
  customer_name: string;
  verified_purchase: boolean;
  featured: boolean;
  helpful_count: number;
  not_helpful_count: number;
  published_at: string | null;
  created_at: string;
  media: { url: string; alt: string | null; position: number; width: number | null; height: number | null }[];
  replies: { author_type: string; author_name: string; content: string; created_at: string }[];
}

export interface ReviewSummary {
  average_rating: number;
  total_count: number;
  distribution: Record<number, number>;
}

export interface ReviewsResponse {
  data: StorefrontReview[];
  summary: ReviewSummary;
  meta: { page: number; page_size: number; total: number; total_pages: number };
}

export async function getProductReviews(
  slug: string,
  handle: string,
  page = 1,
  pageSize = 10,
): Promise<ReviewsResponse | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${slug}/products/${handle}/reviews?page=${page}&page_size=${pageSize}`;
  const res = await fetch(url, { headers: commonHeaders(), next: { revalidate: 0 } });
  if (!res.ok) return null;
  return res.json();
}
```

- [ ] **8.2** Edit `apps/storefront/app/products/[handle]/page.tsx` — add `ReviewSection` below the product grid. Fetch reviews server-side:

```tsx
// Add import at top:
import { getProductReviews } from "@/lib/api/marketplace-api";
import { ReviewSection } from "@/components/ReviewSection";

// Inside StorefrontProductPage, after fetching product:
const reviewsData = await getProductReviews(slug, handle);

// In the JSX, after the product grid div:
<section id="reviews" className="mt-16 border-t border-[color:var(--ink-900)]/10 pt-12">
  <ReviewSection
    reviews={reviewsData?.data ?? []}
    summary={reviewsData?.summary ?? { average_rating: 0, total_count: 0, distribution: {1:0,2:0,3:0,4:0,5:0} }}
    meta={reviewsData?.meta ?? { page: 1, page_size: 10, total: 0, total_pages: 0 }}
    handle={handle}
    slug={slug}
  />
</section>
```

- [ ] **8.3** Create `apps/storefront/components/StarRating.tsx` — same moss-700 / ink-900 opacity-15 pattern as admin, but also supports fractional display for average rating:

```tsx
interface StarRatingProps {
  rating: number;
  size?: "sm" | "md" | "lg";
  showValue?: boolean;
}

export function StarRating({ rating, size = "md", showValue = false }: StarRatingProps) {
  const rounded = Math.round(rating); // snap to nearest int for filled stars
  return (
    <div className="flex items-center gap-1.5">
      <div className="flex gap-0.5" aria-label={`${rating.toFixed(1)} out of 5 stars`}>
        {Array.from({ length: 5 }, (_, i) => (
          <svg
            key={i}
            viewBox="0 0 20 20"
            fill="currentColor"
            className={`${sizeMap[size]} ${
              i < rounded
                ? "text-[color:var(--moss-700)]"
                : "text-[color:var(--ink-900)] opacity-15"
            }`}
          >
            <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
          </svg>
        ))}
      </div>
      {showValue && (
        <span className="text-sm font-medium text-[color:var(--ink-900)]">
          {rating.toFixed(1)}
        </span>
      )}
    </div>
  );
}

const sizeMap = { sm: "h-3.5 w-3.5", md: "h-4 w-4", lg: "h-5 w-5" };
```

- [ ] **8.4** Create `apps/storefront/components/ReviewSummaryBar.tsx` — average rating, total count, and rating distribution bars:

```tsx
import { StarRating } from "./StarRating";
import type { ReviewSummary } from "@/lib/api/marketplace-api";

interface ReviewSummaryBarProps {
  summary: ReviewSummary;
}

export function ReviewSummaryBar({ summary }: ReviewSummaryBarProps) {
  if (summary.total_count === 0) return null;

  const maxCount = Math.max(...Object.values(summary.distribution), 1);

  return (
    <div className="flex gap-12">
      {/* Average rating */}
      <div className="flex flex-col items-center gap-1">
        <span className="font-serif text-4xl font-light text-[color:var(--ink-900)]">
          {summary.average_rating.toFixed(1)}
        </span>
        <StarRating rating={summary.average_rating} size="md" />
        <span className="text-xs text-[color:var(--ink-900)]/50">
          {summary.total_count} {summary.total_count === 1 ? "review" : "reviews"}
        </span>
      </div>

      {/* Distribution bars */}
      <div className="flex flex-1 flex-col gap-1.5">
        {[5, 4, 3, 2, 1].map((star) => {
          const count = summary.distribution[star] ?? 0;
          const pct = (count / maxCount) * 100;
          return (
            <div key={star} className="flex items-center gap-2 text-xs">
              <span className="w-3 text-right text-[color:var(--ink-900)]/50">{star}</span>
              <div className="h-2 flex-1 overflow-hidden rounded-full bg-[color:var(--ink-900)]/5">
                <div
                  className="h-full rounded-full bg-[color:var(--moss-700)]"
                  style={{ width: `${pct}%` }}
                />
              </div>
              <span className="w-6 text-right text-[color:var(--ink-900)]/40">{count}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
```

- [ ] **8.5** Create `apps/storefront/components/ReviewCard.tsx` — individual review display with stars, content, customer name, verified badge, photos, helpful button, and merchant reply:

```tsx
import { StarRating } from "./StarRating";
import type { StorefrontReview } from "@/lib/api/marketplace-api";

interface ReviewCardProps {
  review: StorefrontReview;
}

export function ReviewCard({ review }: ReviewCardProps) {
  return (
    <article className="border-b border-[color:var(--ink-900)]/10 py-6 last:border-b-0">
      <div className="flex items-start justify-between">
        <div>
          <StarRating rating={review.rating} size="sm" />
          {review.title && (
            <h4 className="mt-1 text-sm font-semibold text-[color:var(--ink-900)]">
              {review.title}
            </h4>
          )}
        </div>
        <time className="text-xs text-[color:var(--ink-900)]/40">
          {new Date(review.created_at).toLocaleDateString("en-US", {
            year: "numeric", month: "short", day: "numeric",
          })}
        </time>
      </div>

      <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-[color:var(--ink-900)]/80">
        {review.content}
      </p>

      <div className="mt-2 flex items-center gap-3">
        <span className="text-xs font-medium text-[color:var(--ink-900)]/60">
          {review.customer_name}
        </span>
        {review.verified_purchase && (
          <span className="rounded bg-[color:var(--moss-700)]/10 px-1.5 py-0.5 text-xs font-medium text-[color:var(--moss-700)]">
            Verified purchase
          </span>
        )}
      </div>

      {/* Photos */}
      {review.media.length > 0 && (
        <div className="mt-3 flex gap-2">
          {review.media.map((m, idx) => (
            <img
              key={idx}
              src={m.url}
              alt={m.alt ?? `Review photo ${idx + 1}`}
              className="h-16 w-16 rounded object-cover"
              loading="lazy"
            />
          ))}
        </div>
      )}

      {/* Helpful button */}
      <div className="mt-3">
        <span className="text-xs text-[color:var(--ink-900)]/40">
          {review.helpful_count > 0
            ? `${review.helpful_count} ${review.helpful_count === 1 ? "person" : "people"} found this helpful`
            : "Was this review helpful?"}
        </span>
      </div>

      {/* Merchant replies */}
      {review.replies.map((reply, idx) => (
        <div
          key={idx}
          className="mt-4 ml-4 border-l-2 border-[color:var(--moss-700)]/30 pl-4"
        >
          <span className="text-xs font-semibold text-[color:var(--ink-900)]">
            Store response
          </span>
          <p className="mt-1 whitespace-pre-wrap text-sm text-[color:var(--ink-900)]/70">
            {reply.content}
          </p>
        </div>
      ))}
    </article>
  );
}
```

- [ ] **8.6** Create `apps/storefront/components/ReviewSection.tsx` — orchestrates summary bar, "Write a review" CTA, review list, empty state, and "Load more":

```tsx
import Link from "next/link";
import { ReviewSummaryBar } from "./ReviewSummaryBar";
import { ReviewCard } from "./ReviewCard";
import type { StorefrontReview, ReviewSummary } from "@/lib/api/marketplace-api";

interface ReviewSectionProps {
  reviews: StorefrontReview[];
  summary: ReviewSummary;
  meta: { page: number; page_size: number; total: number; total_pages: number };
  handle: string;
  slug: string;
}

export function ReviewSection({ reviews, summary, meta, handle }: ReviewSectionProps) {
  return (
    <div>
      <div className="flex items-start justify-between">
        <h2 className="font-serif text-2xl font-light text-[color:var(--ink-900)]">
          Reviews
        </h2>
        <Link
          href={`/products/${handle}/review`}
          className="rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--paper-200)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Write a review
        </Link>
      </div>

      {summary.total_count > 0 ? (
        <>
          <div className="mt-8">
            <ReviewSummaryBar summary={summary} />
          </div>
          <div className="mt-8">
            {reviews.map((review) => (
              <ReviewCard key={review.id} review={review} />
            ))}
          </div>
          {meta.page < meta.total_pages && (
            <div className="mt-6 text-center">
              <Link
                href={`?review_page=${meta.page + 1}#reviews`}
                className="text-sm font-medium text-[color:var(--moss-700)] underline underline-offset-4"
              >
                Load more reviews
              </Link>
            </div>
          )}
        </>
      ) : (
        <p className="mt-6 text-sm text-[color:var(--ink-900)]/50">
          No reviews yet. Be the first to share your experience.
        </p>
      )}
    </div>
  );
}
```

- [ ] **8.7** Verify: `npm run build` in `apps/storefront/` compiles. Visually check the product detail page renders the reviews section.

---

## Task 9 — Storefront UI: review submission page

**Files:**
- `apps/storefront/app/products/[handle]/review/page.tsx` (NEW)
- `apps/storefront/components/StarPicker.tsx` (NEW)
- `apps/storefront/components/ReviewPhotoUpload.tsx` (NEW)

### Steps

- [ ] **9.1** Create `apps/storefront/components/StarPicker.tsx` — interactive star picker (1-5, click to select):

```tsx
"use client";

import { useState } from "react";

interface StarPickerProps {
  value: number;
  onChange: (rating: number) => void;
}

export function StarPicker({ value, onChange }: StarPickerProps) {
  const [hovered, setHovered] = useState(0);

  return (
    <div
      className="flex gap-1"
      role="radiogroup"
      aria-label="Rating"
      onMouseLeave={() => setHovered(0)}
    >
      {[1, 2, 3, 4, 5].map((star) => (
        <button
          key={star}
          type="button"
          role="radio"
          aria-checked={value === star}
          aria-label={`${star} star${star !== 1 ? "s" : ""}`}
          onMouseEnter={() => setHovered(star)}
          onClick={() => onChange(star)}
          className="focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[color:var(--moss-700)] rounded"
        >
          <svg
            viewBox="0 0 20 20"
            fill="currentColor"
            className={`h-8 w-8 transition-colors ${
              star <= (hovered || value)
                ? "text-[color:var(--moss-700)]"
                : "text-[color:var(--ink-900)] opacity-15"
            }`}
          >
            <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
          </svg>
        </button>
      ))}
    </div>
  );
}
```

- [ ] **9.2** Create `apps/storefront/components/ReviewPhotoUpload.tsx` — drag/drop or click, max 3 images, 5MB each. Shows previews. Uses the existing GCS signed upload URL flow:

```tsx
"use client";

import { useState, useRef, useCallback } from "react";

interface ReviewPhotoUploadProps {
  photos: File[];
  onPhotosChange: (photos: File[]) => void;
  maxPhotos?: number;
}

export function ReviewPhotoUpload({
  photos,
  onPhotosChange,
  maxPhotos = 3,
}: ReviewPhotoUploadProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragActive, setDragActive] = useState(false);

  const addFiles = useCallback(
    (files: FileList | null) => {
      if (!files) return;
      const valid = Array.from(files)
        .filter((f) => f.type.startsWith("image/") && f.size <= 5 * 1024 * 1024)
        .slice(0, maxPhotos - photos.length);
      if (valid.length > 0) {
        onPhotosChange([...photos, ...valid]);
      }
    },
    [photos, onPhotosChange, maxPhotos],
  );

  const removePhoto = (index: number) => {
    onPhotosChange(photos.filter((_, i) => i !== index));
  };

  return (
    <div>
      {photos.length < maxPhotos && (
        <div
          onDragOver={(e) => { e.preventDefault(); setDragActive(true); }}
          onDragLeave={() => setDragActive(false)}
          onDrop={(e) => { e.preventDefault(); setDragActive(false); addFiles(e.dataTransfer.files); }}
          onClick={() => inputRef.current?.click()}
          className={`cursor-pointer rounded-md border-2 border-dashed px-6 py-4 text-center text-sm transition-colors ${
            dragActive
              ? "border-[color:var(--moss-700)] bg-[color:var(--moss-700)]/5"
              : "border-[color:var(--ink-900)]/15 hover:border-[color:var(--ink-900)]/30"
          }`}
        >
          <p className="text-[color:var(--ink-900)]/50">
            Drop photos here or click to browse
          </p>
          <p className="mt-1 text-xs text-[color:var(--ink-900)]/30">
            Max {maxPhotos} images, 5MB each
          </p>
          <input
            ref={inputRef}
            type="file"
            accept="image/*"
            multiple
            className="hidden"
            onChange={(e) => addFiles(e.target.files)}
          />
        </div>
      )}
      {photos.length > 0 && (
        <div className="mt-3 flex gap-2">
          {photos.map((photo, idx) => (
            <div key={idx} className="group relative">
              <img
                src={URL.createObjectURL(photo)}
                alt={`Upload ${idx + 1}`}
                className="h-16 w-16 rounded object-cover"
              />
              <button
                type="button"
                onClick={() => removePhoto(idx)}
                className="absolute -right-1 -top-1 flex h-5 w-5 items-center justify-center rounded-full bg-[color:var(--ink-900)] text-xs text-white opacity-0 transition-opacity group-hover:opacity-100"
                aria-label={`Remove photo ${idx + 1}`}
              >
                x
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
```

- [ ] **9.3** Create `apps/storefront/app/products/[handle]/review/page.tsx` — the review submission page. Auth gate: if not authenticated, redirect to auth-bff login with `redirect_uri=/products/[handle]/review`:

```tsx
import { headers } from "next/headers";
import { redirect, notFound } from "next/navigation";
import Link from "next/link";

import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { getProductByHandle } from "@/lib/api/marketplace-api";
import { slugFromHost } from "@/lib/slug";
import { getCustomerSession } from "@/lib/auth/customerSession";
import { StorefrontNav } from "@/components/StorefrontNav";
import { ReviewSubmitForm } from "@/components/ReviewSubmitForm";

export const dynamic = "force-dynamic";

interface PageProps {
  params: Promise<{ handle: string }>;
  searchParams: Promise<{ slug?: string }>;
}

export default async function ReviewSubmitPage({ params, searchParams }: PageProps) {
  const { handle } = await params;
  const sp = await searchParams;
  const h = await headers();
  const host = h.get("host");
  const slug = sp.slug || slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "";

  const store = slug ? await fetchStoreBySlug(slug) : null;
  if (!store) notFound();

  const product = await getProductByHandle(slug, handle);
  if (!product) notFound();

  // Auth gate — redirect to auth-bff if not authenticated.
  const session = await getCustomerSession();
  if (!session) {
    const returnUrl = `/products/${handle}/review`;
    const loginUrl = `${process.env.AUTH_BFF_URL}/login?product=mp-customer&redirect_uri=${encodeURIComponent(returnUrl)}`;
    redirect(loginUrl);
  }

  return (
    <main id="main" className="min-h-screen bg-[color:var(--paper-200)]">
      <div className="mx-auto max-w-2xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store.name} />
        <Link
          href={`/products/${handle}`}
          className="mb-8 inline-flex items-center gap-1 text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--ink-900)] opacity-60 transition-opacity hover:opacity-100"
        >
          ← Back to {product.title}
        </Link>

        <h1 className="font-serif text-2xl font-light text-[color:var(--ink-900)]">
          Write a review
        </h1>
        <p className="mt-1 text-sm text-[color:var(--ink-900)]/50">
          Share your experience with {product.title}
        </p>

        <div className="mt-8">
          <ReviewSubmitForm handle={handle} slug={slug} productTitle={product.title} />
        </div>
      </div>
    </main>
  );
}
```

- [ ] **9.4** Create `apps/storefront/components/ReviewSubmitForm.tsx` — client component with StarPicker, title input, content textarea, ReviewPhotoUpload, and submit button:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { StarPicker } from "./StarPicker";
import { ReviewPhotoUpload } from "./ReviewPhotoUpload";

interface ReviewSubmitFormProps {
  handle: string;
  slug: string;
  productTitle: string;
}

export function ReviewSubmitForm({ handle, slug, productTitle }: ReviewSubmitFormProps) {
  const router = useRouter();
  const [rating, setRating] = useState(0);
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [photos, setPhotos] = useState<File[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canSubmit = rating >= 1 && content.length >= 20 && !submitting;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;

    setSubmitting(true);
    setError(null);

    try {
      // 1. Submit the review text.
      const res = await fetch(
        `/api/reviews/submit?slug=${slug}&handle=${handle}`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ rating, title: title || undefined, content }),
        },
      );

      if (!res.ok) {
        const body = await res.json().catch(() => null);
        setError(body?.message ?? "Failed to submit review. Please try again.");
        setSubmitting(false);
        return;
      }

      // 2. Upload photos if any (via the review media endpoint).
      // Photo upload implementation would use signed upload URLs,
      // then POST each storage_key to the review media endpoint.

      // 3. Redirect back to product page with success.
      router.push(`/products/${handle}?review_submitted=1`);
    } catch {
      setError("Something went wrong. Please try again.");
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-6">
      {/* Rating */}
      <div>
        <label className="mb-2 block text-sm font-medium text-[color:var(--ink-900)]">
          Rating
        </label>
        <StarPicker value={rating} onChange={setRating} />
        {rating === 0 && (
          <p className="mt-1 text-xs text-[color:var(--ink-900)]/40">
            Click a star to rate
          </p>
        )}
      </div>

      {/* Title (optional) */}
      <div>
        <label htmlFor="review-title" className="mb-1 block text-sm font-medium text-[color:var(--ink-900)]">
          Title <span className="text-[color:var(--ink-900)]/30">(optional)</span>
        </label>
        <input
          id="review-title"
          type="text"
          maxLength={300}
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          className="w-full rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
          placeholder="Summarize your experience"
        />
      </div>

      {/* Content */}
      <div>
        <label htmlFor="review-content" className="mb-1 block text-sm font-medium text-[color:var(--ink-900)]">
          Review
        </label>
        <textarea
          id="review-content"
          rows={5}
          minLength={20}
          maxLength={5000}
          value={content}
          onChange={(e) => setContent(e.target.value)}
          className="w-full rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
          placeholder="Tell others what you think about this product (min 20 characters)"
          required
        />
        <p className="mt-1 text-right text-xs text-[color:var(--ink-900)]/30">
          {content.length}/5000
        </p>
      </div>

      {/* Photos */}
      <div>
        <label className="mb-2 block text-sm font-medium text-[color:var(--ink-900)]">
          Photos <span className="text-[color:var(--ink-900)]/30">(optional)</span>
        </label>
        <ReviewPhotoUpload photos={photos} onPhotosChange={setPhotos} />
      </div>

      {/* Error */}
      {error && (
        <p className="text-sm text-[color:var(--signal)]" role="alert">
          {error}
        </p>
      )}

      {/* Submit */}
      <button
        type="submit"
        disabled={!canSubmit}
        className="w-full rounded-md bg-[color:var(--ink-900)] px-4 py-3 text-sm font-medium text-[color:var(--paper-200)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40 disabled:cursor-not-allowed"
      >
        {submitting ? "Submitting..." : "Submit review"}
      </button>
    </form>
  );
}
```

- [ ] **9.5** Create `apps/storefront/app/api/reviews/submit/route.ts` — Next.js API route that proxies the review submission to marketplace-api, forwarding the customer auth cookie:

```typescript
import { NextRequest, NextResponse } from "next/server";

const MARKETPLACE_API_URL = process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

export async function POST(request: NextRequest) {
  const { searchParams } = request.nextUrl;
  const slug = searchParams.get("slug");
  const handle = searchParams.get("handle");

  if (!slug || !handle) {
    return NextResponse.json({ error: "missing_params" }, { status: 400 });
  }

  const body = await request.json();

  // Forward the auth cookie to marketplace-api.
  const cookieHeader = request.headers.get("cookie") ?? "";
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Cookie: cookieHeader,
  };
  if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;

  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${slug}/products/${handle}/reviews`,
    { method: "POST", headers, body: JSON.stringify(body) },
  );

  const data = await res.json().catch(() => null);
  return NextResponse.json(data, { status: res.status });
}
```

- [ ] **9.6** Verify: `npm run build` in `apps/storefront/` compiles. Navigate to `/products/[handle]/review` — should redirect to login if unauthenticated, show form if authenticated.

---

## Task 10 — Build verification

### Steps

- [ ] **10.1** Run the Go build for the entire marketplace-api:
  ```
  cd services/marketplace-api && go build ./...
  ```

- [ ] **10.2** Run the admin Next.js build:
  ```
  cd apps/admin && npm run build
  ```

- [ ] **10.3** Run the storefront Next.js build:
  ```
  cd apps/storefront && npm run build
  ```

- [ ] **10.4** Run `make mp-migrate-up` (or equivalent) and verify all tables are created.

- [ ] **10.5** Start the dev server (`make dev` or equivalent) and verify:
  - `GET /api/v1/admin/stores/:storeId/reviews` returns `{"data":[],"meta":{...}}`
  - `GET /api/v1/storefront/stores/:slug/products/:handle/reviews` returns `{"data":[],"summary":{...},"meta":{...}}`
  - `POST /api/v1/storefront/stores/:slug/products/:handle/reviews` returns 401 without auth

- [ ] **10.6** Verify all four UNIQUE constraints are enforced:
  - Attempt to insert two reviews with the same (store_id, product_id, customer_email) — second should fail
  - Attempt to insert two reactions with the same (review_id, customer_profile_id) — should be handled by toggle logic

- [ ] **10.7** Run `go vet ./...` and `go test ./...` from the `services/marketplace-api/` root.

- [ ] **10.8** Run TypeScript checks: `npx tsc --noEmit` in both `apps/admin/` and `apps/storefront/`.

---

## File Inventory

### New files (Go — 6 files)
| File | Purpose |
|------|---------|
| `services/marketplace-api/migrations/000014_reviews.up.sql` | Schema for reviews, media, replies, reactions |
| `services/marketplace-api/migrations/000014_reviews.down.sql` | Rollback migration |
| `services/marketplace-api/internal/review/models.go` | GORM structs for all 4 review tables |
| `services/marketplace-api/internal/review/repository.go` | Data access with FOR UPDATE, atomic reactions |
| `services/marketplace-api/internal/review/service.go` | Business logic, sanitization, verified purchase |
| `services/marketplace-api/internal/review/sanitizer.go` | Plain-text bluemonday policy for review content |

### New files (Go handlers — 4 files)
| File | Purpose |
|------|---------|
| `services/marketplace-api/internal/handlers/admin/reviews.go` | Admin moderation handlers |
| `services/marketplace-api/internal/handlers/admin/review_dto.go` | Admin review DTOs |
| `services/marketplace-api/internal/handlers/storefront/reviews.go` | Storefront review handlers |
| `services/marketplace-api/internal/handlers/storefront/review_dto.go` | Storefront review DTOs (no email leak) |

### New files (Next.js admin — ~8 files)
| File | Purpose |
|------|---------|
| `apps/admin/app/reviews/page.tsx` | Moderation list page |
| `apps/admin/components/reviews/ReviewsListHeader.tsx` | Page header |
| `apps/admin/components/reviews/ReviewStatusTabs.tsx` | Status tab navigation |
| `apps/admin/components/reviews/ReviewsList.tsx` | Reviews list wrapper |
| `apps/admin/components/reviews/ReviewRow.tsx` | Expandable review row |
| `apps/admin/components/reviews/MerchantReplyForm.tsx` | Reply form |
| `apps/admin/components/reviews/ReviewsListEmpty.tsx` | Empty states |
| `apps/admin/components/reviews/StarRating.tsx` | Star display (moss/ink) |

### New files (Next.js storefront — ~8 files)
| File | Purpose |
|------|---------|
| `apps/storefront/app/products/[handle]/review/page.tsx` | Review submission page |
| `apps/storefront/app/api/reviews/submit/route.ts` | API proxy for review submission |
| `apps/storefront/components/ReviewSection.tsx` | Reviews section on product detail |
| `apps/storefront/components/ReviewSummaryBar.tsx` | Average + distribution bars |
| `apps/storefront/components/ReviewCard.tsx` | Individual review display |
| `apps/storefront/components/StarRating.tsx` | Star display (moss/ink) |
| `apps/storefront/components/StarPicker.tsx` | Interactive star picker |
| `apps/storefront/components/ReviewPhotoUpload.tsx` | Drag/drop photo upload |
| `apps/storefront/components/ReviewSubmitForm.tsx` | Full submission form |

### Modified files (~6 files)
| File | Change |
|------|--------|
| `services/marketplace-api/migrations.go` | Bump `ExpectedSchemaVersion` |
| `services/marketplace-api/cmd/marketplace-api/main.go` | Wire review deps for admin + storefront |
| `services/marketplace-api/internal/handlers/admin/routes.go` | Add ReviewHandler to Deps + routes |
| `services/marketplace-api/internal/handlers/storefront/routes.go` | Add ReviewsHandler to Deps + routes |
| `services/marketplace-api/pkg/apperrors/codes.go` | Add `CodeDuplicateReview` |
| `apps/storefront/app/products/[handle]/page.tsx` | Add ReviewSection below product |
| `apps/admin/lib/api/marketplace-api.ts` | Add review API functions |
| `apps/storefront/lib/api/marketplace-api.ts` | Add review fetch function |
