package tenantdiscount

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AppliedOverride is the GORM model for tenant_applied_discounts
// (migration 000132): the record of what THIS SERVICE applied for a tenant.
//
// It is not the grant. The console's tenant_pricing_override_coupons
// (tesserix-home 0047) records who granted what and why, and this service
// cannot read it. What a live row here says, and all it says, is: this service
// attached this coupon for this tenant, and has not since been told to stop.
// A console-side retirement that never reaches mark8ly leaves this row live —
// a known, accepted limitation of duplicating a fact another service owns,
// stated at length in the migration header.
type AppliedOverride struct {
	ID       uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"`

	// StripeCouponID is the `co_…` this service applied. Never blank — the
	// service trims and refuses a blank coupon id, and
	// tenant_applied_discounts_coupon_id_is_not_blank is the second line.
	StripeCouponID string `gorm:"column:stripe_coupon_id;type:text;not null"`

	// GrantedBy is the platform operator who applied it, or nil when no
	// operator was behind the write — which is the subscription-creation
	// hook, running with no request. nil means "this service, on its own
	// behalf"; it never means "unknown operator".
	GrantedBy *string   `gorm:"column:granted_by;type:text"`
	GrantedAt time.Time `gorm:"column:granted_at;not null;default:now()"`

	// RemovedBy and RemovedAt are the retirement pair. Both nil is a live
	// row; both set is a retired one, and the database's
	// tenant_applied_discounts_removal_is_whole rejects any other
	// combination — so a retired row always names someone, even when that
	// someone is systemActor.
	RemovedBy *string    `gorm:"column:removed_by;type:text"`
	RemovedAt *time.Time `gorm:"column:removed_at"`
}

// TableName returns the database table name for GORM.
func (AppliedOverride) TableName() string { return "tenant_applied_discounts" }

// Live reports whether this row records an override that is still in force as
// far as this service knows.
func (o AppliedOverride) Live() bool { return o.RemovedAt == nil }

// systemActor is written to removed_by when a removal has no operator behind
// it. granted_by is left NULL in the same situation, and the asymmetry is the
// database's doing rather than a choice: the removal pair must be whole, so a
// retired row cannot leave removed_by NULL, while a live row's granted_by has
// no such constraint to satisfy.
//
// Production removals always have an operator — Remove is reached only through
// the platform-admin endpoint, whose middleware sets platform_operator_id — so
// this value is a fallback, not the normal case.
const systemActor = "system"

// loadLiveOverride returns the tenant's live override row, or nil when the
// tenant holds none.
//
// It can return at most one row without choosing between candidates, and that
// is the database's guarantee rather than this query's: the partial unique
// index tenant_applied_discounts_one_live_per_tenant permits exactly one row
// per tenant with removed_at IS NULL.
func loadLiveOverride(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (*AppliedOverride, error) {
	var row AppliedOverride
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND removed_at IS NULL", tenantID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("tenantdiscount: load live override for tenant %s: %w", tenantID, err)
	}
	return &row, nil
}

// recordApplied writes the tenant's override row, and is called ONCE per
// Apply, BEFORE the per-store fan-out.
//
// Before, and not after, because the record is what makes a store created
// LATER receive the discount. A fan-out that failed for every store still
// leaves the tenant correctly marked as holding an override — those stores are
// reported failed and the operator retries, but a store provisioned in the
// meantime is covered either way. Writing it afterwards, or only on success,
// would make "the tenant holds an override" depend on how many of today's
// stores Stripe happened to accept.
//
// Re-applying the SAME coupon is a no-op: this is the ceiling on double
// application. Applying a DIFFERENT coupon while one is live is refused with
// ErrOverrideAlreadyRecorded rather than superseded, because superseding would
// write a removal that never happened in Stripe — the old coupon would stay on
// every subscription while this table claimed it was retired.
func (s *Service) recordApplied(ctx context.Context, in Input) error {
	live, err := loadLiveOverride(ctx, s.db, in.TenantID)
	if err != nil {
		return err
	}
	if live != nil {
		if live.StripeCouponID == in.CouponID {
			return nil
		}
		return fmt.Errorf("%w: tenant %s already holds coupon %s (recorded %s); remove it before applying %s",
			ErrOverrideAlreadyRecorded, in.TenantID, live.StripeCouponID,
			live.GrantedAt.UTC().Format(time.RFC3339), in.CouponID)
	}

	row := AppliedOverride{
		TenantID:       in.TenantID,
		StripeCouponID: in.CouponID,
		GrantedBy:      operatorID(in.C),
	}
	// A concurrent Apply for the same tenant loses this insert to the partial
	// unique index rather than producing a second live row. The loser's whole
	// fan-out is refused, which is the safe direction: no Stripe call has been
	// made yet at this point.
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("tenantdiscount: record applied override for tenant %s: %w", in.TenantID, err)
	}
	return nil
}

// recordRemoved retires the tenant's live row for this coupon, and is called
// ONCE per Remove, BEFORE the per-store fan-out — the mirror of
// recordApplied's ordering and for the mirror reason: once the operator has
// said "stop", a store created while the fan-out is still running must not be
// given the coupon.
//
// A tenant with no live row for this coupon is not an error. An override
// applied before this table existed, or one whose record was never written,
// still has to be removable from Stripe; refusing here would leave those
// merchants discounted with no way to stop it.
func (s *Service) recordRemoved(ctx context.Context, in Input) error {
	by := systemActor
	if op := operatorID(in.C); op != nil {
		by = *op
	}

	// now() and not a Go timestamp: granted_at defaults to the DATABASE
	// clock, and tenant_applied_discounts_removal_follows_grant compares the
	// two. Sending the application server's clock would let ordinary skew
	// reject a legitimate removal.
	err := s.db.WithContext(ctx).Model(&AppliedOverride{}).
		Where("tenant_id = ? AND stripe_coupon_id = ? AND removed_at IS NULL", in.TenantID, in.CouponID).
		Updates(map[string]any{
			"removed_by": by,
			"removed_at": gorm.Expr("now()"),
		}).Error
	if err != nil {
		return fmt.Errorf("tenantdiscount: retire applied override for tenant %s: %w", in.TenantID, err)
	}
	return nil
}

// operatorID pulls the platform operator id off the request context, or
// returns nil when there is no request or no operator on it.
//
// The literal "platform_operator_id" is the same key audit's buildEntry reads
// (internal/audit/emitter.go), duplicated here for the same reason audit
// duplicates it rather than importing platformadmin: that package is an HTTP
// handler package. Keep both sides greppable if it changes.
func operatorID(c *gin.Context) *string {
	if c == nil {
		return nil
	}
	id := strings.TrimSpace(c.GetString("platform_operator_id"))
	if id == "" {
		return nil
	}
	return &id
}
