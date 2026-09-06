//go:build integration

package tenantdiscount_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/tenantdiscount"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// couponID is the platform override every test in this file applies. It is a
// single constant so an assertion can never pass by matching the wrong coupon.
const couponID = "co_platform_override"

// merchantPromo stands in for a discount the merchant already redeemed. The
// fan-out must never disturb it — that regression is what T2's
// read-modify-write pair exists for, and this package is its first caller.
const merchantPromo = "co_merchant_promo"

// newDB opens a real, committing handle and truncates everything these tests
// write. NewDB rather than NewTx: the service opens ONE TRANSACTION PER STORE,
// and a handle that is already inside a transaction would turn each of those
// into a savepoint, which is precisely the behaviour under test.
func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.NewDB(t, "audit_logs", "tenant_applied_discounts", "store_subscriptions", "stores")
}

// seededStore is one store plus (optionally) its subscription row.
type seededStore struct {
	StoreID              uuid.UUID
	SubscriptionID       uuid.UUID
	StripeSubscriptionID string
}

// seedStoreWithSubscription creates a store and a store_subscriptions row for
// it. A blank stripeSubID leaves stripe_subscription_id NULL — the card-less
// trialing tenant the plan calls out as exactly the population an operator
// discounts.
func seedStoreWithSubscription(t *testing.T, db *gorm.DB, tenantID uuid.UUID, stripeSubID string) seededStore {
	t.Helper()

	storeID := uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	row := subscription.StoreSubscription{
		TenantID:           tenantID,
		StoreID:            storeID,
		StripeCustomerID:   "cus_" + storeID.String()[:8],
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
	}
	if stripeSubID != "" {
		row.StripeSubscriptionID = &stripeSubID
	}
	require.NoError(t, db.Create(&row).Error)

	return seededStore{StoreID: storeID, SubscriptionID: row.ID, StripeSubscriptionID: stripeSubID}
}

// seedStoreWithoutSubscription creates a store that has no store_subscriptions
// row at all. The fan-out must report it, not skip it.
func seedStoreWithoutSubscription(t *testing.T, db *gorm.DB, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	storeID := uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	return storeID
}

// realEmitter wires the shipped gormRepository so EmitTx's insert really goes
// through Postgres. A double could prove which handle was passed but not what
// the database does with it, and transactional visibility is the whole point.
func realEmitter(t *testing.T, db *gorm.DB) *audit.Emitter {
	t.Helper()
	e, err := audit.NewEmitter(audit.EmitterConfig{DB: db, Repo: audit.NewRepository(), Logger: slog.Default()})
	require.NoError(t, err)
	t.Cleanup(func() { e.Stop(context.Background()) })
	return e
}

func newService(t *testing.T, db *gorm.DB, st tenantdiscount.StripeDiscounts, aw tenantdiscount.AuditWriter) *tenantdiscount.Service {
	t.Helper()
	svc, err := tenantdiscount.NewService(tenantdiscount.Config{
		DB: db, Stripe: st, Audit: aw, Logger: slog.Default(),
	})
	require.NoError(t, err)
	return svc
}

// outcomes indexes a Result by store id so assertions name the store rather
// than a slice position.
func outcomes(r tenantdiscount.Result) map[uuid.UUID]tenantdiscount.StoreResult {
	out := make(map[uuid.UUID]tenantdiscount.StoreResult, len(r.Stores))
	for _, s := range r.Stores {
		out[s.StoreID] = s
	}
	return out
}

// auditRowsForStore counts committed audit rows this package wrote for a store.
func auditRowsForStore(t *testing.T, h *gorm.DB, storeID uuid.UUID, action string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, h.Model(&audit.Entry{}).
		Where("store_id = ? AND action = ?", storeID, action).Count(&n).Error)
	return n
}

// operatorContext is a gin context carrying the platform_operator_id key the
// audit emitter derives ActorOperator from. Passed through Input so the audit
// row names the operator rather than the system.
func operatorContext(operatorID string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	// A Request is mandatory, not decoration: audit's buildEntry reads
	// c.ClientIP() and c.Request.UserAgent(), both of which dereference it.
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/billing/tenants/discount", nil)
	c.Set("platform_operator_id", operatorID)
	return c
}

