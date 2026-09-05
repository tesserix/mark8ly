package migration

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/email"
)

// PriorPlatformValidator is the domain-registration-age check interface. A
// real implementation needs a WHOIS or RDAP lookup; RDAP is the better target,
// being the IETF successor with structured JSON and more predictable rate
// limits than scraping WHOIS text.
//
// Whatever implements this must have THREE outcomes, not two: verified,
// too-new, and undeterminable. Registrar privacy proxies obscure the
// registration date for a large share of domains, and an undeterminable
// result must route to human review rather than passing (#706).
//
// A previous comment here said "wire P7's tax-ID package when it lands". That
// was misattributed: the tax-ID package now exists (internal/billing/tax) and
// is unrelated — it validates tax identifiers, not domain age. Following it
// leads nowhere.
type PriorPlatformValidator interface {
	ValidateWhoisAge(ctx context.Context, domain string, minAgeDays int) error
}

// UnenforcedDomainAge accepts every domain, whatever its registration date.
//
// Named for its CONSEQUENCE rather than its implementation. It was previously
// called NoOpValidator, which reads as a harmless placeholder at the wiring
// site; it is not. With this bound, the 90-day check in Submit cannot reject
// anything, so a merchant may register a domain today, submit it as
// whois_domain evidence, and reach a pending review.
//
// That is survivable ONLY because the CSM review is the real control. What it
// must not do is look automated: a reviewer who assumes the age was checked is
// spending attention on a guarantee nobody made. Callers wiring this are
// expected to log the fact — see cmd/marketplace-api/main.go.
type UnenforcedDomainAge struct{}

// ValidateWhoisAge always returns nil: no domain is ever rejected for age.
func (UnenforcedDomainAge) ValidateWhoisAge(context.Context, string, int) error { return nil }

// reviewStore is the narrow data-access interface the Handler needs.
// *Repository satisfies it automatically. Defined here so handler tests can
// substitute a fake without importing gorm.
type reviewStore interface {
	CreatePending(ctx context.Context, in CreatePendingInput) (*Review, error)
	Approve(ctx context.Context, id, reviewerID uuid.UUID, notes string) (*Review, error)
	Reject(ctx context.Context, id, reviewerID uuid.UUID, notes string) (*Review, error)
}

// RecipientLookup resolves the merchant-facing billing address and store name
// for a store, returning "" for either when it cannot.
//
// A function rather than a database handle so the Handler keeps depending on
// narrow behaviour instead of gorm — the same reason reviewStore exists. The
// production implementation is a closure over the connection in main.go
// (subscription.BillingEmailFor + subscription.StoreNameFor); tests supply a
// literal. It cannot fail: a missing address is "" and is classified by
// email.ValidateRecipient like any other undeliverable one.
type RecipientLookup func(ctx context.Context, storeID uuid.UUID) (address, storeName string)

// CounterIncrementer is a one-method counter so tests can stub it.
type CounterIncrementer interface{ Inc() }

// SentCounter counts decision notices actually delivered, labeled by template.
// SkipCounter counts those deliberately not sent, labeled by template and
// reason (see email.SkipReason for the reason vocabulary).
//
// Both are declared here rather than imported from the trial package, which
// declares its own for the same reason: a shared home would make one billing
// sub-package depend on another purely for a two-line interface.
type SentCounter interface {
	WithTemplate(template string) CounterIncrementer
}

// SkipCounter — see SentCounter.
type SkipCounter interface {
	WithTemplateReason(template, reason string) CounterIncrementer
}

// Handler exposes the fast-path submit and CSM review endpoints.
type Handler struct {
	repo      reviewStore
	validator PriorPlatformValidator
	logger    *slog.Logger
	audit     *audit.Emitter // optional — nil-safe, see WithAudit

	// Email is optional in the same way audit is: a Handler built without
	// WithEmail decides reviews and sends nothing.
	mailer    email.Client
	recipient RecipientLookup
	sent      SentCounter
	skip      SkipCounter
}

