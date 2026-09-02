// Package admin — webhooks.go: the merchant-facing admin API for outbound
// webhook subscriptions, mounted at /admin/stores/:storeId/webhooks (#562
// task 7).
//
// Every handler here scopes its query on the tenant and store the auth
// middleware chain already resolved (c.GetString("tenant_id"), the
// :storeId path param FGA has already checked ownership of) — never on a
// value from the request body. A subscription or delivery id that does not
// belong to that (tenant, store) pair is reported as 404, exactly like an
// id that does not exist at all, so guessing a UUID cannot distinguish
// "not yours" from "doesn't exist".
package admin

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/internal/webhook/ssrfguard"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// allowedEventTypes is the closed set a subscription may select, built from
// internal/outbox's 18 Event* constants. Validating against it turns a
// typo into a 400 at registration instead of a subscription that silently
// never fires — which reads to a merchant as a broken product, not a typo.
var allowedEventTypes = map[string]bool{
	outbox.EventOrderPlaced:                true,
	outbox.EventOrderConfirmed:             true,
	outbox.EventOrderFulfilled:             true,
	outbox.EventOrderPartiallyFulfilled:    true,
	outbox.EventOrderCancelled:             true,
	outbox.EventOrderRefunded:              true,
	outbox.EventReturnRequested:            true,
	outbox.EventReturnApproved:             true,
	outbox.EventReturnReceived:             true,
	outbox.EventReturnRefunded:             true,
	outbox.EventReturnRejected:             true,
	outbox.EventProductCreated:             true,
	outbox.EventProductUpdated:             true,
	outbox.EventProductDeleted:             true,
	outbox.EventCategoryCreated:            true,
	outbox.EventCategoryUpdated:            true,
	outbox.EventCategoryDeleted:            true,
	outbox.EventAbandonedCartRecoveryEmail: true,
}

// testEventType is the synthetic event name a test-send carries. It is
// deliberately not in allowedEventTypes — a merchant can never subscribe
// to it directly.
const testEventType = "webhook.test"

// deliveryListLimit bounds how many recent deliveries one GET returns.
const deliveryListLimit = 50

// WebhooksHandler serves the per-store outbound webhook admin endpoints.
type WebhooksHandler struct {
	subs       *webhook.SubscriptionRepo
	deliveries *webhook.DeliveryRepo
	guard      *ssrfguard.Guard
	sender     *webhook.Sender
	resolver   *plangate.PlanResolver
	logger     *slog.Logger
}

// NewWebhooksHandler constructs a WebhooksHandler. resolver is an explicit
// parameter rather than an optional setter because it enforces the per-store
// subscription cap (#586) — a nil resolver disables that cap, and a limit
// that silently does not apply is worse than no limit at all. Create logs
// loudly and fails closed if it is ever nil.
func NewWebhooksHandler(subs *webhook.SubscriptionRepo, deliveries *webhook.DeliveryRepo, guard *ssrfguard.Guard, sender *webhook.Sender, resolver *plangate.PlanResolver, logger *slog.Logger) *WebhooksHandler {
	return &WebhooksHandler{subs: subs, deliveries: deliveries, guard: guard, sender: sender, resolver: resolver, logger: logger}
}

// ─────────────────────────────────────────────────────────────────────────
// Wire types
// ─────────────────────────────────────────────────────────────────────────

// CreateWebhookRequest is the POST /webhooks body.
type CreateWebhookRequest struct {
	URL        string   `json:"url" binding:"required"`
	EventTypes []string `json:"event_types" binding:"required"`
}

// PatchWebhookRequest is the PATCH /webhooks/:id body. Every field is
// optional — only the fields present are changed.
type PatchWebhookRequest struct {
	URL        *string  `json:"url"`
	EventTypes []string `json:"event_types"`
	Enabled    *bool    `json:"enabled"`
}

