// Package review provides the domain model, repository, and service for
// product reviews, review media, replies, and reactions.
package review

import (
	"time"
)

// Review status constants.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

// Reaction type constants.
const (
	ReactionHelpful    = "helpful"
	ReactionNotHelpful = "not_helpful"
)

// Review maps to the reviews table.
type Review struct {
	ID                string     `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID          string     `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID           string     `gorm:"column:store_id;type:uuid;not null"`
	ProductID         string     `gorm:"column:product_id;type:uuid;not null"`
	CustomerProfileID *string    `gorm:"column:customer_profile_id;type:uuid"`
	CustomerName      string     `gorm:"column:customer_name;type:varchar(200);not null"`
	CustomerEmail     string     `gorm:"column:customer_email;type:varchar(300);not null"`
	Rating            int        `gorm:"column:rating;type:int;not null"`
	Title             *string    `gorm:"column:title;type:varchar(300)"`
	Content           string     `gorm:"column:content;type:text;not null"`
	Status            string     `gorm:"column:status;type:varchar(20);not null;default:pending"`
	VerifiedPurchase  bool       `gorm:"column:verified_purchase;type:boolean;not null;default:false"`
	Featured          bool       `gorm:"column:featured;type:boolean;not null;default:false"`
	HelpfulCount      int        `gorm:"column:helpful_count;type:int;not null;default:0"`
	NotHelpfulCount   int        `gorm:"column:not_helpful_count;type:int;not null;default:0"`
	PublishedAt       *time.Time `gorm:"column:published_at"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null;default:now()"`

	// Preloaded associations.
	Media   []ReviewMedia `gorm:"foreignKey:ReviewID"`
	Replies []ReviewReply `gorm:"foreignKey:ReviewID"`
}

func (Review) TableName() string { return "reviews" }

// ReviewMedia maps to the review_media table.
type ReviewMedia struct {
	ID        string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ReviewID  string    `gorm:"column:review_id;type:uuid;not null"`
	URL       string    `gorm:"column:url;type:text;not null"`
	Alt       *string   `gorm:"column:alt;type:varchar(300)"`
	Position  int       `gorm:"column:position;type:int;not null;default:0"`
	MediaType string    `gorm:"column:media_type;type:varchar(20);not null;default:image"`
	Width     *int      `gorm:"column:width;type:int"`
	Height    *int      `gorm:"column:height;type:int"`
	FileSize  *int      `gorm:"column:file_size;type:int"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"`
}

func (ReviewMedia) TableName() string { return "review_media" }

// ReviewReply maps to the review_replies table.
type ReviewReply struct {
	ID          string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ReviewID    string    `gorm:"column:review_id;type:uuid;not null"`
	AuthorType  string    `gorm:"column:author_type;type:varchar(20);not null"`
	AuthorName  string    `gorm:"column:author_name;type:varchar(200);not null"`
	AuthorEmail *string   `gorm:"column:author_email;type:varchar(300)"`
	Content     string    `gorm:"column:content;type:text;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;default:now()"`
}

func (ReviewReply) TableName() string { return "review_replies" }

// ReviewReaction maps to the review_reactions table.
type ReviewReaction struct {
	ID                string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ReviewID          string    `gorm:"column:review_id;type:uuid;not null"`
	CustomerProfileID string    `gorm:"column:customer_profile_id;type:uuid;not null"`
	Reaction          string    `gorm:"column:reaction;type:varchar(20);not null"`
	CreatedAt         time.Time `gorm:"column:created_at;not null;default:now()"`
}

func (ReviewReaction) TableName() string { return "review_reactions" }

// SubmitReviewInput is the service-layer input for submitting a review.
type SubmitReviewInput struct {
	TenantID          string
	StoreID           string
	ProductID         string
	CustomerProfileID string
	CustomerName      string
	CustomerEmail     string
	Rating            int
	Title             string
	Content           string
}
