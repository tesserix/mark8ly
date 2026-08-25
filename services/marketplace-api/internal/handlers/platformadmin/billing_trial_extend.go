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
//
// callerIdemKey carries the handler's already-scoped idempotency key down
// into the domain, which needs it to derive the key it sends to Stripe. It
// is threaded rather than re-derived because only the handler has the
// caller's Idempotency-Key header (#358 F1).
type TrialExtender interface {
	Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time, callerIdemKey string) (trial.ExtendResult, error)
}

// TrialExtenderFunc adapts a free function to TrialExtender, matching the
// SubscriptionsFunc / TrialListerFunc pattern already used in routes.go.
type TrialExtenderFunc func(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time, callerIdemKey string) (trial.ExtendResult, error)

func (f TrialExtenderFunc) Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time, callerIdemKey string) (trial.ExtendResult, error) {
	return f(ctx, db, storeID, newEnd, now, callerIdemKey)
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

	// Present only for a card-backed extension. omitempty throughout: a
	// card-less extension carries no Stripe keys at all, rather than nulls
	// or a `false` that reads as "we checked and it did not move".
	StripeSubscriptionID string `json:"stripe_subscription_id,omitempty"`
	StripeTrialEnd       string `json:"stripe_trial_end,omitempty"`
	BillingAnchorMoved   bool   `json:"billing_anchor_moved,omitempty"`
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

	// Reserve immediately before the work, AFTER every validation step
	// above: validation is deterministic, so two concurrent callers with
	// the same key either both pass it or both fail it identically, and
	// only then race on Reserve. Reserving any earlier would let a failed
	// attempt (a malformed body, an unknown reason_code, a bad date, or a
	// domain refusal below) leave the key claimed with an empty Response
	// for the full TTL — turning a mistyped request into a key that
	// answers 409 in_progress for a day, which is worse than the race F3
	// closed.
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

	extendedAt := time.Now().UTC()
	res, err := h.ex.Extend(c.Request.Context(), h.db, storeID, newEnd, extendedAt, scopedKey)
	if err != nil {
		h.releaseReservation(c, scopedKey)
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
		if res.StripeApplied {
			ev.Metadata["stripe_subscription_id"] = res.StripeSubscriptionID
			ev.Metadata["stripe_trial_end_unix"] = res.StripeTrialEnd
			ev.Metadata["previous_stripe_trial_end_unix"] = res.PreviousStripeTrialEnd
			ev.Metadata["previous_billing_anchor_unix"] = res.PreviousBillingAnchor
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

	if res.StripeApplied {
		// stripe_trial_end is Stripe's own reply, not our request: the two
		// are different claims and only one of them is authoritative.
		resp.StripeSubscriptionID = res.StripeSubscriptionID
		resp.StripeTrialEnd = time.Unix(res.StripeTrialEnd, 0).UTC().Format(time.RFC3339)
		// Stripe moves billing_cycle_anchor to trial_end on every trial_end
		// update — its documented behaviour, confirmed against the API in
		// #358's verification. The operator learns the merchant's billing
		// date moved from the same response that moved it.
		resp.BillingAnchorMoved = true
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

// releaseReservation drops a Reserve claim that will never be Completed,
// because Extend refused or failed. Without this, a reservation taken
// before this failure would sit on the key with an empty Response for the
// full TTL — turning a mistyped reason_code or a domain refusal (already
// converted, Stripe-managed, not trialing, end not in future, no
// subscription) into a key that answers 409 in_progress for a day, even
// though the caller already has their real answer and a corrected retry
// should proceed normally.
//
// The failure is logged, not surfaced: the caller already has the actual
// error response from respondExtendErr, and a Release failure just means
// the reservation lives out its TTL instead of being cleared early — an
// inconvenience, not a correctness problem.
func (h *BillingTrialExtendHandler) releaseReservation(c *gin.Context, scopedKey string) {
	if h.db == nil {
		return
	}
	if err := idempotency.Release(c.Request.Context(), h.db, scopedKey); err != nil {
		h.logger.Error("trial extend: idempotency release failed", "err", err)
	}
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
			"message": "this trial has a stripe subscription and stripe billing is not configured on this service, so its billing date cannot be moved",
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
	case errors.Is(err, trial.ErrStripeStateConflict):
		c.JSON(http.StatusConflict, gin.H{
			"error":   "stripe_state_conflict",
			"message": "stripe reports this subscription is no longer trialing; it cannot be extended until the two agree",
		})
	case errors.Is(err, trial.ErrTrialEndNotAfterStripe):
		// 400: the operator can fix this by picking a later date — in the
		// common case. The message is the domain error's own text, which
		// carries BOTH the requested end and the one Stripe currently
		// holds, so the retry can be informed rather than guessed. Echoing
		// err.Error() is safe here and only here: this sentinel's message
		// is composed entirely from our own two timestamps, never from a
		// driver or a third party (#358 F2).
		//
		// #358 N1: when the requested date EQUALS what Stripe already
		// holds, "pick a later date" is the wrong instruction — there is no
		// later date to pick, because Stripe already has this exact one.
		// That equality is the signature of a local row that fell behind
		// Stripe after an ErrStripeAppliedLocalWriteFailed divergence: the
		// Stripe call already succeeded at this date, only the local commit
		// failed, and this endpoint refuses the corrective retry rather
		// than guessing (extend.go's ErrTrialEndNotAfterStripe doc explains
		// why). Detected via errors.As on the concrete
		// TrialEndNotAfterStripeError rather than string-matching the
		// message; a plain wrapped sentinel (no struct) falls through to
		// the generic message below unchanged.
		message := err.Error()
		var notAfter *trial.TrialEndNotAfterStripeError
		if errors.As(err, &notAfter) && notAfter.AtStripeEnd() {
			message = "the requested trial_ends_at equals the trial end stripe currently holds; this is not a bad date, it means the local record fell behind stripe after a prior extension where stripe applied the change but this service failed to record it — see stripe_applied_local_write_failed. This requires manual reconciliation, not a different date."
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "trial_end_not_after_stripe",
			"message": message,
			"field":   "trial_ends_at",
		})
	case errors.Is(err, trial.ErrTrialEndTooFar):
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "trial_end_too_far",
			"message": "trial_ends_at is more than two years after the stripe billing anchor, which stripe does not allow",
			"field":   "trial_ends_at",
		})
	case errors.Is(err, trial.ErrStripeAppliedLocalWriteFailed):
		// Checked BEFORE ErrStripeCall: this sentinel wraps the local
		// commit's own error (extend.go's rollback path), never
		// ErrStripeCall — the Stripe call already SUCCEEDED on this path,
		// so the two can never both match on a real error value — but the
		// ordering still puts the more specific, higher-stakes sentinel
		// first so a future wrapping change cannot silently demote it to
		// the 502 branch below. This is the one divergence #358 accepts:
		// Stripe holds the new trial end and this service does not. 502 is
		// deliberately NOT used here — 502 tells the caller nothing
		// happened, and here something very much happened, to a real
		// billing date. No audit emit is attempted (see the handler body);
		// this log is the only signal, so it carries the reconciliation
		// values via the wrapped error itself.
		h.logger.Error("trial extend: stripe applied but local write failed; manual reconciliation required", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "stripe_applied_local_write_failed",
			"message": "stripe accepted the new trial end but this service failed to record it locally; stripe and this service now disagree and require manual reconciliation",
		})
	case errors.Is(err, trial.ErrStripeCall):
		// 502, not the 503 above: 503 means OUR idempotency store is
		// unreachable, 502 means the dependency failed. The distinction
		// matters to the operator because a 502 here also guarantees
		// nothing was written locally.
		h.logger.Error("trial extend: stripe call failed", "err", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "stripe_unavailable", "message": "could not update the trial end in stripe; nothing was changed",
		})
	default:
		h.logger.Error("trial extend failed", "err", err)
		// The driver's error text is logged server-side, never echoed.
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not extend trial",
		})
	}
}
