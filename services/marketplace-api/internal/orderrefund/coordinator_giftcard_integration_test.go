//go:build integration

package orderrefund_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// Gap B: at checkout the gift-card amount is subtracted from grand_total
// BEFORE the order row is written, so a gateway refund returns the gateway
// portion and the store-credit portion silently vanishes. Worse, the old
// refund cap was grand_total — which excludes the gift card — so the card's
// share was not merely unreturned, it was unrequestable.
//
// Run: TEST_DATABASE_URL=... go test -tags=integration ./internal/orderrefund/...

func dc(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// seedGiftCard inserts a card in an explicit state.
func seedGiftCard(t *testing.T, db *gorm.DB, storeID, tenantID uuid.UUID, status giftcard.GiftCardStatus, initial, balance string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := db.Exec(`
		INSERT INTO gift_cards (id, tenant_id, store_id, code, initial_balance,
		                        current_balance, currency_code, status)
		VALUES (?, ?, ?, ?, ?, ?, 'EUR', ?)`,
		id, tenantID, storeID, "GC"+id.String()[:10], initial, balance, status).Error
	if err != nil {
		t.Fatalf("seedGiftCard: %v", err)
	}
	return id
}

// applyGiftCardToOrder replays what checkout does: debit the card against the
// order, leaving the `redeem` ledger row that records the store credit the
// order was part-paid with.
func applyGiftCardToOrder(t *testing.T, db *gorm.DB, cardID, orderID, tenantID uuid.UUID, amount string) {
	t.Helper()
	if _, err := giftcard.NewRepository().DebitInTx(db, cardID, dc(amount), orderID, tenantID); err != nil {
		t.Fatalf("applyGiftCardToOrder: %v", err)
	}
}

func cardState(t *testing.T, db *gorm.DB, cardID uuid.UUID) (decimal.Decimal, giftcard.GiftCardStatus) {
	t.Helper()
	var row struct {
		CurrentBalance decimal.Decimal `gorm:"column:current_balance"`
		Status         giftcard.GiftCardStatus
	}
	if err := db.Table("gift_cards").Where("id = ?", cardID).Take(&row).Error; err != nil {
		t.Fatalf("cardState: %v", err)
	}
	return row.CurrentBalance, row.Status
}

func countCardRefundRows(t *testing.T, db *gorm.DB, orderID uuid.UUID) int64 {
	t.Helper()
	var n int64
	if err := db.Table("gift_card_transactions").
		Where("order_id = ? AND type = 'refund'", orderID).Count(&n).Error; err != nil {
		t.Fatalf("countCardRefundRows: %v", err)
	}
	return n
}

func countOrderEvents(t *testing.T, db *gorm.DB, orderID uuid.UUID, kind order.EventKind) int64 {
	t.Helper()
	var n int64
	if err := db.Table("order_events").
		Where("order_id = ? AND kind = ?", orderID, string(kind)).Count(&n).Error; err != nil {
		t.Fatalf("countOrderEvents: %v", err)
	}
	return n
}

// splitOrder seeds the brief's worked example: a €100 basket paid with a €40
// gift card and €60 on the gateway, so the order row carries grand_total 60.
func splitOrder(t *testing.T, db *gorm.DB) (tenantID, storeID, orderID, cardID uuid.UUID) {
	t.Helper()
	tenantID, storeID, orderID = uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "60.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "60.00", Status: "captured",
	})
	cardID = seedGiftCard(t, db, storeID, tenantID, giftcard.StatusActive, "100.00", "100.00")
	applyGiftCardToOrder(t, db, cardID, orderID, tenantID, "40.00")
	return tenantID, storeID, orderID, cardID
}

func refund(t *testing.T, c *orderrefund.Coordinator, orderID uuid.UUID, amount, scope string) orderrefund.RefundResult {
	t.Helper()
	amt := dc(amount)
	res, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: &amt, Reason: "customer request", Actor: "admin", ScopeID: scope,
	})
	if err != nil {
		t.Fatalf("Refund(%s, %s): %v", amount, scope, err)
	}
	return res
}

