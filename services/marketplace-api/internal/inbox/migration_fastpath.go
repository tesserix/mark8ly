package inbox

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/migration"
)

// MigrationFastPathProvider surfaces merchant-initiated platform migration
// submissions awaiting a CSM decision.
//
// The decision itself is migration.Handler.Review, already mounted on the
// /internal group (see cmd/marketplace-api/main.go). This provider only makes
// the pending queue visible.
type MigrationFastPathProvider struct{ db *gorm.DB }

func NewMigrationFastPathProvider(db *gorm.DB) *MigrationFastPathProvider {
	return &MigrationFastPathProvider{db: db}
}

func (p *MigrationFastPathProvider) Kind() string { return KindMigrationFastPath }

func (p *MigrationFastPathProvider) List(ctx context.Context, f Filter) ([]Item, error) {
	if f.Status != "" && f.Status != "pending" {
		return []Item{}, nil
	}
	q := p.db.WithContext(ctx).Model(&migration.Review{}).Where("status = ?", "pending")
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	q = q.Order("created_at ASC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Page > 1 && f.Limit > 0 {
		q = q.Offset((f.Page - 1) * f.Limit)
	}

	var rows []migration.Review
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, migrationFastPathItem(r))
	}
	return items, nil
}

// migrationFastPathItem is the single definition of this kind's wire shape.
//
// List and Get MUST build items through it. The action-execution endpoint
// validates a requested action against the item's own Actions array, so a Get
// that declared a different set from List would let the console offer a button
// the executor rejects, or accept one it never showed (#281a).
func migrationFastPathItem(r migration.Review) Item {
	return Item{
		ID:           r.ID.String(),
		Kind:         KindMigrationFastPath,
		Title:        "Fast-path migration " + r.ID.String()[:8],
		Subtitle:     r.PriorPlatform,
		WaitingSince: r.CreatedAt,
		Severity:     SeverityNormal,
		Href:         "/admin/migration-fast-path/" + r.ID.String(),
		Actions: []Action{
			{ID: "approve", Label: "Approve", Destructive: false},
			{ID: "reject", Label: "Reject", Destructive: true},
		},
	}
}

// Get reads one pending review back so its declared actions can be checked.
//
// Scoped to `status = 'pending'` exactly as List is: an already-decided review
// is not waiting on a human, so it is not in this queue, and an action against
// it must fail as not-found rather than attempt a second decision.
func (p *MigrationFastPathProvider) Get(ctx context.Context, id string) (Item, error) {
	var row migration.Review
	err := p.db.WithContext(ctx).
		Where("id = ? AND status = ?", id, "pending").
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Item{}, fmt.Errorf("%w: %s/%s", ErrItemNotFound, KindMigrationFastPath, id)
	}
	if err != nil {
		return Item{}, err
	}
	return migrationFastPathItem(row), nil
}

func (p *MigrationFastPathProvider) Count(ctx context.Context, f Filter) (int64, error) {
	if f.Status != "" && f.Status != "pending" {
		return 0, nil
	}
	q := p.db.WithContext(ctx).Model(&migration.Review{}).Where("status = ?", "pending")
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	var n int64
	return n, q.Count(&n).Error
}
