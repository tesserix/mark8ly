//go:build integration

// Package admin_test — HTTP-level integration tests for the gift card
// admin Disable/Enable endpoints. Mirrors orders_integration_test.go's
// router-scaffold pattern: a real Postgres via pkg/testdb, the full
// middleware chain (header-trust auth + store middleware + FGA), and
// admin.RegisterAdmin wiring only what this suite needs.
package admin_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// giftCardStatusTables is the dependency-ordered truncate list for this
// suite. Children before parents — CASCADE handles the rest.
var giftCardStatusTables = []string{
	"gift_card_transactions",
	"gift_cards",
	"stores",
}

// giftCardStatusTestEnv is a focused router scaffold for the gift card
// disable/enable endpoint suite.
type giftCardStatusTestEnv struct {
	router *gin.Engine
	db     *gorm.DB
	fga    *authz.FakeClient
}

func setupGiftCardStatusRouter(t *testing.T) *giftCardStatusTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, giftCardStatusTables...)

	storesRepo := stores.NewRepository(db)
	repo := giftcard.NewRepository()
	svc := giftcard.NewService(db, repo, nil)

	fga := authz.NewFakeClient()
	storeMW := stores.StoreMiddleware(stores.MiddlewareConfig{
		Repo:   storesRepo,
		Client: stubClient{},
		Flight: &singleflight.Group{},
	})
	authzMW := authz.NewMiddleware(fga, nil)

	handler := admin.NewGiftCardHandler(svc, nil)

	r := gin.New()
	admin.RegisterAdmin(r.Group("/api/v1"), admin.Deps{
		GiftCardHandler:  handler,
		StoresMiddleware: storeMW,
		AuthzMiddleware:  authzMW,
		InternalSecret:   "",
	})

	return &giftCardStatusTestEnv{router: r, db: db, fga: fga}
}

// seedGiftCard inserts a gift_cards row directly (this package cannot
// import giftcard's own test helpers — repository_integration_test.go is
// package giftcard, this file is package admin_test) and returns the
// created row.
func seedGiftCard(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID, status giftcard.GiftCardStatus, expiresAt *time.Time) *giftcard.GiftCard {
	t.Helper()
	gc := &giftcard.GiftCard{
		ID:             uuid.New(),
		TenantID:       tenantID,
		StoreID:        storeID,
		Code:           strings.ToUpper("TESTCODE" + uuid.NewString()[:8]),
		InitialBalance: decimal.NewFromInt(100),
		CurrentBalance: decimal.NewFromInt(100),
		CurrencyCode:   "USD",
		Status:         status,
		ExpiresAt:      expiresAt,
	}
	if err := db.Create(gc).Error; err != nil {
		t.Fatalf("seed gift card: %v", err)
	}
	return gc
}

func giftCardStatusURL(storeID, id, action string) string {
	return "/api/v1/admin/stores/" + storeID + "/gift-cards/" + id + "/" + action
}

// giftCardStatusResp is a narrow view of AdminGiftCardResponse — only the
// fields this suite asserts on.
type giftCardStatusResp struct {
	ID             string          `json:"id"`
	Code           string          `json:"code"`
	CodeDisplay    string          `json:"code_display"`
	CurrentBalance decimal.Decimal `json:"current_balance"`
	Status         string          `json:"status"`
}

type giftCardStatusEnvelope struct {
	Data giftCardStatusResp `json:"data"`
}

func TestDisable_ActiveCard_Returns200AndFreezesBalance(t *testing.T) {
	env := setupGiftCardStatusRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	gc := seedGiftCard(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID), giftcard.StatusActive, nil)

	w := request(t, env.router, http.MethodPost,
		giftCardStatusURL(storeID, gc.ID.String(), "disable"), nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("disable: status %d body=%s", w.Code, w.Body.String())
	}
	var resp giftCardStatusEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Status != "disabled" {
		t.Fatalf("status = %q, want disabled", resp.Data.Status)
	}
	if !resp.Data.CurrentBalance.Equal(gc.CurrentBalance) {
		t.Fatalf("current_balance = %s, want unchanged %s", resp.Data.CurrentBalance, gc.CurrentBalance)
	}
}

