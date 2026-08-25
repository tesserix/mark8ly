package platformadmin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/idempotency"
)

// ExtendReasonCodes is the closed set of reasons a trial may be extended
// for. As with SuspendReasonCodes, an audit row saying WHAT happened
// without WHY is the gap this series exists to close, so the code is
// REQUIRED; free text (`reason`) is accepted IN ADDITION, never instead.
//
// Deliberately a different set from SuspendReasonCodes: the reasons for
// granting more trial time are not the reasons for suspending a tenant.
var ExtendReasonCodes = []string{
	"support_escalation", // an open support case needs more time to resolve
	"onboarding_delay",   // setup or migration slipped, outside the merchant's control
	"billing_dispute",    // a billing disagreement is open; the trial should not lapse meanwhile
	"goodwill",           // discretionary grant, no other category applies
	"operator_error",     // correcting a mistaken earlier extension or trial start
}

// maxReasonLen caps the free-text reason. Long enough for a sentence of
// context, short enough that an audit row stays readable.
const maxReasonLen = 500

// TrialExtender is the subset of the trial package this handler needs,
// declared locally so the handler is stubbable — the same reason
// TenantLifecycle and EstateCounts are declared here rather than imported.
type TrialExtender interface {
	Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (trial.ExtendResult, error)
}

// TrialExtenderFunc adapts a free function to TrialExtender, matching the
// SubscriptionsFunc / TrialListerFunc pattern already used in routes.go.
type TrialExtenderFunc func(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (trial.ExtendResult, error)

func (f TrialExtenderFunc) Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (trial.ExtendResult, error) {
	return f(ctx, db, storeID, newEnd, now)
}

// trialExtendAuditFunc records a platform-operator action. Production
// closes over a real *audit.Emitter via EmitOperatorAction; test doubles
// capture the audit.Event synchronously, which the real Emitter cannot do
// because its write happens on an async worker goroutine.
//
// Declared as an alias of lifecycleAuditFunc (tenant_lifecycle.go), not a
// second independent named type: NewOperatorActionAuditFunc returns
// lifecycleAuditFunc, and Go's assignability rules do not implicitly
// convert between two distinct named function types even when their
// underlying signatures match. An alias keeps exactly one adapter in this
// package rather than requiring — or worse, silently needing — a second one.
type trialExtendAuditFunc = lifecycleAuditFunc

// BillingTrialExtendHandler serves POST /admin/billing/trials/{store_id}/extend.
//
// The path parameter is a STORE id, not a subscription id: #285 emits no
// row id, so the console has none to send, and store_subscriptions declares
// UNIQUE (store_id) which makes the store id unambiguous.
type BillingTrialExtendHandler struct {
	db     *gorm.DB
	ex     TrialExtender
	audit  trialExtendAuditFunc
	logger *slog.Logger
}

// NewBillingTrialExtendHandler constructs the handler. logger may be nil —
// slog.Default() is substituted, matching EmitOperatorAction's own
// fallback (audit.go), so a nil logger can never silence an error path.
func NewBillingTrialExtendHandler(db *gorm.DB, ex TrialExtender, aud trialExtendAuditFunc, logger *slog.Logger) *BillingTrialExtendHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &BillingTrialExtendHandler{db: db, ex: ex, audit: aud, logger: logger}
}

// Register mounts the route on the supplied group.
func (h *BillingTrialExtendHandler) Register(g *gin.RouterGroup) {
	g.POST("/admin/billing/trials/:storeID/extend", h.extend)
}

type trialExtendRequest struct {
	ReasonCode  string `json:"reason_code"`
	Reason      string `json:"reason"`
	TrialEndsAt string `json:"trial_ends_at"`
}

type trialExtendResponse struct {
	StoreID             string `json:"store_id"`
	TenantID            string `json:"tenant_id"`
	TrialEndsAt         string `json:"trial_ends_at"`
	PreviousTrialEndsAt string `json:"previous_trial_ends_at"`
	ExtendedAt          string `json:"extended_at"`
	ReasonCode          string `json:"reason_code"`
	Reason              string `json:"reason,omitempty"`
	RemindersCleared    int64  `json:"reminders_cleared"`
}