// ---------------------------------------------------------------------------
// The fan-out
// ---------------------------------------------------------------------------

// A tenant with several stores has several subscriptions and several Stripe
// customers. #660 says "the tenant's subscription customer", singular; it is
// not, and this is the test that says so.
func TestApply_FansOutOverEveryStoreTheTenantOwns(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	a := seedStoreWithSubscription(t, db, tenantID, "sub_a")
	b := seedStoreWithSubscription(t, db, tenantID, "sub_b")
	c := seedStoreWithSubscription(t, db, tenantID, "sub_c")

	// A second tenant's store, to prove the fan-out is tenant-scoped and not
	// simply "every subscription in the table".
	other := seedStoreWithSubscription(t, db, uuid.New(), "sub_other")

	fs := newFakeStripe()
	fs.attach("sub_b", merchantPromo)

	res, err := newService(t, db, fs, realEmitter(t, db)).
		Apply(context.Background(), tenantdiscount.Input{
			TenantID: tenantID, CouponID: couponID, Reason: "negotiated 2026 rate",
			C: operatorContext("op_1"),
		})
	require.NoError(t, err)

	require.Len(t, res.Stores, 3, "every one of the tenant's stores must be reported")
	got := outcomes(res)
	for _, s := range []seededStore{a, b, c} {
		require.Equal(t, tenantdiscount.OutcomeApplied, got[s.StoreID].Outcome)
		require.True(t, fs.has(s.StripeSubscriptionID, couponID))
		require.EqualValues(t, 1, auditRowsForStore(t, db, s.StoreID, tenantdiscount.ActionApply))
	}

	require.NotContains(t, got, other.StoreID, "another tenant's store must not be touched")
	require.False(t, fs.has("sub_other", couponID))

	require.ElementsMatch(t, []string{merchantPromo, couponID}, fs.coupons("sub_b"),
		"the merchant's own promo must survive the platform override")
}

// The card-less trialing tenant. stripe_subscription_id is NULL, so there is
// nothing to attach the coupon to — and that is a reported outcome with its
// own name, not a failure and not a silent skip.
func TestApply_AStoreWithNoStripeSubscriptionIsReportedPending(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	withSub := seedStoreWithSubscription(t, db, tenantID, "sub_a")
	cardless := seedStoreWithSubscription(t, db, tenantID, "")

	fs := newFakeStripe()
	res, err := newService(t, db, fs, realEmitter(t, db)).
		Apply(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})
	require.NoError(t, err)

	got := outcomes(res)
	require.Equal(t, tenantdiscount.OutcomePending, got[cardless.StoreID].Outcome)
	require.Equal(t, tenantdiscount.OutcomeApplied, got[withSub.StoreID].Outcome,
		"a pending sibling must not hold the rest of the fan-out back")

	require.Equal(t, []string{"sub_a/" + couponID}, fs.adds,
		"the pending store has no stripe subscription id, so it must produce no Stripe call")

	require.EqualValues(t, 1, auditRowsForStore(t, db, cardless.StoreID, tenantdiscount.ActionApply),
		"a pending store is still an audited outcome")
}

// A store with no store_subscriptions row at all. The plan is explicit that
// this must be an outcome rather than a store the loop never visits.
func TestApply_AStoreWithNoSubscriptionRowIsAnExplicitOutcome(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	withSub := seedStoreWithSubscription(t, db, tenantID, "sub_a")
	bare := seedStoreWithoutSubscription(t, db, tenantID)

	res, err := newService(t, db, newFakeStripe(), realEmitter(t, db)).
		Apply(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})
	require.NoError(t, err)

	got := outcomes(res)
	require.Len(t, res.Stores, 2)
	require.Equal(t, tenantdiscount.OutcomeNoSubscription, got[bare].Outcome)
	require.Equal(t, tenantdiscount.OutcomeApplied, got[withSub.StoreID].Outcome)
	require.EqualValues(t, 1, auditRowsForStore(t, db, bare, tenantdiscount.ActionApply))
}

