//go:build integration

package tenantdiscount_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/tenantdiscount"
)

// These tests cover migration 000132's table and the two things the service
// does with it: record the tenant's override once before the fan-out, and read
// it back when a subscription is created for a store that did not exist when
// an operator pressed the button.

// overrides returns every tenant_applied_discounts row for a tenant, oldest
// first. Read through the shipped model so a column renamed in the migration
// and not in the struct fails here rather than silently reading zero values.
func overrides(t *testing.T, db *gorm.DB, tenantID uuid.UUID) []tenantdiscount.AppliedOverride {
	t.Helper()
	var rows []tenantdiscount.AppliedOverride
	require.NoError(t, db.Where("tenant_id = ?", tenantID).Order("granted_at").Find(&rows).Error)
	return rows
}

// liveOverride returns the tenant's single live row, failing when there is not
// exactly one. Callers asserting "no live row" use overrides() directly.
func liveOverride(t *testing.T, db *gorm.DB, tenantID uuid.UUID) tenantdiscount.AppliedOverride {
	t.Helper()
	var live []tenantdiscount.AppliedOverride
	for _, r := range overrides(t, db, tenantID) {
		if r.Live() {
			live = append(live, r)
		}
	}
	require.Len(t, live, 1, "expected exactly one live override for the tenant")
	return live[0]
}

// ---------------------------------------------------------------------------
// The record is written once, before the fan-out
// ---------------------------------------------------------------------------

// observingStripe wraps the fake and reads the override table THROUGH A
// SEPARATE, COMMITTED HANDLE at the moment of the first Stripe call.
//
// This is what distinguishes "written before the fan-out" from "written after
// it": a post-hoc count cannot tell the two apart, because both leave the same
// row behind. The read has to happen while the fan-out is in flight.
type observingStripe struct {
	*fakeStripe
	db *gorm.DB

	// liveAtFirstCall is the number of live rows for the tenant observed
	// during the first Stripe call of the fan-out.
	liveAtFirstCall int64
	tenantID        uuid.UUID
	observed        bool
}

func (o *observingStripe) SubscriptionHasDiscount(ctx context.Context, subID, couponID string) (bool, error) {
	if !o.observed {
		o.observed = true
		_ = o.db.Model(&tenantdiscount.AppliedOverride{}).
			Where("tenant_id = ? AND removed_at IS NULL", o.tenantID).
			Count(&o.liveAtFirstCall).Error
	}
	return o.fakeStripe.SubscriptionHasDiscount(ctx, subID, couponID)
}

func TestApply_RecordsTheOverrideBeforeTheFanOutTouchesStripe(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	seedStoreWithSubscription(t, db, tenantID, "sub_a")
	seedStoreWithSubscription(t, db, tenantID, "sub_b")

	obs := &observingStripe{fakeStripe: newFakeStripe(), db: db, tenantID: tenantID}
	_, err := newService(t, db, obs, realEmitter(t, db)).
		Apply(context.Background(), tenantdiscount.Input{
			TenantID: tenantID, CouponID: couponID, Reason: "negotiated 2026 rate",
			C: operatorContext("op_7"),
		})
	require.NoError(t, err)

	require.True(t, obs.observed, "the fan-out must have reached Stripe at least once")
	require.EqualValues(t, 1, obs.liveAtFirstCall,
		"the tenant's override must already be recorded when the first store's Stripe call happens — a store created mid-fan-out has to be covered too")

	live := liveOverride(t, db, tenantID)
	require.Equal(t, couponID, live.StripeCouponID)
	require.NotNil(t, live.GrantedBy)
	require.Equal(t, "op_7", *live.GrantedBy)
	require.Nil(t, live.RemovedBy)
}

// A fan-out where EVERY store fails must still leave the tenant marked as
// holding the override: the operator retries those stores, but a store created
// in the meantime is covered by the record regardless of how today's stores
// fared.
func TestApply_TheOverrideRecordSurvivesAFanOutThatFailedForEveryStore(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	a := seedStoreWithSubscription(t, db, tenantID, "sub_a")
	b := seedStoreWithSubscription(t, db, tenantID, "sub_b")

	fs := newFakeStripe()
	fs.fail("sub_a", errStripeDown)
	fs.fail("sub_b", errStripeDown)

	res, err := newService(t, db, fs, realEmitter(t, db)).
		Apply(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})
	require.NoError(t, err)

	got := outcomes(res)
	require.Equal(t, tenantdiscount.OutcomeFailed, got[a.StoreID].Outcome)
	require.Equal(t, tenantdiscount.OutcomeFailed, got[b.StoreID].Outcome)

	require.Equal(t, couponID, liveOverride(t, db, tenantID).StripeCouponID,
		"the record is tenant-scoped and must not depend on the per-store outcomes")
}