// 1. The headline fixture: FOUR sequential partial refunds on one order,
// crossing the gateway/gift-card boundary mid-sequence. A single full refund
// would prove almost nothing here.
func TestRefund_SequentialPartials_CrossGiftCardBoundary(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	_, _, orderID, cardID := splitOrder(t, db)
	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	// Card started at 100 and paid 40 towards the order.
	if bal, _ := cardState(t, db, cardID); !bal.Equal(dc("60.00")) {
		t.Fatalf("precondition: card balance = %s, want 60.00", bal)
	}

	// (a) €30 — wholly inside the €60 gateway portion.
	r := refund(t, c, orderID, "30.00", "rr1")
	if !r.GiftCardAmount.IsZero() {
		t.Fatalf("refund #1 credited %s to the card; real money must come back first", r.GiftCardAmount)
	}
	if bal, _ := cardState(t, db, cardID); !bal.Equal(dc("60.00")) {
		t.Fatalf("card balance = %s after refund #1, want 60.00 (untouched)", bal)
	}

	// (b) €30 — exactly exhausts the gateway.
	refund(t, c, orderID, "30.00", "rr2")
	if o := getOrder(t, db, orderID); !o.RefundedAmount.Equal(dc("60.00")) {
		t.Fatalf("refunded_amount = %s, want 60.00", o.RefundedAmount)
	}
	if bal, _ := cardState(t, db, cardID); !bal.Equal(dc("60.00")) {
		t.Fatalf("card balance = %s after refund #2, want 60.00 (gateway only just exhausted)", bal)
	}
	if gw.callCount() != 2 {
		t.Fatalf("gateway calls = %d, want 2", gw.callCount())
	}

	// (c) €30 — the gateway is gone; this is pure store credit.
	r = refund(t, c, orderID, "30.00", "rr3")
	if !r.GiftCardAmount.Equal(dc("30.00")) {
		t.Fatalf("refund #3 GiftCardAmount = %s, want 30.00", r.GiftCardAmount)
	}
	if bal, status := cardState(t, db, cardID); !bal.Equal(dc("90.00")) || status != giftcard.StatusActive {
		t.Fatalf("card = %s/%s after refund #3, want 90.00/active", bal, status)
	}
	if gw.callCount() != 2 {
		t.Fatalf("gateway calls = %d after a store-credit-only refund, want 2 — the provider must not be asked to refund zero", gw.callCount())
	}

	// (d) €10 — finishes the card.
	refund(t, c, orderID, "10.00", "rr4")
	if bal, _ := cardState(t, db, cardID); !bal.Equal(dc("100.00")) {
		t.Fatalf("card balance = %s after refund #4, want 100.00 — the customer is whole again", bal)
	}
	// The gateway side never over-refunded.
	if o := getOrder(t, db, orderID); !o.RefundedAmount.Equal(dc("60.00")) {
		t.Fatalf("refunded_amount = %s after all four, want 60.00 (gateway money only)", o.RefundedAmount)
	}

	// (e) Nothing is left on either ledger.
	amt := dc("0.01")
	_, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: &amt, ScopeID: "rr5",
	})
	if !errors.Is(err, apperrors.ErrRefundExceedsTotal) {
		t.Fatalf("fifth refund => %v, want ErrRefundExceedsTotal", err)
	}
}

// 2. A nil Amount ("refund everything") must now reach the gift-card portion.
// Before, the cap was grand_total, so the card's share was unrequestable.
func TestRefund_FullRefundReturnsGiftCardPortion(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	_, _, orderID, cardID := splitOrder(t, db)
	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	res, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: nil, Reason: "cancel", ScopeID: "cancel",
	})
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if !res.Amount.Equal(dc("100.00")) {
		t.Fatalf("Amount = %s, want 100.00 — a full refund covers both ledgers", res.Amount)
	}
	if !res.GiftCardAmount.Equal(dc("40.00")) {
		t.Fatalf("GiftCardAmount = %s, want 40.00", res.GiftCardAmount)
	}
	if !gw.lastCall().Amount.Equal(dc("60.00")) {
		t.Fatalf("gateway asked for %s, want 60.00 — never more than it captured", gw.lastCall().Amount)
	}
	if bal, _ := cardState(t, db, cardID); !bal.Equal(dc("100.00")) {
		t.Fatalf("card balance = %s, want 100.00", bal)
	}
	if res.PaymentStatus != order.PaymentStatusRefunded {
		t.Fatalf("PaymentStatus = %q, want refunded", res.PaymentStatus)
	}
	if n := countOrderEvents(t, db, orderID, order.EventKindGiftCardCredited); n != 1 {
		t.Fatalf("gift_card_credited events = %d, want 1 — the store-credit half must be visible on the order", n)
	}
}

