package loyalty

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

// --- Constants ---

type TransactionType string

const (
	TxTypeEarn     TransactionType = "earn"
	TxTypeRedeem   TransactionType = "redeem"
	TxTypeExpire   TransactionType = "expire"
	TxTypeAdjust   TransactionType = "adjust"
	TxTypeSignup   TransactionType = "signup"
	TxTypeReferral TransactionType = "referral"
)

type ReferralStatus string

const (
	ReferralStatusPending   ReferralStatus = "pending"
	ReferralStatusCompleted ReferralStatus = "completed"
	ReferralStatusExpired   ReferralStatus = "expired"
)

// --- Tier ---

// Tier is the Go representation of one element inside
// loyalty_programs.tiers (JSONB). Validated at the service layer
// before write — never trust the DB contents blindly.
type Tier struct {
	Name       string          `json:"name"`
	MinPoints  int             `json:"min_points"`
	Multiplier decimal.Decimal `json:"multiplier"`
}

// --- GORM Models ---

// LoyaltyProgram is the per-store configuration for the loyalty feature.
// Exactly one row per store_id (UNIQUE constraint).
type LoyaltyProgram struct {
	ID              uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID        uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID         uuid.UUID       `gorm:"column:store_id;type:uuid;not null"`
	IsActive        bool            `gorm:"column:is_active;not null;default:false"`
	PointsPerUnit   decimal.Decimal `gorm:"column:points_per_unit;type:numeric(5,2);not null;default:1.00"`
	PointsCurrency  string          `gorm:"column:points_currency;type:varchar(20);not null;default:'points'"`
	SignupBonus     int             `gorm:"column:signup_bonus;not null;default:0"`
	ReferralBonus   int             `gorm:"column:referral_bonus;not null;default:0"`
	RefereeBonus    int             `gorm:"column:referee_bonus;not null;default:0"`
	PointExpiryDays *int            `gorm:"column:point_expiry_days"`
	MinRedeemPoints int             `gorm:"column:min_redeem_points;not null;default:100"`
	PointsValue     decimal.Decimal `gorm:"column:points_value;type:numeric(8,4);not null;default:0.01"`
	Tiers           datatypes.JSON  `gorm:"column:tiers;type:jsonb;not null;default:'[]'::jsonb"`
	CreatedAt       time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;not null;default:now()"`
}

func (LoyaltyProgram) TableName() string { return "loyalty_programs" }

// CustomerLoyalty is a customer's enrollment + running balance.
type CustomerLoyalty struct {
	ID             uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID       uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID        uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	CustomerEmail  string     `gorm:"column:customer_email;type:varchar(300);not null"`
	CustomerName   *string    `gorm:"column:customer_name;type:varchar(200)"`
	PointsBalance  int        `gorm:"column:points_balance;not null;default:0"`
	LifetimePoints int        `gorm:"column:lifetime_points;not null;default:0"`
	Tier           string     `gorm:"column:tier;type:varchar(50);not null;default:'bronze'"`
	ReferralCode   string     `gorm:"column:referral_code;type:varchar(20);not null"`
	ReferredBy     *uuid.UUID `gorm:"column:referred_by;type:uuid"`
	EnrolledAt     time.Time  `gorm:"column:enrolled_at;not null;default:now()"`
}

func (CustomerLoyalty) TableName() string { return "customer_loyalties" }

// LoyaltyTransaction is an append-only ledger of point changes.
type LoyaltyTransaction struct {
	ID           uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID     uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	LoyaltyID    uuid.UUID       `gorm:"column:loyalty_id;type:uuid;not null"`
	OrderID      *uuid.UUID      `gorm:"column:order_id;type:uuid"`
	Type         TransactionType `gorm:"column:type;type:varchar(20);not null"`
	Points       int             `gorm:"column:points;not null"`
	BalanceAfter int             `gorm:"column:balance_after;not null"`
	Description  *string         `gorm:"column:description;type:varchar(200)"`
	AdjustedBy   *string         `gorm:"column:adjusted_by;type:varchar(200)"`
	CreatedAt    time.Time       `gorm:"column:created_at;not null;default:now()"`
}

func (LoyaltyTransaction) TableName() string { return "loyalty_transactions" }

// Referral tracks who referred whom.
type Referral struct {
	ID            uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID      uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID       uuid.UUID      `gorm:"column:store_id;type:uuid;not null"`
	ReferrerID    uuid.UUID      `gorm:"column:referrer_id;type:uuid;not null"`
	RefereeID     uuid.UUID      `gorm:"column:referee_id;type:uuid;not null"`
	Status        ReferralStatus `gorm:"column:status;type:varchar(20);not null;default:'pending'"`
	ReferrerBonus int            `gorm:"column:referrer_bonus;not null;default:0"`
	RefereeBonus  int            `gorm:"column:referee_bonus;not null;default:0"`
	CompletedAt   *time.Time     `gorm:"column:completed_at"`
	CreatedAt     time.Time      `gorm:"column:created_at;not null;default:now()"`
}

func (Referral) TableName() string { return "referrals" }