// A partial failure — the ordinary case the plan is written around.
func TestApply_TheOverrideRecordSurvivesAPartialFanOutFailure(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	ok := seedStoreWithSubscription(t, db, tenantID, "sub_a")
	broken := seedStoreWithSubscription(t, db, tenantID, "sub_bad")

	fs := newFakeStripe()
	fs.fail("sub_bad", errStripeDown)

	res, err := newService(t, db, fs, realEmitter(t, db)).
		Apply(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})
	require.NoError(t, err)

	got := outcomes(res)
	require.Equal(t, tenantdiscount.OutcomeApplied, got[ok.StoreID].Outcome)
	require.Equal(t, tenantdiscount.OutcomeFailed, got[broken.StoreID].Outcome)
	require.Equal(t, couponID, liveOverride(t, db, tenantID).StripeCouponID)
}

// Re-applying the same coupon must not stack a second live row — the partial
// unique index would refuse it, and a fan-out that a retry cannot re-run is a
// fan-out an operator cannot correct.
func TestApply_ReApplyingTheSameCouponKeepsOneLiveRecord(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	seedStoreWithSubscription(t, db, tenantID, "sub_a")

	svc := newService(t, db, newFakeStripe(), realEmitter(t, db))
	in := tenantdiscount.Input{TenantID: tenantID, CouponID: couponID}

	_, err := svc.Apply(context.Background(), in)
	require.NoError(t, err)
	_, err = svc.Apply(context.Background(), in)
	require.NoError(t, err)

	require.Len(t, overrides(t, db, tenantID), 1, "the second apply must reuse the recorded row, not add one")
}

// A SECOND, different coupon is refused before anything reaches Stripe.
// Superseding the recorded row would claim a retirement that never happened in
// Stripe — the first coupon would stay on every subscription.
func TestApply_RefusesASecondDifferentCouponWhileOneIsRecorded(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	seedStoreWithSubscription(t, db, tenantID, "sub_a")

	fs := newFakeStripe()
	svc := newService(t, db, fs, realEmitter(t, db))

	_, err := svc.Apply(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})
	require.NoError(t, err)

	_, err = svc.Apply(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: "co_second"})
	require.ErrorIs(t, err, tenantdiscount.ErrOverrideAlreadyRecorded)
	require.Contains(t, err.Error(), couponID, "the refusal must name the coupon already held")

	require.Equal(t, []string{"sub_a/" + couponID}, fs.adds,
		"the refusal must happen before any Stripe call, so nothing is half-applied")
	require.Len(t, overrides(t, db, tenantID), 1)
}

// ---------------------------------------------------------------------------
// Remove clears it
// ---------------------------------------------------------------------------

func TestRemove_RetiresTheRecordedOverride(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	seedStoreWithSubscription(t, db, tenantID, "sub_a")

	svc := newService(t, db, newFakeStripe(), realEmitter(t, db))
	_, err := svc.Apply(context.Background(), tenantdiscount.Input{
		TenantID: tenantID, CouponID: couponID, C: operatorContext("op_grant")})
	require.NoError(t, err)

	_, err = svc.Remove(context.Background(), tenantdiscount.Input{
		TenantID: tenantID, CouponID: couponID, C: operatorContext("op_revoke")})
	require.NoError(t, err)

	rows := overrides(t, db, tenantID)
	require.Len(t, rows, 1)
	require.False(t, rows[0].Live())
	require.NotNil(t, rows[0].RemovedBy)
	require.Equal(t, "op_revoke", *rows[0].RemovedBy)
	require.NotNil(t, rows[0].RemovedAt)
	require.False(t, rows[0].RemovedAt.Before(rows[0].GrantedAt),
		"tenant_applied_discounts_removal_follows_grant: a removal cannot precede its grant")
}

// Retiring the row is what lets the tenant be granted a DIFFERENT override
// later — the reason the uniqueness rule is a partial index over the live rows
// rather than a plain UNIQUE (tenant_id).
func TestRemove_ThenApplyingADifferentCouponIsAccepted(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	seedStoreWithSubscription(t, db, tenantID, "sub_a")

	svc := newService(t, db, newFakeStripe(), realEmitter(t, db))
	ctx := context.Background()

	require.NoError(t, firstErr(svc.Apply(ctx, tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})))
	require.NoError(t, firstErr(svc.Remove(ctx, tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})))
	require.NoError(t, firstErr(svc.Apply(ctx, tenantdiscount.Input{TenantID: tenantID, CouponID: "co_second"})))

	require.Len(t, overrides(t, db, tenantID), 2)
	require.Equal(t, "co_second", liveOverride(t, db, tenantID).StripeCouponID)
}

