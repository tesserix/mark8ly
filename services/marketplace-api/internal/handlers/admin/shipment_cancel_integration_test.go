//go:build integration

// Package admin_test — end-to-end coverage for the refund → shipment-cancel
// wiring. Proves against a real Postgres (migrations incl. 000096 applied by
// testdb) that a FULL refund drives the shipmentcancel executor, which cancels
// the carrier shipment and records the outcome on the shipments row, while a
// PARTIAL refund leaves the shipment untouched. Reuses the refund suite's
// fakeGateway/seed helpers (orders_refund_integration_test.go) and drives the
// coordinator directly so no real carrier/gateway is dialed.
package admin_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/shipmentcancel"
	"github.com/mark8ly/marketplace-api/internal/shipping"
)

// recordingCarrier implements shipping.Carrier AND shipping.ReturnToOriginer,
// counting CancelShipment / ReturnToOrigin calls. Every other method is unused.
type recordingCarrier struct {
	cancelCalls int
	rtoCalls    int
	revCalls    int
	lastWaybill string
	err         error
	rtoErr      error
}

func (r *recordingCarrier) CreateReverseShipment(_ context.Context, _ shipping.ReverseShipmentRequest) (*shipping.Shipment, error) {
	r.revCalls++
	return &shipping.Shipment{TrackingNumber: "REV-INT", ProviderShipmentID: "REV-INT", Carrier: "delhivery"}, nil
}

func (r *recordingCarrier) CancelShipment(_ context.Context, waybill string) error {
	r.cancelCalls++
	r.lastWaybill = waybill
	return r.err
}
func (r *recordingCarrier) ReturnToOrigin(_ context.Context, waybill string) error {
	r.rtoCalls++
	r.lastWaybill = waybill
	return r.rtoErr
}
func (r *recordingCarrier) GetRates(context.Context, shipping.RateRequest) ([]shipping.Rate, error) {
	return nil, nil
}
func (r *recordingCarrier) CreateShipment(context.Context, shipping.ShipmentRequest) (*shipping.Shipment, error) {
	return nil, nil
}
func (r *recordingCarrier) GetTracking(context.Context, string) (*shipping.Tracking, error) {
	return nil, nil
}
func (r *recordingCarrier) ProviderName() string         { return "delhivery" }
func (r *recordingCarrier) SupportedCountries() []string { return []string{"IN"} }

// seedShipmentWithStatus inserts a delhivery shipment for the order in the
// given lifecycle status ('pending' = pre-pickup, 'in_transit' = picked up,
// etc). ship_from/ship_to are NOT NULL jsonb on the table.
//
// They are seeded with REAL addresses, in the eight-key shape
// handlers/admin/shipments.go writes. They used to be '{}', which was
// scaffolding rather than intent — but since #435 an address with no line1
// is refused by shipmentcancel.parseShipmentAddress, because a GDPR erasure
// strips these very columns to '{}' and a blank address must never reach a
// carrier as a pickup point. A '{}' fixture would now exercise that refusal
// instead of the reverse-pickup path these tests are about.
func seedShipmentWithStatus(t *testing.T, db *gorm.DB, tenantID, storeID, orderID uuid.UUID, waybill, status string) {
	t.Helper()
	const warehouse = `{"name":"Test Warehouse","line1":"12 Depot Road","city":"Mumbai","region":"MH","postal_code":"400001","country_code":"IN","phone":"+911100000000"}`
	const customer = `{"name":"Test Customer","line1":"9 Buyer Street","city":"Pune","region":"MH","postal_code":"411001","country_code":"IN","phone":"+911122222222"}`
	err := db.Exec(
		`INSERT INTO shipments
			(id, tenant_id, store_id, order_id, carrier, tracking_number, status, ship_from, ship_to, currency_code, created_at, updated_at)
		 VALUES (gen_random_uuid(), ?, ?, ?, 'delhivery', ?, ?, ?::jsonb, ?::jsonb, 'INR', now(), now())`,
		tenantID, storeID, orderID, waybill, status, warehouse, customer,
	).Error
	if err != nil {
		t.Fatalf("seedShipmentWithStatus(%s): %v", status, err)
	}
}

