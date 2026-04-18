package planchange

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// PlanChangeAuditRow is the GORM model for subscription_plan_change_audit.
// This table is append-only: migration 050 issues REVOKE UPDATE, DELETE ON
// subscription_plan_change_audit FROM PUBLIC, so rows cannot be mutated or
// removed after insert — enforced at the DB level.
type PlanChangeAuditRow struct {
	ID                   uuid.UUID                       `gorm:"column:id;type:uuid;primaryKey"`
	TenantID             uuid.UUID                       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID              uuid.UUID                       `gorm:"column:store_id;type:uuid;not null"`
	StripeSubscriptionID string                          `gorm:"column:stripe_subscription_id"`
	StripeInvoiceID      string                          `gorm:"column:stripe_invoice_id"`
	FromPlan             subscription.SubscriptionPlan   `gorm:"column:from_plan;not null"`
	ToPlan               subscription.SubscriptionPlan   `gorm:"column:to_plan;not null"`
	FromPeriod           subscription.SubscriptionPeriod `gorm:"column:from_period;not null"`
	ToPeriod             subscription.SubscriptionPeriod `gorm:"column:to_period;not null"`
	// Action must be one of the values in the spca_action_check constraint:
	// upgrade_committed | downgrade_scheduled | downgrade_committed |
	// downgrade_blocked_over_quota | period_switch_committed.
	Action          string    `gorm:"column:action;not null"`
	BillingCurrency string    `gorm:"column:billing_currency;not null"`
	ProrationCents  int64     `gorm:"column:proration_cents"`
	Actor           string    `gorm:"column:actor;not null"`
	Reason          string    `gorm:"column:reason"`
	EffectiveAt     time.Time `gorm:"column:effective_at;not null"`
	CreatedAt       time.Time `gorm:"column:created_at;not null"`
}

// TableName pins the GORM table name to the migration-created table.
func (PlanChangeAuditRow) TableName() string { return "subscription_plan_change_audit" }

// WritePlanChangeAuditRowTx appends one row inside the caller's transaction tx.
// Auto-fills ID (uuid.New) and CreatedAt (time.Now) when zero so callers only
// need to set the business fields. Must be called inside a transaction — the
// table's REVOKE UPDATE, DELETE means a committed row is permanent.
func WritePlanChangeAuditRowTx(ctx context.Context, tx *gorm.DB, row PlanChangeAuditRow) error {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now()
	}
	return tx.WithContext(ctx).Create(&row).Error
}