// Removing an override this service never recorded is not an error: an
// override applied before migration 000132 existed still has to be removable
// from Stripe, and refusing here would strand those merchants discounted.
func TestRemove_WithNoRecordedOverrideStillFansOut(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	store := seedStoreWithSubscription(t, db, tenantID, "sub_a")

	fs := newFakeStripe()
	fs.attach("sub_a", couponID)

	res, err := newService(t, db, fs, realEmitter(t, db)).
		Remove(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})
	require.NoError(t, err)

	require.Equal(t, tenantdiscount.OutcomeRemoved, outcomes(res)[store.StoreID].Outcome)
	require.Empty(t, overrides(t, db, tenantID))
}

// firstErr discards a two-value (Result, error) return down to its error, so
// the sequences above read as the steps they are.
func firstErr(_ tenantdiscount.Result, err error) error { return err }

// ---------------------------------------------------------------------------
// The schema's own guarantees
// ---------------------------------------------------------------------------

// The ceiling. Asserted against the database rather than the service, because
// the service is not the only thing that could ever write here and the index
// is what makes "which coupon does this tenant hold" have one answer.
func TestSchema_PartialUniqueIndexRefusesASecondLiveRow(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()

	require.NoError(t, db.Create(&tenantdiscount.AppliedOverride{
		TenantID: tenantID, StripeCouponID: couponID}).Error)

	err := db.Create(&tenantdiscount.AppliedOverride{
		TenantID: tenantID, StripeCouponID: "co_second"}).Error
	require.Error(t, err)
	require.Contains(t, err.Error(), "tenant_applied_discounts_one_live_per_tenant")
}

// And the floor it is not: a retired row does not occupy the slot.
func TestSchema_ARetiredRowDoesNotBlockANewLiveOne(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()

	require.NoError(t, db.Create(&tenantdiscount.AppliedOverride{
		TenantID: tenantID, StripeCouponID: couponID}).Error)
	require.NoError(t, db.Model(&tenantdiscount.AppliedOverride{}).
		Where("tenant_id = ?", tenantID).
		Updates(map[string]any{"removed_by": "op_x", "removed_at": gorm.Expr("now()")}).Error)

	require.NoError(t, db.Create(&tenantdiscount.AppliedOverride{
		TenantID: tenantID, StripeCouponID: "co_second"}).Error)
}

// The removal pair is whole in BOTH directions. A removed_at with no
// removed_by is a removal nobody is accountable for; a removed_by with no
// removed_at is a row the partial index still counts as live, which would keep
// handing the coupon to new stores after it was removed.
func TestSchema_TheRemovalPairMustBeWholeInBothDirections(t *testing.T) {
	db := newDB(t)

	err := db.Exec(`INSERT INTO tenant_applied_discounts (tenant_id, stripe_coupon_id, removed_at)
	                VALUES (?, ?, now())`, uuid.New(), couponID).Error
	require.Error(t, err)
	require.Contains(t, err.Error(), "tenant_applied_discounts_removal_is_whole")

	err = db.Exec(`INSERT INTO tenant_applied_discounts (tenant_id, stripe_coupon_id, removed_by)
	               VALUES (?, ?, 'op_x')`, uuid.New(), couponID).Error
	require.Error(t, err)
	require.Contains(t, err.Error(), "tenant_applied_discounts_removal_is_whole")
}

func TestSchema_ABlankCouponIdIsRefused(t *testing.T) {
	db := newDB(t)
	err := db.Create(&tenantdiscount.AppliedOverride{TenantID: uuid.New(), StripeCouponID: "   "}).Error
	require.Error(t, err)
	require.Contains(t, err.Error(), "tenant_applied_discounts_coupon_id_is_not_blank")
}

// ---------------------------------------------------------------------------
// ApplyToNewSubscription — the "future stores" half
// ---------------------------------------------------------------------------

func TestApplyToNewSubscription_AppliesTheTenantsLiveOverride(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	pending := seedStoreWithSubscription(t, db, tenantID, "")

	fs := newFakeStripe()
	svc := newService(t, db, fs, realEmitter(t, db))

	// The operator applies while the store is still card-less: reported
	// pending, and the record is what carries it forward.
	res, err := svc.Apply(context.Background(), tenantdiscount.Input{
		TenantID: tenantID, CouponID: couponID, C: operatorContext("op_1")})
	require.NoError(t, err)
	require.Equal(t, tenantdiscount.OutcomePending, outcomes(res)[pending.StoreID].Outcome)

	// Later, the store gets a Stripe subscription.
	outcome, err := svc.ApplyToNewSubscription(context.Background(), tenantdiscount.NewSubscriptionInput{
		TenantID: tenantID, StoreID: pending.StoreID, StripeSubscriptionID: "sub_new"})
	require.NoError(t, err)
	require.Equal(t, tenantdiscount.OutcomeApplied, outcome)
	require.True(t, fs.has("sub_new", couponID))

	require.EqualValues(t, 2, auditRowsForStore(t, db, pending.StoreID, tenantdiscount.ActionApply),
		"the operator's pending outcome and this application are both audited")
}