func TestEnable_DisabledCard_Returns200AndRestoresBalance(t *testing.T) {
	env := setupGiftCardStatusRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	gc := seedGiftCard(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID), giftcard.StatusDisabled, nil)

	w := request(t, env.router, http.MethodPost,
		giftCardStatusURL(storeID, gc.ID.String(), "enable"), nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("enable: status %d body=%s", w.Code, w.Body.String())
	}
	var resp giftCardStatusEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Status != "active" {
		t.Fatalf("status = %q, want active", resp.Data.Status)
	}
	if !resp.Data.CurrentBalance.Equal(gc.CurrentBalance) {
		t.Fatalf("current_balance = %s, want unchanged %s", resp.Data.CurrentBalance, gc.CurrentBalance)
	}
}

func TestDisable_PendingCard_Returns409InvalidTransition(t *testing.T) {
	env := setupGiftCardStatusRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	gc := seedGiftCard(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID), giftcard.StatusPending, nil)

	w := request(t, env.router, http.MethodPost,
		giftCardStatusURL(storeID, gc.ID.String(), "disable"), nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "invalid_transition" {
		t.Fatalf("error = %v, want invalid_transition", resp["error"])
	}
}

func TestDisable_ExpiredActiveCard_Returns410GiftCardExpired(t *testing.T) {
	env := setupGiftCardStatusRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	expired := time.Now().Add(-24 * time.Hour)
	gc := seedGiftCard(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID), giftcard.StatusActive, &expired)

	w := request(t, env.router, http.MethodPost,
		giftCardStatusURL(storeID, gc.ID.String(), "disable"), nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410 body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "gift_card_expired" {
		t.Fatalf("error = %v, want gift_card_expired", resp["error"])
	}
}

func TestDisable_AlreadyDisabledCard_Returns200Idempotent(t *testing.T) {
	env := setupGiftCardStatusRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	gc := seedGiftCard(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID), giftcard.StatusDisabled, nil)

	w := request(t, env.router, http.MethodPost,
		giftCardStatusURL(storeID, gc.ID.String(), "disable"), nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, isErr := resp["error"]; isErr {
		t.Fatalf("unexpected error envelope: %s", w.Body.String())
	}
	data, _ := resp["data"].(map[string]any)
	if data["status"] != "disabled" {
		t.Fatalf("status = %v, want disabled", data["status"])
	}
}

