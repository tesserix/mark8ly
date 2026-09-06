package platformadmin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/tenantdiscount"
	"github.com/mark8ly/marketplace-api/internal/idempotency"
)

// TenantDiscounter is the subset of internal/billing/tenantdiscount this
// handler needs, declared locally so the handler is stubbable — the same
// reason TrialExtender and TenantLifecycle are declared here rather than
// imported. *tenantdiscount.Service satisfies it.
//
// Unlike TrialExtender there is no audit function parameter beside it: the
// domain writes its own audit row with EmitTx, INSIDE each store's
// transaction, which is the property tesserix-home#331 asks for and the one
// an emit made out here could not have. See the mount guard in routes.go for
// why this handler is still gated on the Emitter being wired.
type TenantDiscounter interface {
	Apply(ctx context.Context, in tenantdiscount.Input) (tenantdiscount.Result, error)
	Remove(ctx context.Context, in tenantdiscount.Input) (tenantdiscount.Result, error)
}

// TenantDiscounterFuncs adapts a pair of free functions to TenantDiscounter.
//
// A struct of fields rather than a single func type like TrialExtenderFunc:
// that pattern works only for a ONE-method interface, and this one has two.
// A nil field panics on call, which is the same failure a nil TenantDiscounter
// would produce and is not a case production wiring can reach — main.go
// passes *tenantdiscount.Service, never this.
type TenantDiscounterFuncs struct {
	ApplyFunc  func(ctx context.Context, in tenantdiscount.Input) (tenantdiscount.Result, error)
	RemoveFunc func(ctx context.Context, in tenantdiscount.Input) (tenantdiscount.Result, error)
}

func (f *TenantDiscounterFuncs) Apply(ctx context.Context, in tenantdiscount.Input) (tenantdiscount.Result, error) {
	return f.ApplyFunc(ctx, in)
}

func (f *TenantDiscounterFuncs) Remove(ctx context.Context, in tenantdiscount.Input) (tenantdiscount.Result, error) {
	return f.RemoveFunc(ctx, in)
}

// BillingTenantDiscountHandler serves POST and DELETE
// /admin/billing/tenants/:tenantID/discount (#660).
//
// The path parameter is a BARE tenant uuid, like every other handler in this
// package (tenant_lifecycle.go). The console addresses tenants with a
// namespaced "<source>:<id>"; platform-api splits that before the request
// reaches this service, so a namespaced value arriving here is a 400 and not
// something to parse leniently.
type BillingTenantDiscountHandler struct {
	db     *gorm.DB
	svc    TenantDiscounter
	logger *slog.Logger
}

// NewBillingTenantDiscountHandler constructs the handler. logger may be nil —
// slog.Default() is substituted, matching NewBillingTrialExtendHandler, so a
// nil logger can never silence the divergence log below.
func NewBillingTenantDiscountHandler(db *gorm.DB, svc TenantDiscounter, logger *slog.Logger) *BillingTenantDiscountHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &BillingTenantDiscountHandler{db: db, svc: svc, logger: logger}
}

// Register mounts both routes on the supplied group.
func (h *BillingTenantDiscountHandler) Register(g *gin.RouterGroup) {
	g.POST("/admin/billing/tenants/:tenantID/discount", h.apply)
	g.DELETE("/admin/billing/tenants/:tenantID/discount", h.remove)
}

// tenantDiscountRequest is the body of BOTH routes. The DELETE carries one
// too: it needs the coupon id to know which discount to take off (the
// subscription may carry several, and a merchant's own promo must survive),
// and it needs a reason for the same rule that makes the POST need one —
// tesserix-home#331's "removal is as audited as application".
type tenantDiscountRequest struct {
	CouponID string `json:"coupon_id"`
	Reason   string `json:"reason"`
}

// tenantDiscountStoreResponse is one store's line in the report. The
// omitempty fields are genuinely absent rather than empty for the outcomes
// that have no such id — a card-less trialing store has no Stripe
// subscription, and a store with no subscription row has neither.
type tenantDiscountStoreResponse struct {
	StoreID              string `json:"store_id"`
	SubscriptionID       string `json:"subscription_id,omitempty"`
	StripeCustomerID     string `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID string `json:"stripe_subscription_id,omitempty"`
	Outcome              string `json:"outcome"`

	// FailureCode and FailureReason are set only for OutcomeFailed.
	// FailureReason is a FIXED message chosen from the code — the domain's
	// error text can carry driver output and is logged server-side instead.
	FailureCode   string `json:"failure_code,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
}