// A newly created subscription for a tenant with NO override is untouched —
// the control that stops the assertion above passing for every tenant.
func TestApplyToNewSubscription_ATenantWithNoOverrideIsUntouched(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	storeID := seedStoreWithoutSubscription(t, db, tenantID)

	fs := newFakeStripe()
	outcome, err := newService(t, db, fs, realEmitter(t, db)).
		ApplyToNewSubscription(context.Background(), tenantdiscount.NewSubscriptionInput{
			TenantID: tenantID, StoreID: storeID, StripeSubscriptionID: "sub_new"})
	require.NoError(t, err)
	require.Equal(t, tenantdiscount.OutcomeNoOverride, outcome)

	require.Empty(t, fs.reads, "no override means no Stripe call at all")
	require.Empty(t, fs.adds)
	require.EqualValues(t, 0, auditRowsForStore(t, db, storeID, tenantdiscount.ActionApply))
}

// A RETIRED override must not be re-applied to a new store. Without this, the
// removal would only hold until the tenant opened another shop.
func TestApplyToNewSubscription_ARemovedOverrideIsNotApplied(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	store := seedStoreWithSubscription(t, db, tenantID, "sub_a")

	fs := newFakeStripe()
	svc := newService(t, db, fs, realEmitter(t, db))
	ctx := context.Background()
	require.NoError(t, firstErr(svc.Apply(ctx, tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})))
	require.NoError(t, firstErr(svc.Remove(ctx, tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})))

	outcome, err := svc.ApplyToNewSubscription(ctx, tenantdiscount.NewSubscriptionInput{
		TenantID: tenantID, StoreID: store.StoreID, StripeSubscriptionID: "sub_new"})
	require.NoError(t, err)
	require.Equal(t, tenantdiscount.OutcomeNoOverride, outcome)
	require.False(t, fs.has("sub_new", couponID))
}

// Creation paths retry, so this has to be safe to run twice.
func TestApplyToNewSubscription_IsIdempotent(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	store := seedStoreWithSubscription(t, db, tenantID, "sub_a")

	fs := newFakeStripe()
	svc := newService(t, db, fs, realEmitter(t, db))
	require.NoError(t, firstErr(svc.Apply(context.Background(),
		tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})))

	in := tenantdiscount.NewSubscriptionInput{
		TenantID: tenantID, StoreID: store.StoreID, StripeSubscriptionID: "sub_new"}

	first, err := svc.ApplyToNewSubscription(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, tenantdiscount.OutcomeApplied, first)

	second, err := svc.ApplyToNewSubscription(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, tenantdiscount.OutcomeAlreadyApplied, second)

	require.Equal(t, []string{"sub_a/" + couponID, "sub_new/" + couponID}, fs.adds,
		"the replay must not send a second attach")
}

// The audit row this path writes has no operator behind it, and says so.
func TestApplyToNewSubscription_TheAuditRowRecordsASystemActorAndTheCause(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	store := seedStoreWithSubscription(t, db, tenantID, "sub_a")

	svc := newService(t, db, newFakeStripe(), realEmitter(t, db))
	require.NoError(t, firstErr(svc.Apply(context.Background(),
		tenantdiscount.Input{TenantID: tenantID, CouponID: couponID, C: operatorContext("op_1")})))

	_, err := svc.ApplyToNewSubscription(context.Background(), tenantdiscount.NewSubscriptionInput{
		TenantID: tenantID, StoreID: store.StoreID, StripeSubscriptionID: "sub_new"})
	require.NoError(t, err)

	var row audit.Entry
	require.NoError(t, db.Where("store_id = ? AND action = ? AND metadata->>'stripe_subscription_id' = ?",
		store.StoreID, tenantdiscount.ActionApply, "sub_new").First(&row).Error)

	require.Nil(t, row.ActorOperatorID, "no operator is behind the creation hook")
	require.Equal(t, couponID, row.Metadata["coupon_id"])
	require.Equal(t, string(tenantdiscount.OutcomeApplied), row.Metadata["outcome"])
	require.Contains(t, row.Metadata["reason"], "standing platform override")
}

func TestApplyToNewSubscription_RefusesABlankSubscriptionID(t *testing.T) {
	db := newDB(t)
	_, err := newService(t, db, newFakeStripe(), realEmitter(t, db)).
		ApplyToNewSubscription(context.Background(), tenantdiscount.NewSubscriptionInput{
			TenantID: uuid.New(), StoreID: uuid.New()})
	require.ErrorIs(t, err, tenantdiscount.ErrNoStripeSubscription)
}
