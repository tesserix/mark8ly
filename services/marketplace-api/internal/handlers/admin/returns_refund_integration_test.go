//go:build integration

// Package admin_test — Task 10 wired ReturnsHandler.MarkRefunded to
// orderrefund.Coordinator so return refunds move real money, mirroring the
// order-level refund wired in Task 9 (orders_refund_integration_test.go).
// This file drives the full return lifecycle (request → approve →
// received → refunded) over HTTP against setupOrdersRouter, which already
// wires returnsHandler with a Coordinator built from a fakeGateway.
package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/payment"
)

// driveReturnToReceived creates an order item-scoped return, approves it,
// and marks it received, returning the return id and the order item id it
// covers. base is the /admin/stores/:storeId/orders path prefix.
func driveReturnToReceived(t *testing.T, env *ordersTestEnv, base, orderID, orderItemID string, headers map[string]string) string {
	t.Helper()

	createBody := map[string]any{
		"currency_code": "USD",
		"items": []map[string]any{
			{"order_item_id": orderItemID, "quantity": 1, "reason": "damaged"},
		},
	}
	w := request(t, env.router, http.MethodPost, base+"/"+orderID+"/returns", createBody, headers)
	if w.Code != http.StatusCreated {
		t.Fatalf("return request: status %d body=%s", w.Code, w.Body.String())
	}
	var created admin.AdminReturnResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("return request: unmarshal: %v", err)
	}

	returnsBase := "/api/v1/admin/stores/" + envStoreIDFromBase(base) + "/returns"

	w = request(t, env.router, http.MethodPost, returnsBase+"/"+created.ID+"/approve", map[string]any{}, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("return approve: status %d body=%s", w.Code, w.Body.String())
	}

	w = request(t, env.router, http.MethodPost, returnsBase+"/"+created.ID+"/received", map[string]any{}, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("return received: status %d body=%s", w.Code, w.Body.String())
	}

	return created.ID
}

// envStoreIDFromBase extracts the storeId path segment from an
// /admin/stores/:storeId/orders base path.
func envStoreIDFromBase(base string) string {
	const prefix = "/api/v1/admin/stores/"
	rest := base[len(prefix):]
	end := 0
	for end < len(rest) && rest[end] != '/' {
		end++
	}
	return rest[:end]
}

// TestAPI_ReturnRefund_MovesMoney drives request→approve→received→refunded
// over HTTP and asserts the refund actually moved money through the
// coordinator: a succeeded refund_transactions row keyed off the return id,
// orders.refunded_amount bumped, and the return stamped refunded.
func TestAPI_ReturnRefund_MovesMoney(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	// Seed a PAID order with a captured payment_transaction + active
	// gateway config so the coordinator can resolve a refund target.
	orderID := createAndConfirmOrder(t, env, base, headers, "120.00")
	seedCapturedPaymentTxn(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID), uuid.MustParse(orderID), "120.00")
	seedActiveGatewayConfig(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID))

	// Fetch the order to grab its single item's id for the return request.
	w := request(t, env.router, http.MethodGet, base+"/"+orderID, nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("get order: status %d body=%s", w.Code, w.Body.String())
	}
	var got admin.AdminOrderResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("get order: unmarshal: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected 1 order item, got %d", len(got.Items))
	}
	orderItemID := got.Items[0].ID

	returnID := driveReturnToReceived(t, env, base, orderID, orderItemID, headers)

	refundedBody := map[string]any{"amount": "50.00", "reason": "item returned"}
	w = request(t, env.router, http.MethodPost, "/api/v1/admin/stores/"+storeID+"/returns/"+returnID+"/refunded", refundedBody, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("mark refunded: status %d body=%s", w.Code, w.Body.String())
	}
	var refunded admin.AdminReturnResponse
	if err := json.Unmarshal(w.Body.Bytes(), &refunded); err != nil {
		t.Fatalf("mark refunded: unmarshal: %v", err)
	}
	if refunded.Status != "refunded" {
		t.Fatalf("return status = %q, want refunded", refunded.Status)
	}
	if refunded.RefundAmount == nil || !refunded.RefundAmount.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("return refund_amount = %v, want 50", refunded.RefundAmount)
	}

	// --- refund_transactions row ---
	wantKey := "refund_" + orderID + "_" + returnID
	var txn payment.RefundTransaction
	if err := env.db.Where("idempotency_key = ?", wantKey).First(&txn).Error; err != nil {
		t.Fatalf("refund_transactions lookup: %v", err)
	}
	if txn.Status != "succeeded" {
		t.Fatalf("refund_transactions.status = %q, want succeeded", txn.Status)
	}
	if txn.OrderID != orderID {
		t.Fatalf("refund_transactions.order_id = %q, want %q", txn.OrderID, orderID)
	}

	// --- orders.refunded_amount ---
	w = request(t, env.router, http.MethodGet, base+"/"+orderID, nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("get order (post-refund): status %d body=%s", w.Code, w.Body.String())
	}
	var postRefundOrder admin.AdminOrderResponse
	if err := json.Unmarshal(w.Body.Bytes(), &postRefundOrder); err != nil {
		t.Fatalf("get order (post-refund): unmarshal: %v", err)
	}
	if !postRefundOrder.RefundedAmount.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("orders.refunded_amount = %s, want 50", postRefundOrder.RefundedAmount.String())
	}
}

// TestAPI_ReturnRefund_NoCapturedPayment_Unavailable asserts that refunding
// a received return whose order has no captured payment_transactions row
// is rejected (422 refund_unavailable) BEFORE the return is ever stamped
// refunded — the return must stay in 'received'.
func TestAPI_ReturnRefund_NoCapturedPayment_Unavailable(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	orderID := createAndConfirmOrder(t, env, base, headers, "120.00")
	// Deliberately do NOT seed payment_transactions or payment_gateway_configs.

	w := request(t, env.router, http.MethodGet, base+"/"+orderID, nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("get order: status %d body=%s", w.Code, w.Body.String())
	}
	var got admin.AdminOrderResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("get order: unmarshal: %v", err)
	}
	orderItemID := got.Items[0].ID

	returnID := driveReturnToReceived(t, env, base, orderID, orderItemID, headers)

	refundedBody := map[string]any{"amount": "50.00", "reason": "item returned"}
	w = request(t, env.router, http.MethodPost, "/api/v1/admin/stores/"+storeID+"/returns/"+returnID+"/refunded", refundedBody, headers)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mark refunded: status %d, want 422 (body=%s)", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("mark refunded: unmarshal: %v", err)
	}
	if body["error"] != "refund_unavailable" {
		t.Fatalf("mark refunded: error = %v, want refund_unavailable", body["error"])
	}

	// The return must NOT have been stamped refunded.
	w = request(t, env.router, http.MethodGet, "/api/v1/admin/stores/"+storeID+"/returns/"+returnID, nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("get return: status %d body=%s", w.Code, w.Body.String())
	}
	var r admin.AdminReturnResponse
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("get return: unmarshal: %v", err)
	}
	if r.Status != "received" {
		t.Fatalf("return status = %q, want received (unchanged)", r.Status)
	}
}