// coordinatorWithCanceller builds a real refund Coordinator (fake gateway) with
// the shipment-cancel executor wired to a recording carrier, mirroring the
// production hook in cmd/marketplace-api/main.go but synchronous for assertion.
func coordinatorWithCanceller(db *gorm.DB, car shipping.Carrier) *orderrefund.Coordinator {
	exec := shipmentcancel.NewExecutor(
		shipping.NewRepository(db),
		func(context.Context, uuid.UUID, string) (shipping.Carrier, error) { return car, nil },
		nil,
	)
	return newTestRefundCoordinator(db, &fakeGateway{}).
		WithShipmentCanceller(func(ctx context.Context, oid uuid.UUID) { exec.CancelForOrder(ctx, oid) })
}

func TestFullRefund_CancelsPendingShipment(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	orderID := createAndConfirmOrder(t, env, base, headers, "100.00")
	tUUID, sUUID, oUUID := uuid.MustParse(tenantID), uuid.MustParse(storeID), uuid.MustParse(orderID)
	seedCapturedPaymentTxn(t, env.db, tUUID, sUUID, oUUID, "100.00")
	seedActiveGatewayConfig(t, env.db, tUUID, sUUID)
	seedShipmentWithStatus(t, env.db, tUUID, sUUID, oUUID, "WBN-INT-1", "pending")

	car := &recordingCarrier{}
	coord := coordinatorWithCanceller(env.db, car)

	// Full refund (Amount nil ⇒ full remaining).
	if _, err := coord.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: oUUID, Amount: nil, Reason: "test", Actor: "test", ScopeID: "req-full",
	}); err != nil {
		t.Fatalf("Refund: %v", err)
	}

	if car.cancelCalls != 1 {
		t.Fatalf("carrier CancelShipment calls = %d, want 1", car.cancelCalls)
	}
	if car.lastWaybill != "WBN-INT-1" {
		t.Errorf("cancelled waybill = %q, want WBN-INT-1", car.lastWaybill)
	}

	var row struct {
		CancelAction string
		CancelStatus string
	}
	if err := env.db.Table("shipments").
		Select("cancel_action", "cancel_status").
		Where("order_id = ?", oUUID).
		Scan(&row).Error; err != nil {
		t.Fatalf("reload shipment: %v", err)
	}
	if row.CancelStatus != "succeeded" || row.CancelAction != "cancel_forward" {
		t.Fatalf("shipment cancel state = %s/%s, want cancel_forward/succeeded", row.CancelAction, row.CancelStatus)
	}
}

func TestPartialRefund_DoesNotCancelShipment(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	orderID := createAndConfirmOrder(t, env, base, headers, "100.00")
	tUUID, sUUID, oUUID := uuid.MustParse(tenantID), uuid.MustParse(storeID), uuid.MustParse(orderID)
	seedCapturedPaymentTxn(t, env.db, tUUID, sUUID, oUUID, "100.00")
	seedActiveGatewayConfig(t, env.db, tUUID, sUUID)
	seedShipmentWithStatus(t, env.db, tUUID, sUUID, oUUID, "WBN-INT-2", "pending")

	car := &recordingCarrier{}
	coord := coordinatorWithCanceller(env.db, car)

	half := decimal.RequireFromString("40.00")
	if _, err := coord.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: oUUID, Amount: &half, Reason: "test", Actor: "test", ScopeID: "req-partial",
	}); err != nil {
		t.Fatalf("Refund: %v", err)
	}

	if car.cancelCalls != 0 {
		t.Fatalf("carrier CancelShipment calls = %d on partial refund, want 0", car.cancelCalls)
	}
	var status string
	if err := env.db.Table("shipments").
		Select("cancel_status").
		Where("order_id = ?", oUUID).
		Scan(&status).Error; err != nil {
		t.Fatalf("reload shipment: %v", err)
	}
	if status != "none" {
		t.Fatalf("shipment cancel_status = %q on partial refund, want none", status)
	}
}

