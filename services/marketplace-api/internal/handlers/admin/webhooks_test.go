//go:build integration

package admin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/webhook"
)

// webhookTestResolver backs the whGuard built in setupTestRouter. It answers
// "public" for any host by default — including a literal loopback IP host
// like an httptest server's — and only carves out the couple of hostnames
// these tests deliberately use to exercise SSRF rejection, so no test here
// depends on real DNS. See ssrfguard's Send tests (allowAll) for the same
// pattern.
func webhookTestResolver(host string) ([]net.IP, error) {
	switch host {
	case "private.example.com":
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	case "unresolvable.example.com":
		return nil, net.UnknownNetworkError("no such host (test resolver)")
	default:
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
}

func webhooksURL(storeID string) string {
	return "/api/v1/admin/stores/" + storeID + "/webhooks"
}

func webhookCreateBody(url string, eventTypes []string) map[string]any {
	return map[string]any{"url": url, "event_types": eventTypes}
}

// createWebhook posts a valid subscription and returns (id, secret).
func createWebhook(t *testing.T, env *testEnv, storeID, tenantID, userID string) (string, string) {
	t.Helper()
	w := request(t, env.router, http.MethodPost, webhooksURL(storeID),
		webhookCreateBody("https://public.example.com/hooks", []string{"order.placed"}),
		authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("create webhook: status = %d, body = %s", w.Code, w.Body.String())
	}
	data := dataObject(t, w.Body.Bytes())
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	secret, _ := resp["secret"].(string)
	return data["id"].(string), secret
}

func webhookOwnerEnv(t *testing.T) (*testEnv, string, string, string) {
	t.Helper()
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	env.fga.Grant(userID, authz.RoleStaff, tenantID)
	return env, storeID, tenantID, userID
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestCreate_RejectsNonPublicURL(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)

	w := request(t, env.router, http.MethodPost, webhooksURL(storeID),
		webhookCreateBody("https://private.example.com/hooks", []string{"order.placed"}),
		authHeaders(userID, tenantID))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "validation_failed" {
		t.Fatalf("expected validation_failed, got %v (%s)", resp["error"], w.Body.String())
	}
}

func TestCreate_RejectsPlainHTTP(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)

	w := request(t, env.router, http.MethodPost, webhooksURL(storeID),
		webhookCreateBody("http://public.example.com/hooks", []string{"order.placed"}),
		authHeaders(userID, tenantID))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestCreate_RejectsUnknownEventType(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)

	w := request(t, env.router, http.MethodPost, webhooksURL(storeID),
		webhookCreateBody("https://public.example.com/hooks", []string{"order.teleported"}),
		authHeaders(userID, tenantID))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "validation_failed" {
		t.Fatalf("expected validation_failed, got %v (%s)", resp["error"], w.Body.String())
	}
}

