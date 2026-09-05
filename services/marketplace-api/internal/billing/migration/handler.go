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

// Handler exposes the fast-path submit and CSM review endpoints.
type Handler struct {
	repo      reviewStore
	validator PriorPlatformValidator
	logger    *slog.Logger
	audit     *audit.Emitter // optional — nil-safe, see WithAudit
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

	// TODO: migration fast-path approval/rejection emails land when the email
	// package and StoreSubscription.email column exist (deferred from P5 scope).
	h.logger.Info("migration fast-path decided; email deferred",
		"review_id", id.String(),
		"decision", req.Decision,
		"reviewer_id", reviewerID.String())

	c.JSON(http.StatusOK, gin.H{"status": statusMap[req.Decision]})
}
