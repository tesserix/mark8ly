package migration

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PriorPlatformValidator is the WHOIS-age check interface. P7 will ship a
// concrete implementation backed by the tax-ID registry; until then
// NoOpValidator passes every request.
//
// TODO: wire P7's tax-ID package when it lands.
type PriorPlatformValidator interface {
	ValidateWhoisAge(ctx context.Context, domain string, minAgeDays int) error
}

// NoOpValidator is the default implementation that passes every WHOIS check.
// It is used until P7 ships the real validator.
type NoOpValidator struct{}

// ValidateWhoisAge always returns nil (passes every domain).
func (NoOpValidator) ValidateWhoisAge(context.Context, string, int) error { return nil }

// reviewStore is the narrow data-access interface the Handler needs.
// *Repository satisfies it automatically. Defined here so handler tests can
// substitute a fake without importing gorm.
type reviewStore interface {
	CreatePending(ctx context.Context, in CreatePendingInput) (*Review, error)
	Approve(ctx context.Context, id, reviewerID uuid.UUID, notes string) error
	Reject(ctx context.Context, id, reviewerID uuid.UUID, notes string) error
}

// Handler exposes the fast-path submit and CSM review endpoints.
type Handler struct {
	repo      reviewStore
	validator PriorPlatformValidator
	logger    *slog.Logger
}

// NewHandler constructs a Handler. If v is nil the NoOpValidator is used.
// If logger is nil slog.Default() is used.
func NewHandler(repo reviewStore, v PriorPlatformValidator, logger *slog.Logger) *Handler {
	if v == nil {
		v = NoOpValidator{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{repo: repo, validator: v, logger: logger}
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
// the X-User-Id trust header. Route wiring is deferred to Task 15.
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

	var repoErr error
	switch req.Decision {
	case "approve":
		repoErr = h.repo.Approve(c.Request.Context(), id, reviewerID, req.Notes)
	case "reject":
		repoErr = h.repo.Reject(c.Request.Context(), id, reviewerID, req.Notes)
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

	// TODO: migration fast-path approval/rejection emails land when the email
	// package and StoreSubscription.email column exist (deferred from P5 scope).
	h.logger.Info("migration fast-path decided; email deferred",
		"review_id", id.String(),
		"decision", req.Decision,
		"reviewer_id", reviewerID.String())

	statusMap := map[string]string{
		"approve": "approved",
		"reject":  "rejected",
	}
	c.JSON(http.StatusOK, gin.H{"status": statusMap[req.Decision]})
}