func TestFullRefund_InTransit_TriggersRTO(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	orderID := createAndConfirmOrder(t, env, base, headers, "100.00")
	tUUID, sUUID, oUUID := uuid.MustParse(tenantID), uuid.MustParse(storeID), uuid.MustParse(orderID)
	seedCapturedPaymentTxn(t, env.db, tUUID, sUUID, oUUID, "100.00")
	seedActiveGatewayConfig(t, env.db, tUUID, sUUID)
	seedShipmentWithStatus(t, env.db, tUUID, sUUID, oUUID, "WBN-INT-RTO", "in_transit")

	car := &recordingCarrier{}
	coord := coordinatorWithCanceller(env.db, car)

	if _, err := coord.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: oUUID, Amount: nil, Reason: "test", Actor: "test", ScopeID: "req-rto",
	}); err != nil {
		t.Fatalf("Refund: %v", err)
	}

	if car.rtoCalls != 1 || car.cancelCalls != 0 {
		t.Fatalf("rtoCalls=%d cancelCalls=%d, want 1/0", car.rtoCalls, car.cancelCalls)
	}
	var row struct{ CancelAction, CancelStatus string }
	if err := env.db.Table("shipments").
		Select("cancel_action", "cancel_status").
		Where("order_id = ?", oUUID).Scan(&row).Error; err != nil {
		t.Fatalf("reload shipment: %v", err)
	}
	if row.CancelAction != "rto" || row.CancelStatus != "succeeded" {
		t.Fatalf("cancel state = %s/%s, want rto/succeeded", row.CancelAction, row.CancelStatus)
	}
}

func coordinatorWithCancellerReverse(db *gorm.DB, car shipping.Carrier) *orderrefund.Coordinator {
	exec := shipmentcancel.NewExecutor(
		shipping.NewRepository(db),
		func(context.Context, uuid.UUID, string) (shipping.Carrier, error) { return car, nil },
		nil,
	).WithReversePickup(true)
	return newTestRefundCoordinator(db, &fakeGateway{}).
		WithShipmentCanceller(func(ctx context.Context, oid uuid.UUID) { exec.CancelForOrder(ctx, oid) })
}

func TestFullRefund_Delivered_CreatesReversePickup(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	orderID := createAndConfirmOrder(t, env, base, headers, "100.00")
	tUUID, sUUID, oUUID := uuid.MustParse(tenantID), uuid.MustParse(storeID), uuid.MustParse(orderID)
	seedCapturedPaymentTxn(t, env.db, tUUID, sUUID, oUUID, "100.00")
	seedActiveGatewayConfig(t, env.db, tUUID, sUUID)
	seedShipmentWithStatus(t, env.db, tUUID, sUUID, oUUID, "WBN-DELIVERED", "delivered")

	car := &recordingCarrier{}
	coord := coordinatorWithCancellerReverse(env.db, car)

	if _, err := coord.Refund(context.Background(), orderrefund.RefundCommand{
		OrderID: oUUID, Amount: nil, Reason: "test", Actor: "test", ScopeID: "req-rev",
	}); err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if car.revCalls != 1 {
		t.Fatalf("CreateReverseShipment calls = %d, want 1", car.revCalls)
	}
	var fwd struct{ CancelAction, CancelStatus string }
	if err := env.db.Table("shipments").Select("cancel_action", "cancel_status").
		Where("order_id = ? AND tracking_number = ?", oUUID, "WBN-DELIVERED").Scan(&fwd).Error; err != nil {
		t.Fatalf("reload forward: %v", err)
	}
	if fwd.CancelAction != "reverse_pickup" || fwd.CancelStatus != "succeeded" {
		t.Fatalf("forward = %s/%s, want reverse_pickup/succeeded", fwd.CancelAction, fwd.CancelStatus)
	}
	var revCount int64
	if err := env.db.Table("shipments").
		Where("order_id = ? AND tracking_number = ?", oUUID, "REV-INT").Count(&revCount).Error; err != nil {
		t.Fatalf("count reverse leg: %v", err)
	}
	if revCount != 1 {
		t.Fatalf("reverse-leg rows = %d, want 1", revCount)
	}
}