// 3. Idempotency. Webhooks and retries redeliver; a second identical refund
// must not double-credit or write a second ledger row.
func TestRefund_GiftCardCreditIsIdempotent(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	_, _, orderID, cardID := splitOrder(t, db)
	c := newCoordinator(db, &fakeGateway{}, true)

	first := refund(t, c, orderID, "100.00", "rr1")
	balAfterFirst, _ := cardState(t, db, cardID)
	if !balAfterFirst.Equal(dc("100.00")) {
		t.Fatalf("card balance = %s after the first refund, want 100.00", balAfterFirst)
	}
	if n := countCardRefundRows(t, db, orderID); n != 1 {
		t.Fatalf("gift card refund rows = %d after the first refund, want 1", n)
	}

	second := refund(t, c, orderID, "100.00", "rr1")
	if !second.AlreadyDone {
		t.Fatalf("replay AlreadyDone = false, want true")
	}
	if bal, _ := cardState(t, db, cardID); !bal.Equal(dc("100.00")) {
		t.Fatalf("card balance = %s after the replay, want 100.00 — the credit was applied twice", bal)
	}
	if n := countCardRefundRows(t, db, orderID); n != 1 {
		t.Fatalf("gift card refund rows = %d after the replay, want 1 — a duplicate ledger row was written", n)
	}
	if n := countRefundTxns(t, db, orderID); n != 1 {
		t.Fatalf("refund ledger rows = %d, want 1", n)
	}
	if first.ProviderRefundID == "" {
		t.Fatalf("first refund has no provider refund id")
	}
}

// 4. The subtle one: a card fully spent on the order sits at `depleted`, and
// DebitInTx pins status = 'active'. Crediting it without flipping it back
// leaves a restored balance that can never be spent.
func TestRefund_DepletedCardFlipsBackToActive(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "60.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "60.00", Status: "captured",
	})
	// The card's whole balance went into this order.
	cardID := seedGiftCard(t, db, storeID, tenantID, giftcard.StatusActive, "40.00", "40.00")
	applyGiftCardToOrder(t, db, cardID, orderID, tenantID, "40.00")
	if _, status := cardState(t, db, cardID); status != giftcard.StatusDepleted {
		t.Fatalf("precondition: card status = %q, want depleted", status)
	}

	c := newCoordinator(db, &fakeGateway{}, true)
	refund(t, c, orderID, "100.00", "rr1")

	bal, status := cardState(t, db, cardID)
	if !bal.Equal(dc("40.00")) {
		t.Fatalf("card balance = %s, want 40.00", bal)
	}
	if status != giftcard.StatusActive {
		t.Fatalf("card status = %q, want active — a restored balance on a depleted card is unspendable money", status)
	}

	// Prove it: the money is actually spendable again.
	if _, err := giftcard.NewRepository().DebitInTx(db, cardID, dc("40.00"), uuid.New(), tenantID); err != nil {
		t.Fatalf("restored balance is not spendable: %v", err)
	}
}

// 5. A card whose OWN purchase was refunded must NOT be credited — the
// customer already had that money back in real currency, so crediting it
// would create value from nothing. The refund itself must still succeed, and
// the gap must be visible to the merchant.
func TestRefund_RefundedCardIsNotCredited(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID, cardID := splitOrder(t, db)
	_ = tenantID
	_ = storeID

	// The buyer later got their gift-card purchase refunded, voiding the card.
	if err := db.Exec(
		`UPDATE gift_cards SET status = 'refunded', current_balance = 0 WHERE id = ?`, cardID).Error; err != nil {
		t.Fatalf("void card: %v", err)
	}

	gw := &fakeGateway{}
	c := newCoordinator(db, gw, true)

	res, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: nil, Reason: "cancel", ScopeID: "cancel",
	})
	if err != nil {
		t.Fatalf("a gift-card problem must never fail a refund the gateway already executed: %v", err)
	}
	if !gw.lastCall().Amount.Equal(dc("60.00")) {
		t.Fatalf("gateway asked for %s, want 60.00", gw.lastCall().Amount)
	}
	if o := getOrder(t, db, orderID); !o.RefundedAmount.Equal(dc("60.00")) {
		t.Fatalf("refunded_amount = %s, want 60.00 — the gateway refund must still be recorded", o.RefundedAmount)
	}
	if bal, status := cardState(t, db, cardID); !bal.IsZero() || status != giftcard.StatusRefunded {
		t.Fatalf("card = %s/%s, want 0/refunded — a refunded card must not be credited", bal, status)
	}
	if n := countCardRefundRows(t, db, orderID); n != 0 {
		t.Fatalf("gift card refund rows = %d, want 0", n)
	}
	if n := countOrderEvents(t, db, orderID, order.EventKindGiftCardCreditSkipped); n != 1 {
		t.Fatalf("gift_card_credit_skipped events = %d, want 1 — the merchant must be able to see this, not just grep logs", n)
	}
	_ = res
}

