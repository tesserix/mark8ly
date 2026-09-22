package csvjob

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// gormStoreLookup reads the local stores projection.
//
// It queries by id alone, with no tenant filter, because the dispatcher
// has no tenant to filter by: it starts from a job row, and the tenant is
// what it is trying to find. Nothing user-supplied reaches here — the
// store id comes from a row the submit handler wrote under a
// tenant-scoped route.
type gormStoreLookup struct{ db *gorm.DB }

// NewStoreLookup constructs a StoreLookup over the stores projection.
func NewStoreLookup(db *gorm.DB) StoreLookup { return &gormStoreLookup{db: db} }

func (l *gormStoreLookup) TenantAndCurrency(ctx context.Context, storeID string) (string, string, error) {
	var row struct {
		TenantID     string
		CurrencyCode string
	}
	err := l.db.WithContext(ctx).
		Table("stores").
		Select("tenant_id", "currency_code").
		Where("id = ?", storeID).
		Take(&row).Error
	if err != nil {
		return "", "", fmt.Errorf("csvjob: look up store %s: %w", storeID, err)
	}
	return row.TenantID, row.CurrencyCode, nil
}