// WebhookResponse is one subscription on the wire. It deliberately has no
// secret field — the create handler puts the secret in a sibling response
// field instead of on this struct, so reusing toWebhookResponse for
// List/Get/Patch can never leak it.
type WebhookResponse struct {
	ID             string     `json:"id"`
	URL            string     `json:"url"`
	EventTypes     []string   `json:"event_types"`
	Enabled        bool       `json:"enabled"`
	DisabledReason *string    `json:"disabled_reason,omitempty"`
	DisabledAt     *time.Time `json:"disabled_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func toWebhookResponse(s *webhook.Subscription) WebhookResponse {
	return WebhookResponse{
		ID:             s.ID.String(),
		URL:            s.URL,
		EventTypes:     []string(s.EventTypes),
		Enabled:        s.Enabled,
		DisabledReason: s.DisabledReason,
		DisabledAt:     s.DisabledAt,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}

// DeliveryResponse is one delivery attempt on the wire.
type DeliveryResponse struct {
	ID             string     `json:"id"`
	EventType      string     `json:"event_type"`
	AggregateID    string     `json:"aggregate_id"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	NextAttemptAt  time.Time  `json:"next_attempt_at"`
	LastStatusCode *int       `json:"last_status_code,omitempty"`
	LastError      *string    `json:"last_error,omitempty"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func toDeliveryResponse(d *webhook.Delivery) DeliveryResponse {
	return DeliveryResponse{
		ID:             d.ID.String(),
		EventType:      d.EventType,
		AggregateID:    d.AggregateID.String(),
		Status:         d.Status,
		Attempts:       d.Attempts,
		NextAttemptAt:  d.NextAttemptAt,
		LastStatusCode: d.LastStatusCode,
		LastError:      d.LastError,
		DeliveredAt:    d.DeliveredAt,
		CreatedAt:      d.CreatedAt,
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Validation helpers
// ─────────────────────────────────────────────────────────────────────────

// validateEventTypes enforces the closed vocabulary and the MaxEventTypes
// cap. A caller must pass at least one type — an empty selection is a
// subscription that can never fire, which is exactly the "looks alive but
// is dead" failure mode this handler exists to prevent at registration.
func validateEventTypes(types []string) *apperrors.Error {
	if len(types) == 0 {
		return apperrors.ValidationFailed("event_types", "at least one event type is required")
	}
	if len(types) > webhook.MaxEventTypes {
		return apperrors.ValidationFailed("event_types",
			fmt.Sprintf("a subscription may select at most %d event types", webhook.MaxEventTypes))
	}
	for _, t := range types {
		if !allowedEventTypes[t] {
			return apperrors.ValidationFailed("event_types", fmt.Sprintf("unknown event type %q", t))
		}
	}
	return nil
}

// mapSSRFErr turns an ssrfguard error into a distinct, human message — a
// merchant needs to know WHICH rule their URL hit, not just "invalid url".
func mapSSRFErr(err error) *apperrors.Error {
	switch {
	case errors.Is(err, ssrfguard.ErrNotHTTPS):
		return apperrors.ValidationFailed("url", "webhook url must use https")
	case errors.Is(err, ssrfguard.ErrPrivateAddress):
		return apperrors.ValidationFailed("url", "webhook url resolves to a private or otherwise non-public address")
	case errors.Is(err, ssrfguard.ErrUnresolvable):
		return apperrors.ValidationFailed("url", "webhook url host could not be resolved")
	case errors.Is(err, ssrfguard.ErrTooLong):
		return apperrors.ValidationFailed("url", "webhook url is too long")
	case errors.Is(err, ssrfguard.ErrMalformed):
		return apperrors.ValidationFailed("url", "webhook url is malformed")
	default:
		return apperrors.ValidationFailed("url", "webhook url is invalid")
	}
}

// scope reads the tenant id the auth middleware set on the context and the
// store id the store middleware has already verified ownership of. Neither
// ever comes from the request body.
func (h *WebhooksHandler) scope(c *gin.Context) (tenantID, storeID uuid.UUID, ok bool) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return uuid.Nil, uuid.Nil, false
	}
	tenantID, err = uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, storeID, true
}

// ownedSubscription fetches the subscription named by the :id path param
// and verifies it belongs to (tenantID, storeID). A subscription that
// exists but belongs to someone else is reported identically to one that
// does not exist — the security boundary this whole file exists for.
func (h *WebhooksHandler) ownedSubscription(c *gin.Context, tenantID, storeID uuid.UUID) (*webhook.Subscription, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return nil, false
	}
	sub, err := h.subs.ByID(c.Request.Context(), id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return nil, false
	}
	if sub == nil || sub.TenantID != tenantID || sub.StoreID != storeID {
		RespondErr(c, apperrors.NotFound("webhook subscription"), h.logger)
		return nil, false
	}
	return sub, true
}

// ─────────────────────────────────────────────────────────────────────────
// Handlers
// ─────────────────────────────────────────────────────────────────────────

// Create handles POST /admin/stores/:storeId/webhooks. Runs the SSRF guard
// and the event-type check before ever generating a secret or touching the
// database, and returns the secret exactly once — it never leaves the
// server again after this response (Subscription.Secret is json:"-").
// withinSubscriptionCap reports whether the store may create one more
// subscription, writing the error response itself when it may not. Fails
// CLOSED on a nil resolver or a count error: the cap protects shared
// database capacity, so "we could not check" must not mean "allow".
func (h *WebhooksHandler) withinSubscriptionCap(c *gin.Context, tenantID, storeID uuid.UUID) bool {
	if h.resolver == nil {
		// Wiring bug, not a merchant error. Loud, and closed.
		if h.logger != nil {
			h.logger.Error("webhooks: no plan resolver wired, cannot enforce subscription cap",
				"tenant_id", tenantID, "store_id", storeID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Webhook subscription limits are unavailable. Please try again later.",
		})
		return false
	}

	ctx := c.Request.Context()
	plan := h.resolver.Resolve(ctx, tenantID, storeID)
	limit := plangate.Limit(plan, plangate.FeatureWebhookSubscriptions)

	count, err := h.subs.CountForStore(ctx, tenantID, storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return false
	}

	if count >= limit {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "webhook_subscription_limit_reached",
			"message": fmt.Sprintf(
				"This store has %d of %d webhook subscriptions allowed on the %s plan. Delete one, or upgrade for a higher limit.",
				count, limit, plan),
			"limit":   limit,
			"current": count,
			"plan":    string(plan),
			"feature": string(plangate.FeatureWebhookSubscriptions),
		})
		return false
	}
	return true
}

func (h *WebhooksHandler) Create(c *gin.Context) {
	tenantID, storeID, ok := h.scope(c)
	if !ok {
		return
	}

	var req CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if _, err := h.guard.Check(req.URL); err != nil {
		RespondErr(c, mapSSRFErr(err), h.logger)
		return
	}
	if verr := validateEventTypes(req.EventTypes); verr != nil {
		RespondErr(c, verr, h.logger)
		return
	}

	// Per-store subscription cap (#586). Dispatch fan-out is
	// `outbox rows × matching subscriptions`, so an unbounded count turns
	// one order.placed into an unbounded number of delivery rows and
	// outbound HTTP attempts. Enforced here, at creation, only — a
	// downgrade never deletes a merchant's existing subscriptions, the same
	// contract FeatureImagesPerProduct has.
	if !h.withinSubscriptionCap(c, tenantID, storeID) {
		return
	}

	secret, err := webhook.GenerateSecret()
	if err != nil {
		RespondErr(c, fmt.Errorf("webhooks: generate secret: %w", err), h.logger)
		return
	}

	sub := &webhook.Subscription{
		TenantID:   tenantID,
		StoreID:    storeID,
		URL:        req.URL,
		EventTypes: pq.StringArray(req.EventTypes),
		Secret:     secret,
		Enabled:    true,
	}
	if err := h.subs.Create(c.Request.Context(), sub); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data":   toWebhookResponse(sub),
		"secret": secret,
	})
}

// List handles GET /admin/stores/:storeId/webhooks, scoped to the caller's
// tenant and store.
func (h *WebhooksHandler) List(c *gin.Context) {
	tenantID, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	subs, err := h.subs.ListForStore(c.Request.Context(), tenantID, storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	out := make([]WebhookResponse, 0, len(subs))
	for i := range subs {
		out = append(out, toWebhookResponse(&subs[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// Patch handles PATCH /admin/stores/:storeId/webhooks/:id — url, event
// types and enabled are each optional. Re-enabling a subscription clears
// the auto-disable bookkeeping, since it no longer describes the current
// state once a merchant has acted on it.
func (h *WebhooksHandler) Patch(c *gin.Context) {
	tenantID, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	sub, ok := h.ownedSubscription(c, tenantID, storeID)
	if !ok {
		return
	}

	var req PatchWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	updated := *sub
	if req.URL != nil {
		if _, err := h.guard.Check(*req.URL); err != nil {
			RespondErr(c, mapSSRFErr(err), h.logger)
			return
		}
		updated.URL = *req.URL
	}
	if req.EventTypes != nil {
		if verr := validateEventTypes(req.EventTypes); verr != nil {
			RespondErr(c, verr, h.logger)
			return
		}
		updated.EventTypes = pq.StringArray(req.EventTypes)
	}
	if req.Enabled != nil {
		updated.Enabled = *req.Enabled
		if *req.Enabled {
			updated.DisabledReason = nil
			updated.DisabledAt = nil
			updated.ConsecutiveFailures = 0
		}
	}

	if err := h.subs.Update(c.Request.Context(), &updated); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toWebhookResponse(&updated)})
}

// Delete handles DELETE /admin/stores/:storeId/webhooks/:id.
func (h *WebhooksHandler) Delete(c *gin.Context) {
	tenantID, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	sub, ok := h.ownedSubscription(c, tenantID, storeID)
	if !ok {
		return
	}
	if err := h.subs.Delete(c.Request.Context(), sub.ID); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "webhook subscription deleted"})
}

// TestSend handles POST /admin/stores/:storeId/webhooks/:id/test. It builds
// a synthetic delivery — never persisted, never fanned out — and calls the
// same Sender the real delivery worker uses, so a merchant can debug their
// endpoint without waiting for a real event.
//
// The Send error (which may embed the endpoint's response body) is
// returned to the merchant in the JSON body, exactly like a real delivery
// record would surface it — but it is NEVER passed to RespondErr/the
// logger, which would log it server-side.
func (h *WebhooksHandler) TestSend(c *gin.Context) {
	tenantID, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	sub, ok := h.ownedSubscription(c, tenantID, storeID)
	if !ok {
		return
	}

	delivery := webhook.Delivery{
		ID:             uuid.New(),
		SubscriptionID: sub.ID,
		OutboxEventID:  uuid.New(),
		EventType:      testEventType,
		AggregateID:    uuid.New(),
		CreatedAt:      time.Now(),
	}
	status, sendErr := h.sender.Send(c.Request.Context(), *sub, delivery)

	resp := gin.H{"status_code": status, "success": sendErr == nil}
	if sendErr != nil {
		resp["error"] = sendErr.Error()
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ListDeliveries handles GET /admin/stores/:storeId/webhooks/:id/deliveries.
func (h *WebhooksHandler) ListDeliveries(c *gin.Context) {
	tenantID, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	sub, ok := h.ownedSubscription(c, tenantID, storeID)
	if !ok {
		return
	}

	deliveries, err := h.deliveries.ListForSubscription(c.Request.Context(), sub.ID, deliveryListLimit)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	out := make([]DeliveryResponse, 0, len(deliveries))
	for i := range deliveries {
		out = append(out, toDeliveryResponse(&deliveries[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// ReplayDelivery handles
// POST /admin/stores/:storeId/webhooks/:id/deliveries/:deliveryID/replay.
// It resets the delivery to pending, due now, so the worker's next poll
// retries it. Scoped through ownedSubscription first, then again by
// subscription id in the UPDATE itself, so a deliveryID that belongs to
// another tenant's subscription can never be replayed by guessing it.
//
// A delivery that is ALREADY pending is not replayable and answers 404
// here: it may be leased by a worker mid-send right now, and resetting
// next_attempt_at under that lease would let a second worker claim and
// send it too. It is also pointless — a pending delivery is already going
// to be attempted. The admin UI only offers the button on settled rows;
// this is the enforcement behind that.
func (h *WebhooksHandler) ReplayDelivery(c *gin.Context) {
	tenantID, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	sub, ok := h.ownedSubscription(c, tenantID, storeID)
	if !ok {
		return
	}

	deliveryID, err := uuid.Parse(c.Param("deliveryID"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("deliveryID", "invalid uuid"), h.logger)
		return
	}

	found, err := h.deliveries.Replay(c.Request.Context(), sub.ID, deliveryID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if !found {
		RespondErr(c, apperrors.NotFound("webhook delivery"), h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "delivery reset to pending"})
}
