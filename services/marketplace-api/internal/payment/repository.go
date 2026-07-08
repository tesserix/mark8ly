package payment

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---------------------------------------------------------------------------
// GORM Models
// ---------------------------------------------------------------------------

// PaymentTransaction represents a row in the payment_transactions table.
type PaymentTransaction struct {
	ID               string          `gorm:"column:id;type:uuid;primaryKey"           json:"id"`
	TenantID         string          `gorm:"column:tenant_id;type:uuid;not null;index" json:"tenant_id"`
	StoreID          string          `gorm:"column:store_id;type:uuid;not null;index"  json:"store_id"`
	OrderID          string          `gorm:"column:order_id;type:uuid;not null;index"  json:"order_id"`
	Provider         string          `gorm:"column:provider;type:varchar(20);not null"  json:"provider"`
	ProviderIntentID string          `gorm:"column:provider_intent_id;type:varchar(255);not null" json:"provider_intent_id"`
	Amount           decimal.Decimal `gorm:"column:amount;type:numeric(12,2);not null" json:"amount"`
	CurrencyCode     string          `gorm:"column:currency_code;type:char(3);not null" json:"currency_code"`
	Status           string          `gorm:"column:status;type:varchar(30);not null"    json:"status"`
	PaymentMethod    string          `gorm:"column:payment_method;type:varchar(50)"     json:"payment_method,omitempty"`
	CreatedAt        time.Time       `gorm:"column:created_at;autoCreateTime"           json:"created_at"`
	UpdatedAt        time.Time       `gorm:"column:updated_at;autoUpdateTime"           json:"updated_at"`
}

func (PaymentTransaction) TableName() string { return "payment_transactions" }

func (p *PaymentTransaction) BeforeCreate(_ *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

// RefundTransaction represents a row in the refund_transactions table.
type RefundTransaction struct {
	ID                string          `gorm:"column:id;type:uuid;primaryKey"                json:"id"`
	TenantID          string          `gorm:"column:tenant_id;type:uuid;not null;index"      json:"tenant_id"`
	StoreID           string          `gorm:"column:store_id;type:uuid;not null;index"       json:"store_id"`
	OrderID           string          `gorm:"column:order_id;type:uuid;index"                json:"order_id"`
	IdempotencyKey    string          `gorm:"column:idempotency_key;type:varchar(255);uniqueIndex" json:"idempotency_key"`
	ProviderPaymentID string          `gorm:"column:provider_payment_id;type:varchar(255);not null" json:"provider_payment_id"`
	ProviderRefundID  string          `gorm:"column:provider_refund_id;type:varchar(255);not null"  json:"provider_refund_id"`
	Provider          string          `gorm:"column:provider;type:varchar(20);not null"       json:"provider"`
	Amount            decimal.Decimal `gorm:"column:amount;type:numeric(12,2);not null"       json:"amount"`
	Reason            string          `gorm:"column:reason;type:text"                         json:"reason,omitempty"`
	Status            string          `gorm:"column:status;type:varchar(30);not null"          json:"status"`
	CreatedAt         time.Time       `gorm:"column:created_at;autoCreateTime"                 json:"created_at"`
	UpdatedAt         time.Time       `gorm:"column:updated_at;autoUpdateTime"                 json:"updated_at"`
}

func (RefundTransaction) TableName() string { return "refund_transactions" }

func (r *RefundTransaction) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	return nil
}

// WebhookEventRecord represents a row in the webhook_events table.
type WebhookEventRecord struct {
	ID              string    `gorm:"column:id;type:uuid;primaryKey"                       json:"id"`
	TenantID        string    `gorm:"column:tenant_id;type:uuid;not null;index"             json:"tenant_id"`
	StoreID         string    `gorm:"column:store_id;type:uuid;not null;index"              json:"store_id"`
	Provider        string    `gorm:"column:provider;type:varchar(20);not null"              json:"provider"`
	ProviderEventID string    `gorm:"column:provider_event_id;type:varchar(255);not null;uniqueIndex" json:"provider_event_id"`
	EventType       string    `gorm:"column:event_type;type:varchar(50);not null"            json:"event_type"`
	OrderID         string    `gorm:"column:order_id;type:uuid"                              json:"order_id,omitempty"`
	RawPayload      []byte    `gorm:"column:raw_payload;type:jsonb"                          json:"-"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"                       json:"created_at"`
}

func (WebhookEventRecord) TableName() string { return "webhook_events" }

func (w *WebhookEventRecord) BeforeCreate(_ *gorm.DB) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Repository interface + GORM implementation
// ---------------------------------------------------------------------------