func TestCreate_RejectsMoreThanMaxEventTypes(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)

	types := make([]string, 0, webhook.MaxEventTypes+1)
	for i := 0; i < webhook.MaxEventTypes+1; i++ {
		types = append(types, "order.placed")
	}

	w := request(t, env.router, http.MethodPost, webhooksURL(storeID),
		webhookCreateBody("https://public.example.com/hooks", types),
		authHeaders(userID, tenantID))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestCreate_ReturnsTheSecretExactlyOnce(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)

	w := request(t, env.router, http.MethodPost, webhooksURL(storeID),
		webhookCreateBody("https://public.example.com/hooks", []string{"order.placed"}),
		authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var createResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	secret, _ := createResp["secret"].(string)
	if secret == "" {
		t.Fatalf("create response missing secret: %s", w.Body.String())
	}
	data, _ := createResp["data"].(map[string]any)
	if _, ok := data["secret"]; ok {
		t.Fatalf("secret must not also appear inside data: %s", w.Body.String())
	}
	id, _ := data["id"].(string)

	// A subsequent GET (via List, the only read endpoint) must not carry it.
	lw := request(t, env.router, http.MethodGet, webhooksURL(storeID), nil, authHeaders(userID, tenantID))
	if lw.Code != http.StatusOK {
		t.Fatalf("list: status = %d, body = %s", lw.Code, lw.Body.String())
	}
	items := dataArray(t, lw.Body.Bytes())
	found := false
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["id"] == id {
			found = true
			if _, ok := item["secret"]; ok {
				t.Fatalf("list response must never carry secret: %s", lw.Body.String())
			}
		}
	}
	if !found {
		t.Fatalf("created webhook not found in list: %s", lw.Body.String())
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestList_ScopesToTheCallersTenantAndStore(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)
	createWebhook(t, env, storeID, tenantID, userID)

	// A different tenant/store must see none of it.
	otherStoreID, otherTenantID := seedStoreRow(t, env.db, "")
	otherUserID := uuid.NewString()
	env.fga.Grant(otherUserID, authz.RoleStaff, otherTenantID)

	w := request(t, env.router, http.MethodGet, webhooksURL(otherStoreID), nil, authHeaders(otherUserID, otherTenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	items := dataArray(t, w.Body.Bytes())
	if len(items) != 0 {
		t.Fatalf("expected no subscriptions visible to another tenant, got %s", w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Cross-tenant access is refused everywhere an :id is taken from the path.
// ---------------------------------------------------------------------------

func TestCrossTenantAccess_IsRefusedOnEveryIDRoute(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)
	id, _ := createWebhook(t, env, storeID, tenantID, userID)

	otherStoreID, otherTenantID := seedStoreRow(t, env.db, "")
	otherUserID := uuid.NewString()
	env.fga.Grant(otherUserID, authz.RoleOwner, otherTenantID)
	env.fga.Grant(otherUserID, authz.RoleAdmin, otherTenantID)
	env.fga.Grant(otherUserID, authz.RoleStaff, otherTenantID)
	otherHeaders := authHeaders(otherUserID, otherTenantID)

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"patch", http.MethodPatch, webhooksURL(otherStoreID) + "/" + id, map[string]any{"enabled": false}},
		{"test-send", http.MethodPost, webhooksURL(otherStoreID) + "/" + id + "/test", nil},
		{"deliveries", http.MethodGet, webhooksURL(otherStoreID) + "/" + id + "/deliveries", nil},
		{"replay", http.MethodPost, webhooksURL(otherStoreID) + "/" + id + "/deliveries/" + uuid.NewString() + "/replay", nil},
		{"delete", http.MethodDelete, webhooksURL(otherStoreID) + "/" + id, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := request(t, env.router, tc.method, tc.path, tc.body, otherHeaders)
			if w.Code != http.StatusNotFound {
				t.Fatalf("%s: status = %d, want 404, body = %s", tc.name, w.Code, w.Body.String())
			}
		})
	}

	// The subscription must be untouched by the (refused) delete/patch above.
	lw := request(t, env.router, http.MethodGet, webhooksURL(storeID), nil, authHeaders(userID, tenantID))
	items := dataArray(t, lw.Body.Bytes())
	if len(items) != 1 {
		t.Fatalf("expected the original subscription to survive, got %s", lw.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Replay
// ---------------------------------------------------------------------------

func TestReplay_ResetsAFailedDeliveryToPendingAndDueNow(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)
	subID, _ := createWebhook(t, env, storeID, tenantID, userID)

	deliveryRepo := webhook.NewDeliveryRepo(env.db)
	future := time.Now().Add(2 * time.Hour)
	lastErr := "endpoint returned 500"
	code := 500
	delivery := webhook.Delivery{
		SubscriptionID: uuid.MustParse(subID),
		OutboxEventID:  uuid.New(),
		EventType:      "order.placed",
		AggregateID:    uuid.New(),
		Status:         webhook.StatusFailed,
		Attempts:       3,
		NextAttemptAt:  future,
		LastStatusCode: &code,
		LastError:      &lastErr,
	}
	if _, err := deliveryRepo.FanOut(context.Background(), []webhook.Delivery{delivery}); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	seeded, err := deliveryRepo.ListForSubscription(context.Background(), uuid.MustParse(subID), 1)
	if err != nil || len(seeded) != 1 {
		t.Fatalf("seed delivery: list = %v, err = %v", seeded, err)
	}
	deliveryID := seeded[0].ID.String()

	w := request(t, env.router, http.MethodPost,
		webhooksURL(storeID)+"/"+subID+"/deliveries/"+deliveryID+"/replay", nil,
		authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	after, err := deliveryRepo.ListForSubscription(context.Background(), uuid.MustParse(subID), 1)
	if err != nil || len(after) != 1 {
		t.Fatalf("post-replay list = %v, err = %v", after, err)
	}
	got := after[0]
	if got.Status != webhook.StatusPending {
		t.Fatalf("status = %q, want pending", got.Status)
	}
	if got.Attempts != 0 {
		t.Fatalf("attempts = %d, want 0", got.Attempts)
	}
	if got.NextAttemptAt.After(time.Now()) {
		t.Fatalf("next_attempt_at = %v, want due now", got.NextAttemptAt)
	}
}

func TestReplay_UnknownDeliveryID_404s(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)
	subID, _ := createWebhook(t, env, storeID, tenantID, userID)

	w := request(t, env.router, http.MethodPost,
		webhooksURL(storeID)+"/"+subID+"/deliveries/"+uuid.NewString()+"/replay", nil,
		authHeaders(userID, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test-send
// ---------------------------------------------------------------------------

// TestSend refuses to leak an unhandled-error 500 (which would log the
// remote body) — a Send failure comes back as a 200 with success=false and
// the error text in the JSON body instead.
func TestTestSend_ReportsFailureWithoutServerError(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)

	w := request(t, env.router, http.MethodPost, webhooksURL(storeID),
		webhookCreateBody("https://unresolvable.example.com/hooks", []string{"order.placed"}),
		authHeaders(userID, tenantID))
	// Registration itself refuses an unresolvable host, so seed the
	// subscription directly through the repo to exercise Send's own guard
	// re-check instead of Create's.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected registration to refuse an unresolvable host: status = %d, body = %s", w.Code, w.Body.String())
	}

	subs := webhook.NewSubscriptionRepo(env.db)
	secret, err := webhook.GenerateSecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	sub := &webhook.Subscription{
		TenantID:   uuid.MustParse(tenantID),
		StoreID:    uuid.MustParse(storeID),
		URL:        "https://unresolvable.example.com/hooks",
		EventTypes: []string{"order.placed"},
		Secret:     secret,
		Enabled:    true,
	}
	if err := subs.Create(context.Background(), sub); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	tw := request(t, env.router, http.MethodPost, webhooksURL(storeID)+"/"+sub.ID.String()+"/test", nil,
		authHeaders(userID, tenantID))
	if tw.Code != http.StatusOK {
		t.Fatalf("test-send status = %d, body = %s", tw.Code, tw.Body.String())
	}
	data := dataObject(t, tw.Body.Bytes())
	if data["success"] != false {
		t.Fatalf("expected success=false, got %s", tw.Body.String())
	}
	if data["error"] == nil || data["error"] == "" {
		t.Fatalf("expected an error message, got %s", tw.Body.String())
	}
}

// TestCreateWebhook_EnforcesPerStoreCap pins the #586 cap. Dispatch fan-out
// is `outbox rows × matching subscriptions`, so an unbounded subscription
// count turns one order.placed into an unbounded number of delivery rows and
// outbound HTTP attempts against a shared db-f1-micro. A seeded store has no
// subscription row, so PlanResolver falls back to PlanTrial — a cap of 5.
func TestCreateWebhook_EnforcesPerStoreCap(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)

	const trialCap = 5
	for i := 0; i < trialCap; i++ {
		w := request(t, env.router, http.MethodPost, webhooksURL(storeID),
			webhookCreateBody(fmt.Sprintf("https://public.example.com/hooks/%d", i), []string{"order.placed"}),
			authHeaders(userID, tenantID))
		if w.Code != http.StatusCreated {
			t.Fatalf("create #%d: status = %d, body = %s (all %d should fit under the cap)",
				i+1, w.Code, w.Body.String(), trialCap)
		}
	}

	// The one past the cap must be refused with a 400 the merchant can act
	// on — naming the limit, the current count and the plan.
	w := request(t, env.router, http.MethodPost, webhooksURL(storeID),
		webhookCreateBody("https://public.example.com/hooks/over", []string{"order.placed"}),
		authHeaders(userID, tenantID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create past cap: status = %d, want 400, body = %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if resp["error"] != "webhook_subscription_limit_reached" {
		t.Fatalf("error code = %v, want webhook_subscription_limit_reached (body = %s)", resp["error"], w.Body.String())
	}
	if got := resp["limit"]; got != float64(trialCap) {
		t.Fatalf("limit = %v, want %d — the response must name the cap so the UI can explain it", got, trialCap)
	}
	if got := resp["current"]; got != float64(trialCap) {
		t.Fatalf("current = %v, want %d", got, trialCap)
	}
	if msg, _ := resp["message"].(string); msg == "" {
		t.Fatal("expected a human-readable message a merchant can act on")
	}

	// The refusal must not have written a row.
	subs := webhook.NewSubscriptionRepo(env.db)
	n, err := subs.CountForStore(context.Background(), uuid.MustParse(tenantID), uuid.MustParse(storeID))
	if err != nil {
		t.Fatalf("CountForStore: %v", err)
	}
	if n != trialCap {
		t.Fatalf("subscription count = %d, want %d — a rejected create must not persist", n, trialCap)
	}
}

// TestCreateWebhook_DisabledSubscriptionsCountTowardCap closes the obvious
// way around the cap: parking subscriptions in the disabled state. A disabled
// row is still a row the merchant can flip back on, so it must count.
func TestCreateWebhook_DisabledSubscriptionsCountTowardCap(t *testing.T) {
	env, storeID, tenantID, userID := webhookOwnerEnv(t)

	const trialCap = 5
	var firstID string
	for i := 0; i < trialCap; i++ {
		w := request(t, env.router, http.MethodPost, webhooksURL(storeID),
			webhookCreateBody(fmt.Sprintf("https://public.example.com/hooks/%d", i), []string{"order.placed"}),
			authHeaders(userID, tenantID))
		if w.Code != http.StatusCreated {
			t.Fatalf("create #%d: status = %d, body = %s", i+1, w.Code, w.Body.String())
		}
		if i == 0 {
			first := dataObject(t, w.Body.Bytes())
			firstID, _ = first["id"].(string)
		}
	}

	// Disable one — this must NOT free a slot.
	pw := request(t, env.router, http.MethodPatch, webhooksURL(storeID)+"/"+firstID,
		map[string]any{"enabled": false}, authHeaders(userID, tenantID))
	if pw.Code != http.StatusOK {
		t.Fatalf("disable: status = %d, body = %s", pw.Code, pw.Body.String())
	}

	w := request(t, env.router, http.MethodPost, webhooksURL(storeID),
		webhookCreateBody("https://public.example.com/hooks/after-disable", []string{"order.placed"}),
		authHeaders(userID, tenantID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create after disabling one: status = %d, want 400 — disabling must not free a slot (body = %s)",
			w.Code, w.Body.String())
	}
}
