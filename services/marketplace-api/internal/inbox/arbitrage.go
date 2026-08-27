package inbox

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
)

// ArbitrageProvider surfaces geo-pricing arbitrage flags still under review.
type ArbitrageProvider struct{ db *gorm.DB }

func NewArbitrageProvider(db *gorm.DB) *ArbitrageProvider { return &ArbitrageProvider{db: db} }

func (p *ArbitrageProvider) Kind() string { return KindArbitrageAppeal }

type arbitrageRow struct {
	ID                string
	ResolvedPriceTier string
	MismatchReason    *string
	FlaggedAt         time.Time
}

func (p *ArbitrageProvider) List(ctx context.Context, f Filter) ([]Item, error) {
	q := p.db.WithContext(ctx).
		Table("subscription_arbitrage_audit").
		Select("id::text AS id, resolved_price_tier, mismatch_reason, flagged_at").
		Where("resolution = ?", string(arbitrage.ResolutionOngoing))
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	q = q.Order("flagged_at ASC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Page > 1 && f.Limit > 0 {
		q = q.Offset((f.Page - 1) * f.Limit)
	}

	var rows []arbitrageRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		subtitle := "Price tier " + r.ResolvedPriceTier
		if r.MismatchReason != nil && *r.MismatchReason != "" {
			subtitle = *r.MismatchReason
		}
		items = append(items, Item{
			ID:           r.ID,
			Kind:         KindArbitrageAppeal,
			Title:        "Arbitrage flag " + r.ID[:8],
			Subtitle:     subtitle,
			WaitingSince: r.FlaggedAt,
			Severity:     SeverityNormal,
			Href:         "/admin/arbitrage/" + r.ID,
			Actions: []Action{
				{ID: "uphold", Label: "Uphold flag", Destructive: false},
				{ID: "dismiss", Label: "Dismiss flag", Destructive: true},
			},
		})
	}
	return items, nil
}

func (p *ArbitrageProvider) Count(ctx context.Context, f Filter) (int64, error) {
	q := p.db.WithContext(ctx).
		Table("subscription_arbitrage_audit").
		Where("resolution = ?", string(arbitrage.ResolutionOngoing))
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	var n int64
	return n, q.Count(&n).Error
}