// Repository defines data access for payment-related tables.
type Repository interface {
	CreateTransaction(ctx context.Context, tx *PaymentTransaction) error
	GetTransactionByOrderID(ctx context.Context, orderID string) (*PaymentTransaction, error)
	UpdateTransactionStatus(ctx context.Context, orderID, status string) error

	CreateRefundTransaction(ctx context.Context, refund *RefundTransaction) error
	ListRefundsByPaymentID(ctx context.Context, providerPaymentID string) ([]RefundTransaction, error)

	// InsertRefundPending inserts a pending refund row inside tx. Returns
	// (row, created). created=false ⇒ a row with this idempotency_key already
	// existed (returned instead) — the saga re-entry guard.
	InsertRefundPending(tx *gorm.DB, r *RefundTransaction) (*RefundTransaction, bool, error)
	// UpdateRefundOutcome sets status + provider_refund_id on a ledger row.
	UpdateRefundOutcome(tx *gorm.DB, ledgerID, providerRefundID, status string) error
	// GetRefundByIdempotencyKey reads a ledger row by its unique key.
	GetRefundByIdempotencyKey(ctx context.Context, key string) (*RefundTransaction, error)

	CreateWebhookEvent(ctx context.Context, evt *WebhookEventRecord) error
	GetWebhookEventByProviderID(ctx context.Context, providerEventID string) (*WebhookEventRecord, error)
}

type gormRepository struct{ db *gorm.DB }

// NewRepository returns a GORM-backed payment Repository.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

// ---------------------------------------------------------------------------
// PaymentTransaction CRUD
// ---------------------------------------------------------------------------

func (r *gormRepository) CreateTransaction(ctx context.Context, tx *PaymentTransaction) error {
	return r.db.WithContext(ctx).Create(tx).Error
}

func (r *gormRepository) GetTransactionByOrderID(ctx context.Context, orderID string) (*PaymentTransaction, error) {
	var tx PaymentTransaction
	err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Order("created_at DESC").
		First(&tx).Error
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (r *gormRepository) UpdateTransactionStatus(ctx context.Context, orderID, status string) error {
	return r.db.WithContext(ctx).
		Model(&PaymentTransaction{}).
		Where("order_id = ?", orderID).
		Update("status", status).Error
}

// ---------------------------------------------------------------------------
// RefundTransaction CRUD
// ---------------------------------------------------------------------------

func (r *gormRepository) CreateRefundTransaction(ctx context.Context, refund *RefundTransaction) error {
	return r.db.WithContext(ctx).Create(refund).Error
}

func (r *gormRepository) ListRefundsByPaymentID(ctx context.Context, providerPaymentID string) ([]RefundTransaction, error) {
	var rows []RefundTransaction
	err := r.db.WithContext(ctx).
		Where("provider_payment_id = ?", providerPaymentID).
		Order("created_at DESC").
		Find(&rows).Error
	return rows, err
}

// InsertRefundPending inserts a pending refund row inside tx, guarding
// against duplicate saga re-entry via a unique idempotency_key. If a row
// with the same key already exists, ON CONFLICT DO NOTHING skips the
// insert and the existing row is re-selected and returned with created=false.
func (r *gormRepository) InsertRefundPending(tx *gorm.DB, row *RefundTransaction) (*RefundTransaction, bool, error) {
	if row.Status == "" {
		row.Status = "pending"
	}
	res := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(row)
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected == 1 {
		return row, true, nil
	}
	// Losing transaction: the winner's row isn't visible in our own
	// snapshot yet, so re-select it. This relies on READ COMMITTED (the
	// default isolation level) so that once the winning tx commits, our
	// next statement in this tx sees its committed row.
	var existing RefundTransaction
	if err := tx.Where("idempotency_key = ?", row.IdempotencyKey).First(&existing).Error; err != nil {
		return nil, false, err
	}
	return &existing, false, nil
}

// UpdateRefundOutcome sets status + provider_refund_id on a ledger row.
func (r *gormRepository) UpdateRefundOutcome(tx *gorm.DB, ledgerID, providerRefundID, status string) error {
	res := tx.Model(&RefundTransaction{}).
		Where("id = ?", ledgerID).
		Updates(map[string]any{
			"provider_refund_id": providerRefundID,
			"status":             status,
			"updated_at":         gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("payment: UpdateRefundOutcome: no refund_transactions row with id %s", ledgerID)
	}
	return nil
}

// GetRefundByIdempotencyKey reads a ledger row by its unique key.
func (r *gormRepository) GetRefundByIdempotencyKey(ctx context.Context, key string) (*RefundTransaction, error) {
	var row RefundTransaction
	if err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// ---------------------------------------------------------------------------
// WebhookEventRecord CRUD
// ---------------------------------------------------------------------------

func (r *gormRepository) CreateWebhookEvent(ctx context.Context, evt *WebhookEventRecord) error {
	return r.db.WithContext(ctx).Create(evt).Error
}

func (r *gormRepository) GetWebhookEventByProviderID(ctx context.Context, providerEventID string) (*WebhookEventRecord, error) {
	var evt WebhookEventRecord
	err := r.db.WithContext(ctx).
		Where("provider_event_id = ?", providerEventID).
		First(&evt).Error
	if err != nil {
		return nil, err
	}
	return &evt, nil
}