// tenantDiscountResponse is the fan-out report. Stores is the point of it:
// outcomes differ per store, and flattening them to one status would discard
// exactly what the console renders.
type tenantDiscountResponse struct {
	TenantID  string `json:"tenant_id"`
	CouponID  string `json:"coupon_id"`
	Operation string `json:"operation"` // "apply" | "remove"
	Reason    string `json:"reason"`
	// PerformedAt is the instant the fan-out started, not per store.
	PerformedAt string `json:"performed_at"`

	// Status is "ok" (no store failed), "partial" (some did) or "failed"
	// (all did). A summary line, never a substitute for reading Stores.
	Status string `json:"status"`

	// RequiresReconciliation is set only when at least one store hit the
	// ErrStripeChangedAuditWriteFailed divergence: Stripe holds — or no
	// longer holds — the discount and no audit row explains it. Omitted
	// rather than false so its presence is the signal.
	RequiresReconciliation bool `json:"requires_reconciliation,omitempty"`

	Stores []tenantDiscountStoreResponse `json:"stores"`
}

func (h *BillingTenantDiscountHandler) apply(c *gin.Context) {
	h.handle(c, "apply", h.svc.Apply)
}

func (h *BillingTenantDiscountHandler) remove(c *gin.Context) {
	h.handle(c, "remove", h.svc.Remove)
}