// NewHandler constructs a Handler. A nil validator falls back to
// UnenforcedDomainAge, which accepts every domain — see its doc comment.
// If logger is nil slog.Default() is used.
func NewHandler(repo reviewStore, v PriorPlatformValidator, logger *slog.Logger) *Handler {
	if v == nil {
		v = UnenforcedDomainAge{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{repo: repo, validator: v, logger: logger}
}

// WithAudit attaches an audit emitter so approve/reject decisions are
// recorded in the audit log. Omitting this call leaves h.audit nil, which
// audit.Emitter.Emit tolerates (nil-receiver no-op) — safe for tests and
// opted-out wiring.
func (h *Handler) WithAudit(e *audit.Emitter) *Handler {
	h.audit = e
	return h
}

// WithEmail attaches the transactional client used to tell a merchant their
// fast-path request was decided, the lookup that finds their address, and the
// optional delivered/skipped counters.
//
// Chainable rather than constructor parameters so existing NewHandler callers
// compile untouched, mirroring trial.ExpiryCron.WithEmail. A Handler missing
// either the client or the lookup sends nothing: a CSM's decision must never
// fail because email is unconfigured.
func (h *Handler) WithEmail(cl email.Client, lookup RecipientLookup, sent SentCounter, skip SkipCounter) *Handler {
	h.mailer = cl
	h.recipient = lookup
	h.sent = sent
	h.skip = skip
	return h
}

// RegisterInternalRoutes mounts the CSM review route behind
// auth.HeaderTrustAuth on the supplied /internal group, so the full path is
// POST /internal/csm/migration-fast-path/:id/review. Call this once per
// engine (mode.Both and mode.Admin both mount an /internal group) so the
// route is never present on one engine and missing on the other (#323).
func (h *Handler) RegisterInternalRoutes(g *gin.RouterGroup, internalSecret string) {
	g.POST("/csm/migration-fast-path/:id/review", auth.HeaderTrustAuth(internalSecret), h.Review)
}

type submitRequest struct {
	EvidenceType  string `json:"evidence_type"  binding:"required,oneof=whois_domain platform_screenshot"`
	EvidenceURL   string `json:"evidence_url"   binding:"required,url"`
	PriorPlatform string `json:"prior_platform" binding:"omitempty,oneof=shopify woocommerce bigcommerce"`
	WhoisDomain   string `json:"whois_domain"`
}

// Submit handles POST /admin/stores/:storeId/migration-fast-path/submit.
// The route is mounted behind the standard admin auth middleware (GIPAuth +
// TenantMiddleware). Route wiring is deferred to Task 15.
func (h *Handler) Submit(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id"})
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_tenant_id"})
		return
	}

	var req submitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}

	if req.EvidenceType == "whois_domain" {
		if req.WhoisDomain == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "whois_domain_required"})
			return
		}
		if err := h.validator.ValidateWhoisAge(c.Request.Context(), req.WhoisDomain, 90); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "whois_too_young"})
			return
		}
	}

	review, err := h.repo.CreatePending(c.Request.Context(), CreatePendingInput{
		TenantID:      tenantID,
		StoreID:       storeID,
		EvidenceType:  req.EvidenceType,
		EvidenceURL:   req.EvidenceURL,
		PriorPlatform: req.PriorPlatform,
		WhoisDomain:   req.WhoisDomain,
	})
	switch {
	case errors.Is(err, ErrAlreadyPending):
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "already_pending"})
		return
	case err != nil:
		h.logger.Error("migration fast-path submit failed", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "submit_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"review_id": review.ID, "status": "pending"})
}

type reviewRequest struct {
	Decision string `json:"decision" binding:"required,oneof=approve reject"`
	Notes    string `json:"notes"    binding:"required,min=3,max=2000"`
}