// TestDisable_DifferentTenantCard_Returns404 seeds the gift card under a
// different tenant_id than the one the store row (and therefore the
// request's auth/tenant context) belongs to. The repository's SetStatus
// WHERE clause filters on tenant_id, so this must 404 — not leak that the
// card exists under a different tenant.
func TestDisable_DifferentTenantCard_Returns404(t *testing.T) {
	env := setupGiftCardStatusRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	otherTenantID := uuid.New()
	gc := seedGiftCard(t, env.db, otherTenantID, uuid.MustParse(storeID), giftcard.StatusActive, nil)

	w := request(t, env.router, http.MethodPost,
		giftCardStatusURL(storeID, gc.ID.String(), "disable"), nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "not_found" {
		t.Fatalf("error = %v, want not_found", resp["error"])
	}
}

// TestDisable_PreservesFullCardFields is the cross-task regression test for
// the RETURNING column-list bug: SetStatus's atomic UPDATE only listed 11 of
// the GiftCard model's 24 columns, so a card that genuinely came from the
// storefront purchase flow — with PurchasedViaStorefront, RecipientEmail,
// PaymentStatus all set — came back from a real Disable/Enable flip with
// those fields silently wrong (purchased_via_storefront=false, a lie) or
// missing (recipient_email absent) even though the SAME row read via
// GetByID/ListByStore returns them correctly. Must prove red -> green
// against the pre-fix RETURNING clause.
func TestDisable_PreservesFullCardFields(t *testing.T) {
	env := setupGiftCardStatusRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	recipientEmail := "recipient@example.com"
	paid := giftcard.PaymentStatusPaid
	gc := &giftcard.GiftCard{
		ID:                     uuid.New(),
		TenantID:               uuid.MustParse(tenantID),
		StoreID:                uuid.MustParse(storeID),
		Code:                   strings.ToUpper("STOREFRONT" + uuid.NewString()[:8]),
		InitialBalance:         decimal.NewFromInt(100),
		CurrentBalance:         decimal.NewFromInt(100),
		CurrencyCode:           "USD",
		Status:                 giftcard.StatusActive,
		RecipientEmail:         &recipientEmail,
		PurchasedViaStorefront: true,
		PaymentStatus:          &paid,
	}
	if err := env.db.Create(gc).Error; err != nil {
		t.Fatalf("seed gift card: %v", err)
	}

	w := request(t, env.router, http.MethodPost,
		giftCardStatusURL(storeID, gc.ID.String(), "disable"), nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("disable: status %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data["status"] != "disabled" {
		t.Fatalf("status = %v, want disabled", resp.Data["status"])
	}
	if purchasedViaStorefront, _ := resp.Data["purchased_via_storefront"].(bool); !purchasedViaStorefront {
		t.Fatalf("purchased_via_storefront = %v, want true (real flip must not truncate the returned row)", resp.Data["purchased_via_storefront"])
	}
	if resp.Data["recipient_email"] != recipientEmail {
		t.Fatalf("recipient_email = %v, want %q", resp.Data["recipient_email"], recipientEmail)
	}
	if resp.Data["payment_status"] != string(paid) {
		t.Fatalf("payment_status = %v, want %q", resp.Data["payment_status"], string(paid))
	}
}

// TestDisableThenEnable_RoundTrip_SameBalanceSpendableAgain is the
// handler-level echo of Task 1's repository round-trip test — it proves
// the wiring end to end through the actual HTTP routes, not just the
// repository. Must prove red -> green: before routes.go wires the
// disable/enable routes, this 404s on the route itself (not a compile
// error), because the handler methods it calls through HTTP exist only
// once the route is registered.
func TestDisableThenEnable_RoundTrip_SameBalanceSpendableAgain(t *testing.T) {
	env := setupGiftCardStatusRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	gc := seedGiftCard(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID), giftcard.StatusActive, nil)
	headers := authHeaders(userID, tenantID)

	// --- Disable ---
	w := request(t, env.router, http.MethodPost,
		giftCardStatusURL(storeID, gc.ID.String(), "disable"), nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("disable: status %d body=%s", w.Code, w.Body.String())
	}
	var disabled giftCardStatusEnvelope
	_ = json.Unmarshal(w.Body.Bytes(), &disabled)
	if disabled.Data.Status != "disabled" {
		t.Fatalf("disable: status = %q, want disabled", disabled.Data.Status)
	}

	// --- Enable ---
	w = request(t, env.router, http.MethodPost,
		giftCardStatusURL(storeID, gc.ID.String(), "enable"), nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("enable: status %d body=%s", w.Code, w.Body.String())
	}
	var enabled giftCardStatusEnvelope
	_ = json.Unmarshal(w.Body.Bytes(), &enabled)
	if enabled.Data.Status != "active" {
		t.Fatalf("enable: status = %q, want active", enabled.Data.Status)
	}
	if !enabled.Data.CurrentBalance.Equal(gc.CurrentBalance) {
		t.Fatalf("enable: current_balance = %s, want unchanged %s", enabled.Data.CurrentBalance, gc.CurrentBalance)
	}

	// --- Confirm the balance is intact and the card is spendable again
	// via the service's own CheckBalance + Debit, same as Task 1's test
	// #15 verified at the repository layer. ---
	repo := giftcard.NewRepository()
	svc := giftcard.NewService(env.db, repo, nil)

	bal, err := svc.CheckBalance(t.Context(), uuid.MustParse(storeID), gc.Code)
	if err != nil {
		t.Fatalf("CheckBalance: %v", err)
	}
	if !bal.CurrentBalance.Equal(gc.CurrentBalance) {
		t.Fatalf("CheckBalance: current_balance = %s, want unchanged %s", bal.CurrentBalance, gc.CurrentBalance)
	}
	if bal.Status != giftcard.StatusActive {
		t.Fatalf("CheckBalance: status = %q, want active", bal.Status)
	}

	err = env.db.Transaction(func(tx *gorm.DB) error {
		_, debitErr := svc.Debit(tx, gc.ID, decimal.NewFromInt(10), uuid.New(), uuid.MustParse(tenantID))
		return debitErr
	})
	if err != nil {
		t.Fatalf("Debit after round trip: %v", err)
	}
}
