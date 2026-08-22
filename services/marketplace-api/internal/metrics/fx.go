package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// fxRate is the GORM model for the fx_rates table (migration 063).
type fxRate struct {
	Currency   string          `gorm:"column:currency;primaryKey"`
	USDMidRate decimal.Decimal `gorm:"column:usd_mid_rate;type:numeric(18,10)"`
	FetchedAt  time.Time       `gorm:"column:fetched_at"`
	Source     string          `gorm:"column:source"`
}

func (fxRate) TableName() string { return "fx_rates" }

// FXRepository provides read and write access to the fx_rates table.
type FXRepository struct {
	db *gorm.DB
}

// NewFXRepository constructs a FXRepository backed by db.
func NewFXRepository(db *gorm.DB) *FXRepository {
	return &FXRepository{db: db}
}

// Upsert inserts or updates all rates in the map. source identifies the
// data provider (e.g. "static_fallback", "openexchangerates").
func (r *FXRepository) Upsert(ctx context.Context, rates map[string]decimal.Decimal, source string) error {
	now := time.Now().UTC()
	for currency, rate := range rates {
		row := fxRate{
			Currency:   currency,
			USDMidRate: rate,
			FetchedAt:  now,
			Source:     source,
		}
		res := r.db.WithContext(ctx).
			Where(fxRate{Currency: currency}).
			Assign(fxRate{USDMidRate: rate, FetchedAt: now, Source: source}).
			FirstOrCreate(&row)
		if res.Error != nil {
			return fmt.Errorf("fx: upsert %s: %w", currency, res.Error)
		}
		// If the row already existed, update it explicitly.
		if res.RowsAffected == 0 {
			if err := r.db.WithContext(ctx).
				Model(&fxRate{}).
				Where("currency = ?", currency).
				Updates(map[string]any{
					"usd_mid_rate": rate,
					"fetched_at":   now,
					"source":       source,
				}).Error; err != nil {
				return fmt.Errorf("fx: update %s: %w", currency, err)
			}
		}
	}
	return nil
}

// Get returns the USD mid-rate for the given ISO 4217 currency code (upper-case).
// Returns an error if the currency is not found in the table.
func (r *FXRepository) Get(ctx context.Context, currency string) (decimal.Decimal, error) {
	var row fxRate
	if err := r.db.WithContext(ctx).
		Where("currency = ?", currency).
		First(&row).Error; err != nil {
		return decimal.Zero, fmt.Errorf("fx: get %s: %w", currency, err)
	}
	return row.USDMidRate, nil
}

// GetAll returns all stored currency→USD rates as a map.
func (r *FXRepository) GetAll(ctx context.Context) (map[string]decimal.Decimal, error) {
	var rows []fxRate
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("fx: get all: %w", err)
	}
	out := make(map[string]decimal.Decimal, len(rows))
	for _, row := range rows {
		out[row.Currency] = row.USDMidRate
	}
	return out, nil
}