// handle is the whole endpoint. Apply and Remove differ only in which domain
// call they make and in the operation name that scopes their idempotency key
// and labels their report, so the validation, the idempotency dance and the
// error mapping are written once.
func (h *BillingTenantDiscountHandler) handle(
	c *gin.Context,
	op string,
	call func(ctx context.Context, in tenantdiscount.Input) (tenantdiscount.Result, error),
) {
	idemKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idemKey == "" {
		// A write that cannot be retried safely is worse than one that
		// refuses to start. The DELETE is held to the same rule as the
		// POST: revoking a discount is as much a billing change as
		// granting one.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "idempotency_key_required", "message": "the Idempotency-Key header is required for this endpoint",
		})
		return
	}

	tenantID, err := uuid.Parse(strings.TrimSpace(c.Param("tenantID")))
	if err != nil {
		// 400, not 500: a malformed id is the caller's error. Same shape as
		// tenant_lifecycle.go's invalid_tenant_id so the console handles
		// every write on this surface the same way. A namespaced
		// "<source>:<id>" from the console lands here too — platform-api
		// splits it upstream, and guessing at one here would silently
		// address a tenant nobody named.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_tenant_id", "message": "tenant id is not a valid uuid", "field": "tenant_id",
		})
		return
	}

	var req tenantDiscountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// gin returns io.EOF for a completely empty body, so an omitted
		// body is rejected HERE. `{}` binds to the zero value and is what
		// the two field checks below exist to catch.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "request body could not be parsed",
		})
		return
	}

	couponID := strings.TrimSpace(req.CouponID)
	if couponID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_coupon_id", "message": "coupon_id is required", "field": "coupon_id",
		})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		// Mandatory, both directions. An audit row that says what happened
		// without saying why is the gap this series exists to close, and
		// there is no reason_code vocabulary for a discount to fall back on
		// the way ExtendReasonCodes serves trial extension.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_reason", "message": "reason is required", "field": "reason",
		})
		return
	}
	if len(reason) > maxReasonLen {
		// truncateUTF8, never reason[:maxReasonLen]: the domain puts this
		// string straight into the audit row's jsonb metadata, and a
		// half-rune left by a raw byte slice fails that marshal. Under
		// EmitTx that failure is no longer "the write succeeded
		// unaudited" — the audit insert is inside the store's
		// transaction, so it rolls the store back and the discount
		// SILENTLY DOES NOT APPLY. See truncateUTF8's own doc comment for
		// the pre-EmitTx version of this hazard.
		reason = truncateUTF8(reason, maxReasonLen)
	}

	// Namespaced by OPERATION as well as by tenant. idempotency_keys.key is
	// a bare primary key shared by the whole service and the caller's raw
	// header is theirs to choose, so without the tenant an operator's key
	// would replay one tenant's report onto another. The operation is in
	// there for the same reason and one step further: apply and remove are
	// opposite changes on the same path, so a key reused to REVOKE a
	// discount the operator just granted would otherwise replay the grant's
	// stored report and leave the discount on.
	scopedKey := "tenant_discount:" + op + ":" + tenantID.String() + ":" + idemKey

	// Reserve immediately before the work, AFTER every validation step
	// above: validation is deterministic, so two concurrent callers with
	// the same key either both pass it or both fail it identically, and
	// only then race on Reserve. Reserving any earlier would let a failed
	// attempt (a malformed body, a blank coupon id, a missing reason, or a
	// domain refusal below) leave the key claimed with an empty Response
	// for the full TTL — turning a mistyped request into a key that answers
	// 409 in_progress for a day.
	if h.db != nil {
		// Reserve BEFORE doing the work, not Lookup-then-Save after: two
		// pods handling the same retry can both miss a plain Lookup before
		// either writes. Reserve claims the key itself first.
		claimed, err := idempotency.Reserve(c.Request.Context(), h.db, scopedKey, tenantID.String(), time.Now().UTC(), idempotency.DefaultTTL)
		if err != nil {
			// Fail CLOSED: a caller that cannot be told whether this key
			// was already used must not be allowed through to a second
			// fan-out and a second set of audit rows.
			h.logger.Error("tenant discount: idempotency reserve failed",
				"op", op, "tenant_id", tenantID.String(), "err", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "unavailable", "message": "could not verify idempotency key",
			})
			return
		}
		if !claimed {
			stored, ok, err := idempotency.Lookup(c.Request.Context(), h.db, scopedKey)
			if err != nil {
				h.logger.Error("tenant discount: idempotency lookup failed",
					"op", op, "tenant_id", tenantID.String(), "err", err)
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": "unavailable", "message": "could not verify idempotency key",
				})
				return
			}
			if ok {
				// A replay: return the stored bytes verbatim, without a
				// second fan-out and so without a second Stripe call or a
				// second audit row.
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

	performedAt := time.Now().UTC()
	res, err := call(c.Request.Context(), tenantdiscount.Input{
		TenantID: tenantID,
		CouponID: couponID,
		Reason:   reason,
		// The gin context is what lets audit.buildEntry derive the
		// operator, capability and IP for every row the fan-out writes.
		C: c,
	})
	if err != nil {
		h.releaseReservation(c, op, scopedKey)
		h.respondDiscountErr(c, op, err)
		return
	}

	resp := h.buildResponse(op, reason, performedAt, res)

	// The key is COMPLETED even for a partial result. The stores that
	// committed must not be re-applied by a retry of the same key; retrying
	// the stores that failed is a new operator decision and takes a new
	// key, which the domain answers idempotently anyway (a store that
	// already carries the coupon reports already_applied).
	if h.db != nil {
		if body, err := json.Marshal(resp); err != nil {
			h.logger.Error("tenant discount: could not marshal response for idempotency save", "op", op, "err", err)
		} else if err := idempotency.Complete(c.Request.Context(), h.db, scopedKey, body); err != nil {
			// Logged, not surfaced: the fan-out already happened, and
			// failing the response would make the caller retry work that
			// succeeded.
			h.logger.Error("tenant discount: idempotency complete failed",
				"op", op, "tenant_id", tenantID.String(), "err", err)
		}
	}

	c.JSON(http.StatusOK, resp)
}