func (h *BillingTrialExtendHandler) extend(c *gin.Context) {
	idemKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idemKey == "" {
		// A write that cannot be retried safely is worse than one that
		// refuses to start.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "idempotency_key_required", "message": "the Idempotency-Key header is required for this endpoint",
		})
		return
	}

	storeID, err := uuid.Parse(strings.TrimSpace(c.Param("storeID")))
	if err != nil {
		// 400, not 500: a malformed id is the caller's error. #343 records
		// the opposite happening on another internal route. Matches
		// tenant_lifecycle.go's invalid_tenant_id shape so the console
		// handles both writes the same way.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_store_id", "message": "store id is not a valid uuid", "field": "store_id",
		})
		return
	}

	// Namespaced so a key can never replay across stores or across
	// endpoints. idempotency_keys.key is a bare primary key shared by the
	// whole service, and the caller's raw header is theirs to choose —
	// without this, reusing a key against a different store returns the
	// first store's response and silently skips the second extension.
	scopedKey := "trial_extend:" + storeID.String() + ":" + idemKey

	if h.db != nil {
		// Reserve BEFORE doing the work, not Lookup-then-Save after: two
		// pods handling the same retry can both miss a plain Lookup before
		// either writes, and ON CONFLICT DO NOTHING on the final save only
		// keeps the first BODY — the loser's response would still be
		// returned to its own caller and never stored, so a third retry
		// would then replay a THIRD different body. Reserve closes that
		// race by claiming the key itself first.
		claimed, err := idempotency.Reserve(c.Request.Context(), h.db, scopedKey, storeID.String(), time.Now().UTC(), idempotency.DefaultTTL)
		if err != nil {
			// Fail CLOSED: a caller that cannot be told whether this key
			// was already used must not be allowed through to a second
			// Extend + a second audit row. Costs nothing — Extend would
			// fail on the same unreachable DB anyway — and 503 is exactly
			// what should make the caller retry.
			h.logger.Error("trial extend: idempotency reserve failed", "store_id", storeID.String(), "err", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "unavailable", "message": "could not verify idempotency key",
			})
			return
		}
		if !claimed {
			stored, ok, err := idempotency.Lookup(c.Request.Context(), h.db, scopedKey)
			if err != nil {
				h.logger.Error("trial extend: idempotency lookup failed", "store_id", storeID.String(), "err", err)
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": "unavailable", "message": "could not verify idempotency key",
				})
				return
			}
			if ok {
				// A replay: return the stored bytes verbatim, without
				// calling Extend and without emitting a second audit row.
				c.Data(http.StatusOK, "application/json; charset=utf-8", stored)
				return
			}
			// Reserved but not yet completed: another caller (or another
			// pod handling the SAME retry) is still doing the work.
			c.JSON(http.StatusConflict, gin.H{
				"error": "in_progress", "message": "a request with this Idempotency-Key is already in flight",
			})
			return
		}
	}

	var req trialExtendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// gin returns io.EOF for a completely empty body, so an omitted
		// body is rejected HERE. `{}` binds to the zero value and is the
		// case the reason-code check below exists to catch.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "request body could not be parsed",
		})
		return
	}

	if !isKnownReasonCode(req.ReasonCode, ExtendReasonCodes) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_reason_code",
			"message": "reason_code is required and must be one of the declared codes",
			"field":   "reason_code",
			"allowed": ExtendReasonCodes,
		})
		return
	}

	newEnd, err := time.Parse(time.RFC3339, strings.TrimSpace(req.TrialEndsAt))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "trial_ends_at must be an RFC3339 timestamp",
		})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if len(reason) > maxReasonLen {
		reason = truncateUTF8(reason, maxReasonLen)
	}

	extendedAt := time.Now().UTC()
	res, err := h.ex.Extend(c.Request.Context(), h.db, storeID, newEnd, extendedAt)
	if err != nil {
		h.respondExtendErr(c, err)
		return
	}

	prev := res.PreviousEndsAt.UTC().Format(time.RFC3339)
	next := res.NewEndsAt.UTC().Format(time.RFC3339)

	// EmitOperatorAction, never audit.Emit: nothing on this surface sets
	// tenant_id on the gin context, and resolveScope drops a tenant-less
	// event silently with no error. The tenant is a required parameter here
	// so it cannot be forgotten (trap 3, #310).
	if h.audit != nil {
		ev := audit.Event{
			Action:       "trial.extended",
			ResourceType: "subscription",
			ResourceID:   res.SubscriptionID.String(),
			// StoreID is deliberately LEFT NIL even though we have one.
			// audit.Event's own comment groups trial extend with the
			// tenant-scoped platform writes, and a store-scoped audit row
			// would surface this operator action inside the MERCHANT's own
			// store-scoped audit view — a product decision about what a
			// merchant sees, not a detail to settle by default here. The
			// store id is still recorded, in metadata below.
			//
			// TenantID is NOT set here either: EmitOperatorAction assigns
			// it from its own tenantID parameter (audit.go:44). Setting it
			// in this literal would be overwritten with the same value and
			// would imply the caller owns a field the helper owns.
			Metadata: map[string]any{
				"reason_code":            req.ReasonCode,
				"reason":                 reason,
				"previous_trial_ends_at": prev,
				"trial_ends_at":          next,
				"store_id":               res.StoreID.String(),
				"reminders_cleared":      res.RemindersCleared,
			},
		}
		if err := h.audit(c, res.TenantID, ev); err != nil {
			// Logged, not surfaced: the extension already happened, and
			// failing the response would make the caller retry a write that
			// succeeded.
			h.logger.Error("trial extend: audit emit failed",
				"store_id", res.StoreID.String(), "err", err)
		}
	}

	resp := trialExtendResponse{
		StoreID:             res.StoreID.String(),
		TenantID:            res.TenantID.String(),
		TrialEndsAt:         next,
		PreviousTrialEndsAt: prev,
		ExtendedAt:          extendedAt.Format(time.RFC3339),
		ReasonCode:          req.ReasonCode,
		Reason:              reason,
		RemindersCleared:    res.RemindersCleared,
	}

	if h.db != nil {
		if body, err := json.Marshal(resp); err != nil {
			h.logger.Error("trial extend: could not marshal response for idempotency save", "err", err)
		} else if err := idempotency.Complete(c.Request.Context(), h.db, scopedKey, body); err != nil {
			// Logged, not surfaced: the extension already happened, and
			// failing the response would make the caller retry a write
			// that succeeded.
			h.logger.Error("trial extend: idempotency complete failed", "store_id", res.StoreID.String(), "err", err)
		}
	}

	c.JSON(http.StatusOK, resp)
}