// An operator applying an override to a tenant with no stores achieves
// nothing, and reporting an empty success would read as "done".
func TestApply_RefusesATenantWithNoStores(t *testing.T) {
	db := newDB(t)

	_, err := newService(t, db, newFakeStripe(), realEmitter(t, db)).
		Apply(context.Background(), tenantdiscount.Input{TenantID: uuid.New(), CouponID: couponID})
	require.ErrorIs(t, err, tenantdiscount.ErrNoStores)
}

// ---------------------------------------------------------------------------
// One transaction PER STORE
// ---------------------------------------------------------------------------

// The load-bearing isolation assertion. One transaction across N stores would
// let one store's Stripe failure roll the others back; this pins that it does
// not — the siblings keep both their Stripe discount and their committed audit
// row, and only the failing store is reported failed.
func TestApply_OneStoreFailingLeavesTheOthersAppliedAndAudited(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	first := seedStoreWithSubscription(t, db, tenantID, "sub_a")
	broken := seedStoreWithSubscription(t, db, tenantID, "sub_bad")
	last := seedStoreWithSubscription(t, db, tenantID, "sub_c")

	fs := newFakeStripe()
	fs.fail("sub_bad", errStripeDown)

	res, err := newService(t, db, fs, realEmitter(t, db)).
		Apply(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})
	require.NoError(t, err, "a per-store failure is reported in the Result, not as a fan-out error")

	got := outcomes(res)
	require.Equal(t, tenantdiscount.OutcomeFailed, got[broken.StoreID].Outcome)
	require.ErrorIs(t, got[broken.StoreID].Err, tenantdiscount.ErrStripeCall)
	require.EqualValues(t, 0, auditRowsForStore(t, db, broken.StoreID, tenantdiscount.ActionApply),
		"the failing store's transaction must roll its audit row back")

	for _, s := range []seededStore{first, last} {
		require.Equal(t, tenantdiscount.OutcomeApplied, got[s.StoreID].Outcome,
			"a sibling's failure must neither roll back nor block this store")
		require.True(t, fs.has(s.StripeSubscriptionID, couponID))
		require.EqualValues(t, 1, auditRowsForStore(t, db, s.StoreID, tenantdiscount.ActionApply))
	}
}

// ---------------------------------------------------------------------------
// The audit row is written LAST, inside the store's transaction
// ---------------------------------------------------------------------------

// rollbackAfterAudit writes the real audit row through the caller's
// transaction and THEN fails. Nothing follows EmitTx inside the transaction,
// so this is the only way to force a rollback after a successful audit insert
// — and forcing exactly that is what separates "written on the transaction"
// from "written on the emitter's own handle", which no post-hoc count can
// tell apart on the happy path.
type rollbackAfterAudit struct {
	inner  *audit.Emitter
	forSub string // the stripe subscription id whose store must roll back

	// seenInTx records the row count observed THROUGH the caller's
	// transaction immediately after the insert. Without it, an EmitTx that
	// wrote nothing at all would satisfy the post-rollback assertion.
	seenInTx int64
}

func (r *rollbackAfterAudit) EmitTx(ctx context.Context, tx *gorm.DB, c *gin.Context, ev audit.Event) error {
	if err := r.inner.EmitTx(ctx, tx, c, ev); err != nil {
		return err
	}
	if sub, _ := ev.Metadata["stripe_subscription_id"].(string); sub != r.forSub {
		return nil
	}
	if err := tx.Model(&audit.Entry{}).
		Where("store_id = ?", ev.StoreID).Count(&r.seenInTx).Error; err != nil {
		return err
	}
	return errAuditWriteRejected
}

var errAuditWriteRejected = errors.New("audit write rejected")

func TestApply_TheAuditRowRollsBackWithItsStoresTransaction(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	doomed := seedStoreWithSubscription(t, db, tenantID, "sub_doomed")
	survivor := seedStoreWithSubscription(t, db, tenantID, "sub_ok")

	fs := newFakeStripe()
	aw := &rollbackAfterAudit{inner: realEmitter(t, db), forSub: "sub_doomed"}

	res, err := newService(t, db, fs, aw).
		Apply(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})
	require.NoError(t, err)

	require.EqualValues(t, 1, aw.seenInTx,
		"the row must be visible inside the store's own transaction — otherwise EmitTx wrote nothing and the assertion below is vacuous")
	require.EqualValues(t, 0, auditRowsForStore(t, db, doomed.StoreID, tenantdiscount.ActionApply),
		"the audit row must die with the transaction that carried it")
	require.EqualValues(t, 1, auditRowsForStore(t, db, survivor.StoreID, tenantdiscount.ActionApply),
		"the sibling store's own transaction committed independently")

	got := outcomes(res)
	require.Equal(t, tenantdiscount.OutcomeFailed, got[doomed.StoreID].Outcome)
	require.Equal(t, tenantdiscount.OutcomeApplied, got[survivor.StoreID].Outcome)
}