// buildResponse turns the domain's report into the wire shape, and logs the
// per-store failures whose detail must not travel in the response.
//
// Deliberately NO failure audit row is written here. See the package-level
// note in routes.go's mount guard for the decision; in short: the domain
// already writes an EmitTx row for every store whose transaction COMMITS,
// including the ones where nothing was applied, so the only unrecorded cases
// are stores that rolled back — and for the divergence, a handler-written
// row saying "failed" would be actively false, because Stripe did change.
func (h *BillingTenantDiscountHandler) buildResponse(
	op, reason string, performedAt time.Time, res tenantdiscount.Result,
) tenantDiscountResponse {
	resp := tenantDiscountResponse{
		TenantID:    res.TenantID.String(),
		CouponID:    res.CouponID,
		Operation:   op,
		Reason:      reason,
		PerformedAt: performedAt.Format(time.RFC3339),
		Stores:      make([]tenantDiscountStoreResponse, 0, len(res.Stores)),
	}

	failed := 0
	for _, s := range res.Stores {
		line := tenantDiscountStoreResponse{
			StoreID:              s.StoreID.String(),
			StripeCustomerID:     s.StripeCustomerID,
			StripeSubscriptionID: s.StripeSubscriptionID,
			Outcome:              string(s.Outcome),
		}
		if s.SubscriptionID != uuid.Nil {
			line.SubscriptionID = s.SubscriptionID.String()
		}
		if s.Outcome == tenantdiscount.OutcomeFailed {
			failed++
			line.FailureCode, line.FailureReason = storeFailure(s)
			if line.FailureCode == codeStripeChangedAuditWriteFailed {
				resp.RequiresReconciliation = true
			}
			// The domain already logs the divergence with the three ids a
			// human needs. This line adds the ones only the handler knows —
			// which operator request this store belonged to — and is the
			// only record of an ordinary per-store failure, whose driver
			// text never travels in the response.
			h.logger.Error("tenant discount: store failed",
				"op", op,
				"tenant_id", res.TenantID.String(),
				"store_id", s.StoreID.String(),
				"coupon_id", res.CouponID,
				"failure_code", line.FailureCode,
				"err", s.Err)
		}
		resp.Stores = append(resp.Stores, line)
	}

	switch {
	case failed == 0:
		resp.Status = "ok"
	case failed == len(res.Stores):
		resp.Status = "failed"
	default:
		resp.Status = "partial"
	}
	return resp
}

// codeStripeChangedAuditWriteFailed names the one divergence this design
// accepts: Stripe's subscription changed and the audit row that would have
// explained it did not commit. It is its own code — never the routine
// audit_write_failed or commit_failed the domain also records on the same
// StoreResult, and never a plain database error — for the same reason
// trial's stripe_applied_local_write_failed is: something very much happened,
// to a real billing arrangement, and it needs manual reconciliation.
const codeStripeChangedAuditWriteFailed = "stripe_changed_audit_write_failed"

// storeFailure maps a failed store to a stable code and a fixed message.
//
// The divergence is checked FIRST, before the Stripe branch. Today the two
// can never both match a real error value — ErrStripeChangedAuditWriteFailed
// wraps the audit insert's or the commit's own error, on a path where the
// Stripe call already succeeded — but the ordering still puts the more
// specific, higher-stakes sentinel first so a future wrapping change cannot
// silently demote it to the ordinary Stripe failure.
//
// The message is composed here, never taken from err.Error(): the domain
// wraps driver output, which is logged server-side and not echoed.
func storeFailure(s tenantdiscount.StoreResult) (code, message string) {
	switch {
	case errors.Is(s.Err, tenantdiscount.ErrStripeChangedAuditWriteFailed):
		return codeStripeChangedAuditWriteFailed,
			"stripe accepted the discount change but the audit row was not written, so the change was rolled back locally and stripe and this service now disagree; this store requires manual reconciliation"
	case errors.Is(s.Err, tenantdiscount.ErrStripeCall):
		return string(tenantdiscount.FailureStripeCall),
			"the stripe call failed and nothing was changed for this store; it can be retried"
	case s.FailureCode == tenantdiscount.FailureLoadSubscription:
		return string(tenantdiscount.FailureLoadSubscription),
			"this store's subscription could not be read, so nothing was changed for it"
	case s.FailureCode == tenantdiscount.FailureAuditWrite, s.FailureCode == tenantdiscount.FailureCommit:
		// Reached only when Stripe was NOT changed — a store whose outcome
		// needed no Stripe call (pending, no_subscription, no_stripe_customer)
		// and whose audit row or commit then failed. The transaction rolled
		// back and nothing diverged.
		return string(s.FailureCode),
			"this store's audit row could not be committed, so nothing was changed for it"
	default:
		return "internal_error", "this store failed for an unexpected reason; see the service logs"
	}
}

