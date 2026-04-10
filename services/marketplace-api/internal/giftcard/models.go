// Package giftcard implements the gift card domain: models, repository,
// service layer with crypto/rand code generation and atomic balance debit.
package giftcard

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// GiftCardStatus enumerates the lifecycle states of a gift card.
type GiftCardStatus string

const (
	StatusActive   GiftCardStatus = "active"
	StatusDisabled GiftCardStatus = "disabled"
	StatusDepleted GiftCardStatus = "depleted"
)

// TransactionType enumerates the types of balance mutations.
type TransactionType string

const (
	TxnPurchase   TransactionType = "purchase"
	TxnRedeem     TransactionType = "redeem"
	TxnRefund     TransactionType = "refund"
	TxnAdjustment TransactionType = "adjustment"
)

// GiftCard is the GORM model for the gift_cards table.
type GiftCard struct {
	ID             uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID       uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID        uuid.UUID       `gorm:"column:store_id;type:uuid;not null"`
	Code           string          `gorm:"column:code;type:varchar(50);not null"`
	InitialBalance decimal.Decimal `gorm:"column:initial_balance;type:numeric(12,2);not null"`
	CurrentBalance decimal.Decimal `gorm:"column:current_balance;type:numeric(12,2);not null"`
	CurrencyCode   string          `gorm:"column:currency_code;type:char(3);not null"`
	Status         GiftCardStatus  `gorm:"column:status;type:varchar(20);not null;default:active"`
	SenderName     *string         `gorm:"column:sender_name;type:varchar(200)"`
	SenderEmail    *string         `gorm:"column:sender_email;type:varchar(300)"`
	RecipientName  *string         `gorm:"column:recipient_name;type:varchar(200)"`
	RecipientEmail *string         `gorm:"column:recipient_email;type:varchar(300)"`
	Message        *string         `gorm:"column:message;type:text"`
	PurchasedAt    *time.Time      `gorm:"column:purchased_at"`
	ExpiresAt      *time.Time      `gorm:"column:expires_at"`
	CreatedAt      time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt      time.Time       `gorm:"column:updated_at;not null;default:now()"`
}

func (GiftCard) TableName() string { return "gift_cards" }

// Transaction is the GORM model for the gift_card_transactions table.
type Transaction struct {
	ID           uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID     uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	GiftCardID   uuid.UUID       `gorm:"column:gift_card_id;type:uuid;not null"`
	OrderID      *uuid.UUID      `gorm:"column:order_id;type:uuid"`
	Type         TransactionType `gorm:"column:type;type:varchar(20);not null"`
	Amount       decimal.Decimal `gorm:"column:amount;type:numeric(12,2);not null"`
	BalanceAfter decimal.Decimal `gorm:"column:balance_after;type:numeric(12,2);not null"`
	Note         *string         `gorm:"column:note;type:varchar(200)"`
	CreatedAt    time.Time       `gorm:"column:created_at;not null;default:now()"`
}

func (Transaction) TableName() string { return "gift_card_transactions" }