// truncateUTF8 truncates s to at most maxBytes bytes without splitting a
// multibyte rune. A raw byte slice (reason[:n]) can cut a rune in half at
// the boundary, producing invalid UTF-8 that fails to marshal into the
// audit row's jsonb Metadata column — the emit then fails silently and the
// extension succeeds UNAUDITED, which is the exact gap this series exists
// to close.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	b := s[:maxBytes]
	for len(b) > 0 {
		r, size := utf8.DecodeLastRuneInString(b)
		if r != utf8.RuneError || size != 1 {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}

// respondExtendErr maps the domain's sentinel errors to distinct statuses
// and codes, so the console can tell "already converted" from "expired"
// from "Stripe owns this one" rather than getting one opaque refusal.
func (h *BillingTrialExtendHandler) respondExtendErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, trial.ErrAlreadyConverted):
		c.JSON(http.StatusConflict, gin.H{
			"error": "already_converted", "message": "the subscription has already converted to a paid plan",
		})
	case errors.Is(err, trial.ErrStripeManaged):
		c.JSON(http.StatusConflict, gin.H{
			"error":   "stripe_managed",
			"message": "this trial has a Stripe subscription; Stripe owns its billing date and it cannot be extended here",
		})
	case errors.Is(err, trial.ErrNotTrialing):
		c.JSON(http.StatusConflict, gin.H{
			"error": "not_trialing", "message": "the subscription is not in a trial state",
		})
	case errors.Is(err, trial.ErrEndNotInFuture):
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "trial_ends_at must be in the future",
		})
	case errors.Is(err, trial.ErrNoSubscription):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "no subscription for that store",
		})
	default:
		h.logger.Error("trial extend failed", "err", err)
		// The driver's error text is logged server-side, never echoed.
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not extend trial",
		})
	}
}