// The residual divergence, named. The transaction rolled back but Stripe still
// holds the discount, so the operator gets a distinct sentinel rather than a
// routine database error — the same treatment trial's
// ErrStripeAppliedLocalWriteFailed gets.
func TestApply_AnUnattributableDiscountIsNamedNotSwallowed(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	doomed := seedStoreWithSubscription(t, db, tenantID, "sub_doomed")

	fs := newFakeStripe()
	aw := &rollbackAfterAudit{inner: realEmitter(t, db), forSub: "sub_doomed"}

	res, err := newService(t, db, fs, aw).
		Apply(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})
	require.NoError(t, err)

	got := outcomes(res)[doomed.StoreID]
	require.Equal(t, tenantdiscount.OutcomeFailed, got.Outcome)
	require.ErrorIs(t, got.Err, tenantdiscount.ErrStripeChangedAuditWriteFailed)
	require.ErrorIs(t, got.Err, errAuditWriteRejected, "the underlying cause must stay reachable")
	require.Equal(t, tenantdiscount.FailureAuditWrite, got.FailureCode)

	require.True(t, fs.has("sub_doomed", couponID),
		"the local rollback cannot and does not undo Stripe — that is the divergence being named")
	require.Contains(t, got.Err.Error(), couponID)
	require.Contains(t, got.Err.Error(), "sub_doomed")
	require.Contains(t, got.Err.Error(), got.StripeCustomerID)
}

// An audit failure on a store where Stripe was NOT changed is an ordinary
// failure, not the divergence. Without this control the sentinel above could
// be returned for every audit failure and still look correct.
func TestApply_AnAuditFailureWithoutAStripeChangeIsNotTheDivergence(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	already := seedStoreWithSubscription(t, db, tenantID, "sub_already")

	fs := newFakeStripe()
	fs.attach("sub_already", couponID) // already there; this apply changes nothing
	aw := &rollbackAfterAudit{inner: realEmitter(t, db), forSub: "sub_already"}

	res, err := newService(t, db, fs, aw).
		Apply(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})
	require.NoError(t, err)

	got := outcomes(res)[already.StoreID]
	require.Equal(t, tenantdiscount.OutcomeFailed, got.Outcome)
	require.NotErrorIs(t, got.Err, tenantdiscount.ErrStripeChangedAuditWriteFailed)
	require.ErrorIs(t, got.Err, errAuditWriteRejected)
}

// ---------------------------------------------------------------------------
// Idempotency
// ---------------------------------------------------------------------------

// Applying twice must not report the second run as a fresh application, and
// must not send a second attach. This is idempotent APPLICATION, which is not
// the "at most one active override per tenant" guarantee #660 asserts — the
// local record migration 000132 added bounds what this service applied, not
// what the Stripe customer carries.
func TestApply_ReAppliedCouponReportsAlreadyApplied(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	store := seedStoreWithSubscription(t, db, tenantID, "sub_a")

	fs := newFakeStripe()
	svc := newService(t, db, fs, realEmitter(t, db))
	in := tenantdiscount.Input{TenantID: tenantID, CouponID: couponID}

	first, err := svc.Apply(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, tenantdiscount.OutcomeApplied, outcomes(first)[store.StoreID].Outcome)

	second, err := svc.Apply(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, tenantdiscount.OutcomeAlreadyApplied, outcomes(second)[store.StoreID].Outcome)

	require.Equal(t, []string{"sub_a/" + couponID}, fs.adds,
		"the second apply must not send a second attach")
	require.EqualValues(t, 2, auditRowsForStore(t, db, store.StoreID, tenantdiscount.ActionApply),
		"both operator actions are audited; only one of them changed Stripe")
}