// Review handles POST /internal/csm/migration-fast-path/:id/review. The route
// is mounted behind auth.HeaderTrustAuth so only internal callers (CSM tooling)
// reach it. The CSM's user_id is set on the gin context by that middleware via
// the X-User-Id trust header. Mounted via RegisterInternalRoutes.
func (h *Handler) Review(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}
	reviewerID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing_reviewer"})
		return
	}

	var req reviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}

	var (
		review  *Review
		repoErr error
	)
	switch req.Decision {
	case "approve":
		review, repoErr = h.repo.Approve(c.Request.Context(), id, reviewerID, req.Notes)
	case "reject":
		review, repoErr = h.repo.Reject(c.Request.Context(), id, reviewerID, req.Notes)
	}

	switch {
	case errors.Is(repoErr, ErrNotFound):
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	case repoErr != nil:
		h.logger.Error("migration fast-path review failed",
			"err", repoErr,
			"decision", req.Decision)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "review_failed"})
		return
	}

	statusMap := map[string]string{
		"approve": "approved",
		"reject":  "rejected",
	}

	// The CSM caller addresses the review by id alone and need not send a
	// tenant header — audit.Emit's gin-context fallback for TenantID/StoreID
	// cannot be relied on here (#310: an event with no tenant is silently
	// dropped, not written with a NULL tenant). Set both explicitly from the
	// review row the repository just handed back.
	h.audit.Emit(c, audit.Event{
		Action:       "migration_fast_path.review_" + statusMap[req.Decision],
		ResourceType: "migration_fast_path_review",
		ResourceID:   id.String(),
		TenantID:     review.TenantID,
		StoreID:      review.StoreID,
		Metadata: map[string]any{
			"decision":    req.Decision,
			"reviewer_id": reviewerID.String(),
			"notes":       req.Notes,
		},
	})

	h.logger.Info("migration fast-path decided",
		"review_id", id.String(),
		"decision", req.Decision,
		"reviewer_id", reviewerID.String())

	h.notifyDecision(c.Request.Context(), review, req.Decision)

	c.JSON(http.StatusOK, gin.H{"status": statusMap[req.Decision]})
}

// decisionTemplates maps a decision to the merchant-facing template. Both
// branches must be present: a decision with no template would silently send
// nothing, which is the failure this whole change exists to remove.
var decisionTemplates = map[string]email.TemplateID{
	"approve": email.TemplateMigrationFastPathApproved,
	"reject":  email.TemplateMigrationFastPathRejected,
}

// notifyDecision tells the merchant their fast-path request was approved or
// rejected. Strictly best-effort: the review row is committed and the audit
// event emitted before this runs, so every failure here is logged and counted
// and none is returned. The CSM's write is not undone by a mail outage.
//
// The review notes are deliberately absent from the template data. They are
// authored by a CSM for internal review, and nothing in their validation
// (required, 3–2000 chars) makes them fit to show the merchant they describe.
func (h *Handler) notifyDecision(ctx context.Context, review *Review, decision string) {
	if h.mailer == nil || h.recipient == nil {
		// Email is not configured for this deployment. The decision itself
		// has already succeeded.
		return
	}
	template, ok := decisionTemplates[decision]
	if !ok {
		// Unreachable: the binding tag admits only approve|reject. Guarded
		// so a third decision added later fails loudly here rather than
		// quietly sending nothing.
		h.logger.Error("migration fast-path: no template for decision",
			"review_id", review.ID.String(), "decision", decision)
		return
	}

	to, storeName := h.recipient(ctx, review.StoreID)

	// Classify before the provider sees the address: the placeholder
	// billing+<uuid>@mark8ly.local addresses minted at bootstrap would
	// hard-bounce and cost sender reputation.
	if err := email.ValidateRecipient(to); err != nil {
		h.countSkip(template, email.SkipReason(err))
		h.logger.Warn("migration fast-path decision notice not sent",
			"review_id", review.ID.String(),
			"store_id", review.StoreID.String(),
			"reason", email.SkipReason(err))
		return
	}

	if err := h.mailer.Send(ctx, template, to, map[string]any{
		"store_id":   review.StoreID.String(),
		"tenant_id":  review.TenantID.String(),
		"store_name": storeName,
	}); err != nil {
		h.countSkip(template, email.SkipReason(err))
		h.logger.Warn("migration fast-path decision notice not sent",
			"review_id", review.ID.String(),
			"store_id", review.StoreID.String(),
			"reason", email.SkipReason(err),
			"err", err.Error())
		return
	}

	if h.sent != nil {
		h.sent.WithTemplate(string(template)).Inc()
	}
	h.logger.Info("migration fast-path decision notice sent",
		"review_id", review.ID.String(),
		"store_id", review.StoreID.String())
}

func (h *Handler) countSkip(template email.TemplateID, reason string) {
	if h.skip != nil {
		h.skip.WithTemplateReason(string(template), reason).Inc()
	}
}
