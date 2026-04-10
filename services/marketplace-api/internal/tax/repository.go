package tax

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ── GORM models ──────────────────────────────────────────────────────────────

// OrderTaxLine persists a single computed tax line for an order.
type OrderTaxLine struct {
	ID           uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderID      uuid.UUID       `gorm:"column:order_id;type:uuid;not null"`
	Description  string          `gorm:"column:description;type:varchar(120);not null"`
	Rate         decimal.Decimal `gorm:"column:rate;type:numeric(8,6);not null"`
	Amount       decimal.Decimal `gorm:"column:amount;type:numeric(12,2);not null"`
	Jurisdiction string          `gorm:"column:jurisdiction;type:varchar(120)"`
	CreatedAt    time.Time       `gorm:"column:created_at;not null;default:now()"`
}

func (OrderTaxLine) TableName() string { return "order_tax_lines" }

// TaxProviderConfig stores the per-store tax provider configuration.
type TaxProviderConfig struct {
	ID              uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID        uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID         uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	Provider        string     `gorm:"column:provider;type:varchar(40);not null"`
	APIKeyEncrypted string     `gorm:"column:api_key_encrypted;type:text"`
	Mode            string     `gorm:"column:mode;type:varchar(20);not null;default:live"`
	IsActive        bool       `gorm:"column:is_active;type:boolean;not null;default:true"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;not null;default:now()"`
	DeletedAt       *time.Time `gorm:"column:deleted_at"`
}

func (TaxProviderConfig) TableName() string { return "tax_provider_configs" }

// ── Repository interface ─────────────────────────────────────────────────────

// Repository is the data-access surface for tax-related persistence.
type Repository interface {
	// SaveTaxLines inserts the computed tax lines for an order. Existing lines
	// for the same order_id are deleted first (idempotent recalculation).
	SaveTaxLines(ctx context.Context, db *gorm.DB, orderID uuid.UUID, lines []TaxLine) error

	// GetTaxLines returns all persisted tax lines for the given order.
	GetTaxLines(ctx context.Context, db *gorm.DB, orderID uuid.UUID) ([]OrderTaxLine, error)

	// GetProviderConfig returns the active tax provider config for a store, or
	// an error wrapping gorm.ErrRecordNotFound when none exists.
	GetProviderConfig(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*TaxProviderConfig, error)
}

// ── GORM implementation ──────────────────────────────────────────────────────

type gormRepository struct{}

// NewRepository returns a stateless GORM-backed repository. The *gorm.DB is
// supplied per-call so callers can thread transactions through.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) SaveTaxLines(ctx context.Context, db *gorm.DB, orderID uuid.UUID, lines []TaxLine) error {
	// Wrap delete+insert in a transaction for atomicity. If the caller
	// already passed a tx, db.Transaction runs as a savepoint.
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete existing lines for idempotent recalculation.
		if err := tx.Where("order_id = ?", orderID).
			Delete(&OrderTaxLine{}).Error; err != nil {
			return fmt.Errorf("tax: delete existing lines: %w", err)
		}

		if len(lines) == 0 {
			return nil
		}

		rows := make([]OrderTaxLine, 0, len(lines))
		for _, l := range lines {
			rows = append(rows, OrderTaxLine{
				ID:           uuid.New(),
				OrderID:      orderID,
				Description:  l.Description,
				Rate:         l.Rate,
				Amount:       l.Amount,
				Jurisdiction: l.Jurisdiction,
			})
		}

		if err := tx.Create(&rows).Error; err != nil {
			return fmt.Errorf("tax: insert tax lines: %w", err)
		}

		return nil
	})
}

func (gormRepository) GetTaxLines(ctx context.Context, db *gorm.DB, orderID uuid.UUID) ([]OrderTaxLine, error) {
	var rows []OrderTaxLine
	if err := db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("tax: get tax lines: %w", err)
	}
	return rows, nil
}

func (gormRepository) GetProviderConfig(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*TaxProviderConfig, error) {
	var cfg TaxProviderConfig
	err := db.WithContext(ctx).
		Where("store_id = ? AND is_active = ?", storeID, true).
		First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("tax: provider config not found for store %s: %w", storeID, err)
		}
		return nil, fmt.Errorf("tax: get provider config: %w", err)
	}
	return &cfg, nil
}