// 6. A disabled card keeps its balance frozen but not destroyed, so a refund
// still credits it — the value becomes spendable again on re-enable.
func TestRefund_DisabledCardIsStillCredited(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	_, _, orderID, cardID := splitOrder(t, db)
	if err := db.Exec(`UPDATE gift_cards SET status = 'disabled' WHERE id = ?`, cardID).Error; err != nil {
		t.Fatalf("disable card: %v", err)
	}

	c := newCoordinator(db, &fakeGateway{}, true)
	refund(t, c, orderID, "100.00", "rr1")

	bal, status := cardState(t, db, cardID)
	if !bal.Equal(dc("100.00")) {
		t.Fatalf("card balance = %s, want 100.00 — a disabled card's balance is frozen, not destroyed", bal)
	}
	if status != giftcard.StatusDisabled {
		t.Fatalf("card status = %q, want disabled — crediting must not silently re-enable a card the merchant froze", status)
	}
}

// 7. Two cards paid for one order. The credit must be distributed by what
// each card actually contributed — pouring the whole owed amount into the
// first card would over-credit it and leave the second one short.
func TestRefund_MultipleCardsAreCreditedByContribution(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "60.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "60.00", Status: "captured",
	})
	cardA := seedGiftCard(t, db, storeID, tenantID, giftcard.StatusActive, "100.00", "100.00")
	cardB := seedGiftCard(t, db, storeID, tenantID, giftcard.StatusActive, "100.00", "100.00")
	applyGiftCardToOrder(t, db, cardA, orderID, tenantID, "15.00")
	applyGiftCardToOrder(t, db, cardB, orderID, tenantID, "25.00")

	c := newCoordinator(db, &fakeGateway{}, true)
	res := refund(t, c, orderID, "100.00", "rr1")
	if !res.GiftCardAmount.Equal(dc("40.00")) {
		t.Fatalf("GiftCardAmount = %s, want 40.00", res.GiftCardAmount)
	}

	if bal, _ := cardState(t, db, cardA); !bal.Equal(dc("100.00")) {
		t.Fatalf("card A balance = %s, want 100.00 (it contributed 15 and gets 15 back)", bal)
	}
	if bal, _ := cardState(t, db, cardB); !bal.Equal(dc("100.00")) {
		t.Fatalf("card B balance = %s, want 100.00 (it contributed 25 and gets 25 back)", bal)
	}
	if n := countCardRefundRows(t, db, orderID); n != 2 {
		t.Fatalf("gift card refund rows = %d, want 2 — one per contributing card", n)
	}
}

// 8. An order with no gift card behaves exactly as before — the cap is
// grand_total and nothing gift-card-shaped happens.
func TestRefund_NoGiftCardIsUnchanged(t *testing.T) {
	db := testdb.NewDB(t, coordinatorTruncateTables...)
	tenantID, storeID, orderID := uuid.New(), uuid.New(), uuid.New()
	seedStore(t, db, storeID, tenantID)
	seedPaidOrder(t, db, orderID, storeID, tenantID, "120.00", "0")
	seedPaymentTxn(t, db, tenantID, storeID, orderID, seedPaymentTxnOpts{
		Provider: "stripe", ProviderPaymentID: "pi_x", Amount: "120.00", Status: "captured",
	})
	c := newCoordinator(db, &fakeGateway{}, true)

	res := refund(t, c, orderID, "120.00", "rr1")
	if !res.GiftCardAmount.IsZero() {
		t.Fatalf("GiftCardAmount = %s, want 0", res.GiftCardAmount)
	}

	over := dc("0.01")
	if _, err := c.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: orderID, Amount: &over, ScopeID: "rr2",
	}); !errors.Is(err, apperrors.ErrRefundExceedsTotal) {
		t.Fatalf("over-refund => %v, want ErrRefundExceedsTotal", err)
	}
}