// ---------------------------------------------------------------------------
// Remove
// ---------------------------------------------------------------------------

// Removal fans out the same way and is as audited as the application
// (tesserix-home#331), and it takes out only the platform coupon.
func TestRemove_FansOutAndLeavesTheMerchantPromoAlone(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	a := seedStoreWithSubscription(t, db, tenantID, "sub_a")
	b := seedStoreWithSubscription(t, db, tenantID, "sub_b")
	cardless := seedStoreWithSubscription(t, db, tenantID, "")

	fs := newFakeStripe()
	fs.attach("sub_a", couponID)
	fs.attach("sub_b", couponID)
	fs.attach("sub_b", merchantPromo)

	res, err := newService(t, db, fs, realEmitter(t, db)).
		Remove(context.Background(), tenantdiscount.Input{
			TenantID: tenantID, CouponID: couponID, Reason: "override expired",
		})
	require.NoError(t, err)

	got := outcomes(res)
	require.Equal(t, tenantdiscount.OutcomeRemoved, got[a.StoreID].Outcome)
	require.Equal(t, tenantdiscount.OutcomeRemoved, got[b.StoreID].Outcome)
	require.Equal(t, tenantdiscount.OutcomePending, got[cardless.StoreID].Outcome)

	require.False(t, fs.has("sub_a", couponID))
	require.Equal(t, []string{merchantPromo}, fs.coupons("sub_b"),
		"removal must take out the platform coupon and nothing else")

	for _, id := range []uuid.UUID{a.StoreID, b.StoreID, cardless.StoreID} {
		require.EqualValues(t, 1, auditRowsForStore(t, db, id, tenantdiscount.ActionRemove))
	}
}

// A coupon that is not attached is reported as such rather than as a removal
// that happened, so a replayed removal cannot read as a fresh one.
func TestRemove_ACouponThatIsNotAttachedReportsNotApplied(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	store := seedStoreWithSubscription(t, db, tenantID, "sub_a")

	fs := newFakeStripe()
	fs.attach("sub_a", merchantPromo)

	res, err := newService(t, db, fs, realEmitter(t, db)).
		Remove(context.Background(), tenantdiscount.Input{TenantID: tenantID, CouponID: couponID})
	require.NoError(t, err)

	require.Equal(t, tenantdiscount.OutcomeNotApplied, outcomes(res)[store.StoreID].Outcome)
	require.Empty(t, fs.removes, "nothing to remove means no update is sent")
	require.Equal(t, []string{merchantPromo}, fs.coupons("sub_a"))
}

// ---------------------------------------------------------------------------
// Attribution
// ---------------------------------------------------------------------------

// The audit row must carry the operator and the facts a human needs to
// reconcile: which coupon, which Stripe subscription, which customer, and the
// operator's stated reason.
func TestApply_TheAuditRowCarriesTheOperatorAndTheStripeFacts(t *testing.T) {
	db := newDB(t)
	tenantID := uuid.New()
	store := seedStoreWithSubscription(t, db, tenantID, "sub_a")

	_, err := newService(t, db, newFakeStripe(), realEmitter(t, db)).
		Apply(context.Background(), tenantdiscount.Input{
			TenantID: tenantID, CouponID: couponID, Reason: "negotiated 2026 rate",
			C: operatorContext("op_42"),
		})
	require.NoError(t, err)

	var row audit.Entry
	require.NoError(t, db.Where("store_id = ? AND action = ?", store.StoreID, tenantdiscount.ActionApply).
		First(&row).Error)

	require.Equal(t, audit.ActorOperator, row.ActorType)
	require.NotNil(t, row.ActorOperatorID)
	require.Equal(t, "op_42", *row.ActorOperatorID)
	require.Equal(t, tenantID, row.TenantID)
	require.Equal(t, couponID, row.Metadata["coupon_id"])
	require.Equal(t, string(tenantdiscount.OutcomeApplied), row.Metadata["outcome"])
	require.Equal(t, "sub_a", row.Metadata["stripe_subscription_id"])
	require.Equal(t, "negotiated 2026 rate", row.Metadata["reason"])
	require.NotEmpty(t, row.Metadata["stripe_customer_id"])
}
