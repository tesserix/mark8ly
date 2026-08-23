package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/domain"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

type DomainsHandler struct {
	svc       *domain.Service
	storeRepo stores.Repository
	audit     *audit.Emitter // optional — nil-safe
	logger    *slog.Logger
}

func NewDomainsHandler(svc *domain.Service, storeRepo stores.Repository, logger *slog.Logger) *DomainsHandler {
	return &DomainsHandler{svc: svc, storeRepo: storeRepo, logger: logger}
}

// WithAudit attaches an audit emitter so domain lifecycle events are
// recorded. Nil-safe.
func (h *DomainsHandler) WithAudit(e *audit.Emitter) *DomainsHandler {
	h.audit = e
	return h
}

type DomainResponse struct {
	ID          string  `json:"id"`
	Domain      string  `json:"domain"`
	DNSMethod   string  `json:"dns_method"`
	CnameTarget *string `json:"cname_target,omitempty"`
	// The TXT ownership proof, present only while it is outstanding.
	ChallengeHost  *string `json:"challenge_host,omitempty"`
	ChallengeValue *string `json:"challenge_value,omitempty"`
	Status         string  `json:"status"`
	SSLStatus      string  `json:"ssl_status"`
	VerifiedAt     *string `json:"verified_at,omitempty"`
	Error          *string `json:"error,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

func (h *DomainsHandler) toDomainResponse(d domain.CustomDomain) DomainResponse {
	resp := DomainResponse{
		ID:          d.ID.String(),
		Domain:      d.Domain,
		DNSMethod:   string(d.DNSMethod),
		CnameTarget: d.CnameTarget,
		Status:      string(d.Status),
		SSLStatus:   string(d.SSLStatus),
		Error:       d.ErrorMessage,
		CreatedAt:   d.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if d.VerifiedAt != nil {
		t := d.VerifiedAt.Format("2006-01-02T15:04:05Z")
		resp.VerifiedAt = &t
	}
	if host, token, required := h.svc.Challenge(&d); required {
		resp.ChallengeHost = &host
		resp.ChallengeValue = &token
	}
	return resp
}

func (h *DomainsHandler) List(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	domains, err := h.svc.List(c.Request.Context(), storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]DomainResponse, 0, len(domains))
	for _, d := range domains {
		out = append(out, h.toDomainResponse(d))
	}

	c.JSON(http.StatusOK, gin.H{"data": out})
}

type AddDomainRequest struct {
	Domain     string `json:"domain" binding:"required"`
	DNSMethod  string `json:"dns_method"`
	CFAPIToken string `json:"cf_api_token"`
}

type ValidateDomainRequest struct {
	Domain string `json:"domain" binding:"required"`
}

type ValidateDomainResponse struct {
	Valid     bool   `json:"valid"`
	Canonical string `json:"canonical,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Validate handles POST /admin/.../domains/validate. Used by the UI as
// a debounced pre-flight before the merchant clicks "Add domain", and
// re-run server-side inside Add as defense-in-depth. Always returns
// HTTP 200 with a discriminated body so the client doesn't have to
// pattern-match on status codes.
func (h *DomainsHandler) Validate(c *gin.Context) {
	var req ValidateDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, ValidateDomainResponse{
			Valid: false,
			Error: "Domain is required.",
		})
		return
	}
	canonical, err := h.svc.ValidateDomain(c.Request.Context(), req.Domain)
	if err != nil {
		// Strip the sentinel prefix so the merchant sees the friendly
		// half of the message ("xyz doesn't appear to be a registered
		// domain.") without the engineering label.
		msg := err.Error()
		const prefix = "invalid domain: "
		if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
			msg = msg[len(prefix):]
		}
		c.JSON(http.StatusOK, ValidateDomainResponse{
			Valid: false,
			Error: msg,
		})
		return
	}
	c.JSON(http.StatusOK, ValidateDomainResponse{
		Valid:     true,
		Canonical: canonical,
	})
}

func (h *DomainsHandler) Add(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	// Resolve store slug so cname_target can be populated correctly.
	store, err := h.storeRepo.GetByIDForTenant(c.Request.Context(), storeID.String(), tenantID.String())
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	storeSlug := store.Slug

	var req AddDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	method := domain.DNSMethodManual
	if req.DNSMethod == "cloudflare" {
		method = domain.DNSMethodCloudflare
	}

	d, err := h.svc.Add(c.Request.Context(), domain.AddInput{
		TenantID:   tenantID,
		StoreID:    storeID,
		StoreSlug:  storeSlug,
		Domain:     req.Domain,
		DNSMethod:  method,
		CFAPIToken: req.CFAPIToken,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	h.audit.Emit(c, audit.Event{
		Action:       "domain.added",
		ResourceType: "domain",
		ResourceID:   d.ID.String(),
		Metadata: map[string]any{
			"domain":     d.Domain,
			"dns_method": string(d.DNSMethod),
		},
	})
	c.JSON(http.StatusCreated, gin.H{"data": h.toDomainResponse(*d)})
}

func (h *DomainsHandler) Remove(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	if err := h.svc.Remove(c.Request.Context(), storeID, id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	h.audit.Emit(c, audit.Event{
		Action:       "domain.removed",
		ResourceType: "domain",
		ResourceID:   id.String(),
		Severity:     audit.SeverityWarning,
	})
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *DomainsHandler) Verify(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	d, err := h.svc.Verify(c.Request.Context(), storeID, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	h.audit.Emit(c, audit.Event{
		Action:       "domain.verified",
		ResourceType: "domain",
		ResourceID:   id.String(),
		Metadata: map[string]any{
			"domain": d.Domain,
			"status": string(d.Status),
		},
	})
	c.JSON(http.StatusOK, gin.H{"data": h.toDomainResponse(*d)})
}

// RefreshStatus handles POST /admin/.../domains/:id/refresh-status.
// Merchants click "Refresh SSL" after DNS changes / cert issuance to
// sync the DB row with cert-manager's actual state.
func (h *DomainsHandler) RefreshStatus(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}
	d, err := h.svc.RefreshCertStatus(c.Request.Context(), storeID, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	h.audit.Emit(c, audit.Event{
		Action:       "domain.ssl_refreshed",
		ResourceType: "domain",
		ResourceID:   id.String(),
		Metadata: map[string]any{
			"ssl_status": string(d.SSLStatus),
		},
	})
	c.JSON(http.StatusOK, gin.H{"data": h.toDomainResponse(*d)})
}

// ResolveDomain handles GET /storefront/resolve-domain?domain=x — public,
// used by the storefront to map custom domains to store slugs.
func (h *DomainsHandler) ResolveDomain(c *gin.Context) {
	domainName := c.Query("domain")
	if domainName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain query param is required"})
		return
	}

	d, err := h.svc.ResolveByDomain(c.Request.Context(), domainName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	// Look up the actual store slug — not the cname_target, which is
	// now a generic edge hostname shared by all custom domains.
	store, err := h.storeRepo.GetByIDForTenant(c.Request.Context(), d.StoreID.String(), d.TenantID.String())
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"store_id": d.StoreID.String(),
		"slug":     store.Slug,
	})
}