// releaseReservation drops a Reserve claim that will never be Completed,
// because the domain refused or failed the WHOLE request. Without it the
// reservation would sit on the key with an empty Response for the full TTL,
// answering 409 in_progress to a corrected retry for a day.
//
// A per-store failure does NOT come through here: those requests completed,
// and their report is stored — see the Complete call in handle.
//
// The Release failure is logged, not surfaced: the caller already has the
// real error from respondDiscountErr, and a failed Release just means the
// reservation lives out its TTL instead of being cleared early.
func (h *BillingTenantDiscountHandler) releaseReservation(c *gin.Context, op, scopedKey string) {
	if h.db == nil {
		return
	}
	if err := idempotency.Release(c.Request.Context(), h.db, scopedKey); err != nil {
		h.logger.Error("tenant discount: idempotency release failed", "op", op, "err", err)
	}
}

// respondDiscountErr maps the errors Apply and Remove return for the WHOLE
// request to distinct statuses and codes, so the console can tell "this
// tenant owns no stores" from "the store lookup broke" rather than getting
// one opaque refusal.
//
// Apply and Remove return ErrNoTenant, ErrNoCoupon, ErrNoStores,
// ErrOverrideAlreadyRecorded, or a wrapped store-lookup or override-record
// error, and nothing else — a single store's failure is a StoreResult, not an
// error, because its siblings still committed. The two
// per-store sentinels are matched here anyway, divergence first, so that a
// future change which does surface one of them at the request level cannot
// land it in the default branch and report a live billing divergence as a
// generic internal error.
func (h *BillingTenantDiscountHandler) respondDiscountErr(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, tenantdiscount.ErrNoStores):
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "no_stores",
			"message": "this tenant owns no stores, so there is no subscription to carry the discount",
		})
	case errors.Is(err, tenantdiscount.ErrNoTenant):
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_tenant_id", "message": "tenant id is required", "field": "tenant_id",
		})
	case errors.Is(err, tenantdiscount.ErrNoCoupon):
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_coupon_id", "message": "coupon_id is required", "field": "coupon_id",
		})
	case errors.Is(err, tenantdiscount.ErrOverrideAlreadyRecorded):
		// 409 and not 400: the request is well formed and would have been
		// accepted a moment earlier or after a removal. Nothing was sent to
		// Stripe, so a corrected retry is safe.
		//
		// The message says what to do rather than restating the refusal,
		// because the operator's next move is not obvious: replacing an
		// override is a removal and then an application, two audited acts,
		// and this service will not fold them into one.
		c.JSON(http.StatusConflict, gin.H{
			"error":   "override_already_recorded",
			"message": "this tenant already holds a platform discount applied by this service; remove it before applying a different coupon",
		})
	case errors.Is(err, tenantdiscount.ErrStripeChangedAuditWriteFailed):
		// Checked BEFORE ErrStripeCall, and 500 rather than the 502 below:
		// a 502 tells the caller nothing happened, and here something very
		// much happened to a real billing arrangement. The wrapped
		// AuditDivergenceError carries the coupon, subscription and
		// customer ids; they go to the log, not to the response.
		h.logger.Error("tenant discount: stripe changed but the audit row was not written; manual reconciliation required",
			"op", op, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   codeStripeChangedAuditWriteFailed,
			"message": "stripe accepted the discount change but the audit row was not written; stripe and this service now disagree and require manual reconciliation",
		})
	case errors.Is(err, tenantdiscount.ErrStripeCall):
		// 502, not the 503 above: 503 means OUR idempotency store is
		// unreachable, 502 means the dependency failed.
		h.logger.Error("tenant discount: stripe call failed", "op", op, "err", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "stripe_unavailable", "message": "could not change the discount in stripe; nothing was changed",
		})
	default:
		h.logger.Error("tenant discount failed", "op", op, "err", err)
		// The driver's error text is logged server-side, never echoed.
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not change the tenant discount",
		})
	}
}
