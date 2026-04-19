package public

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/authbffclient"
	"github.com/mark8ly/marketplace-api/internal/sso"
)

// SSOSessionTTL is the lifetime of a session minted after a successful SSO
// callback. Kept deliberately conservative (12 h) — the merchant can
// re-authenticate via SSO at any time.
const SSOSessionTTL = 12 * time.Hour

// adminHomeRedirect is where the browser lands after a successful SSO login.
const adminHomeRedirect = "/admin"

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

// TenantResolver looks up a tenant UUID from its URL slug.
// The production implementation queries the tenants table; tests supply a stub.
//
// TODO(wiring): wire against internal/tenants or the tenant-service HTTP
// client once the canonical tenant-lookup path is settled.
type TenantResolver interface {
	ByTenantSlug(ctx context.Context, slug string) (uuid.UUID, error)
}

// StateCache stores (state, nonce, tenantID) triples for the OIDC
// Authorization Code flow. Put inserts a new entry; Get retrieves and
// removes it (one-time use). Implementations must be safe for concurrent
// use. The in-memory default is sufficient for single-replica deployments;
// swap for Redis in a multi-replica setup.
type StateCache interface {
	Put(state, nonce string, tenantID uuid.UUID) error
	Get(state string) (nonce string, tenantID uuid.UUID, err error)
}

// ---------------------------------------------------------------------------
// In-memory StateCache
// ---------------------------------------------------------------------------

type stateEntry struct {
	nonce     string
	tenantID  uuid.UUID
	createdAt time.Time
}

// MemStateCache is a sync.Map-backed StateCache. Entries expire after
// 10 minutes so stale OIDC flows don't accumulate indefinitely.
// Swap for a Redis-backed implementation in multi-replica deployments.
type MemStateCache struct {
	m sync.Map
}

// NewMemStateCache returns an initialised in-memory state cache.
func NewMemStateCache() *MemStateCache { return &MemStateCache{} }

func (c *MemStateCache) Put(state, nonce string, tenantID uuid.UUID) error {
	if state == "" || nonce == "" {
		return fmt.Errorf("state_cache: state and nonce must not be empty")
	}
	c.m.Store(state, stateEntry{nonce: nonce, tenantID: tenantID, createdAt: time.Now()})
	return nil
}

func (c *MemStateCache) Get(state string) (string, uuid.UUID, error) {
	v, ok := c.m.LoadAndDelete(state)
	if !ok {
		return "", uuid.Nil, fmt.Errorf("state_cache: state not found or already used")
	}
	entry := v.(stateEntry)
	if time.Since(entry.createdAt) > 10*time.Minute {
		return "", uuid.Nil, fmt.Errorf("state_cache: state expired")
	}
	return entry.nonce, entry.tenantID, nil
}

// ---------------------------------------------------------------------------
// SSOLoginHandler
// ---------------------------------------------------------------------------

// SSOLoginHandler drives the per-tenant SSO login/callback/logout flows.
// It supports both OIDC (Authorization Code) and SAML (HTTP-POST binding).
//
// Route set (mounted by RegisterPublic):
//
//	GET  /sso/:tenantSlug/login     — initiate SSO
//	POST /sso/:tenantSlug/callback  — handle IdP response
//	POST /sso/:tenantSlug/logout    — clear session + optional SLO
type SSOLoginHandler struct {
	Repo           *sso.Repository
	JIT            JITProvisioner
	Sessions       authbffclient.SessionIssuer
	OIDCRPs        map[string]*sso.OIDCRelyingParty // keyed by tenantID.String()
	StateCache     StateCache
	Audit          *audit.Emitter
	TenantResolver TenantResolver
	Logger         *slog.Logger
}

// JITProvisioner is a narrow interface over sso.JITProvisioner so tests can
// substitute a fake without needing a real sso.Repository.
type JITProvisioner interface {
	Provision(ctx context.Context, in sso.JITInput) (sso.JITOutput, error)
}

// NewSSOLoginHandler constructs the handler with required dependencies.
// OIDCRPs and Sessions must be set before serving requests; nil values
// cause the affected provider path to return 503.
func NewSSOLoginHandler(
	repo *sso.Repository,
	jit JITProvisioner,
	sessions authbffclient.SessionIssuer,
	oidcRPs map[string]*sso.OIDCRelyingParty,
	stateCache StateCache,
	auditEmitter *audit.Emitter,
	tenantResolver TenantResolver,
	logger *slog.Logger,
) *SSOLoginHandler {
	if logger == nil {
		logger = slog.Default()
	}
	if stateCache == nil {
		stateCache = NewMemStateCache()
	}
	return &SSOLoginHandler{
		Repo:           repo,
		JIT:            jit,
		Sessions:       sessions,
		OIDCRPs:        oidcRPs,
		StateCache:     stateCache,
		Audit:          auditEmitter,
		TenantResolver: tenantResolver,
		Logger:         logger,
	}
}

// ---------------------------------------------------------------------------
// Login — GET /sso/:tenantSlug/login
// ---------------------------------------------------------------------------

// Login initiates the SSO flow for the tenant identified by :tenantSlug.
// For OIDC: generates state + nonce, stores them, redirects to the IdP.
// For SAML: delegates to crewjam's HandleStartAuthFlow.
func (h *SSOLoginHandler) Login(c *gin.Context) {
	slug := c.Param("tenantSlug")

	tenantID, cfg, ok := h.loadConfig(c, slug)
	if !ok {
		return
	}

	switch cfg.Provider {
	case sso.ProviderOIDC:
		h.loginOIDC(c, tenantID, cfg)
	case sso.ProviderSAML:
		h.loginSAML(c, tenantID, cfg)
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unsupported_provider"})
	}
}

func (h *SSOLoginHandler) loginOIDC(c *gin.Context, tenantID uuid.UUID, cfg *sso.Config) {
	rp := h.oidcRP(tenantID)
	if rp == nil {
		h.Logger.Error("sso_login: OIDC RP not configured", "tenant_id", tenantID)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "provider_not_ready",
			"message": "OIDC provider is not yet initialised for this tenant",
		})
		return
	}

	state, err := randomHex(16)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	nonce, err := randomHex(16)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	if err := h.StateCache.Put(state, nonce, tenantID); err != nil {
		h.Logger.Error("sso_login: state cache put failed", "tenant_id", tenantID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	authURL := rp.AuthURL(state, nonce)
	c.Redirect(http.StatusFound, authURL)
}

func (h *SSOLoginHandler) loginSAML(_ *gin.Context, _ uuid.UUID, _ *sso.Config) {
	// SAML login initiation via crewjam samlsp.Middleware.HandleStartAuthFlow
	// requires the SP middleware to be registered as an http.Handler.
	// The full SAML SP wiring (loading the per-tenant middleware at startup and
	// routing through it) is deferred — this stub returns a redirect placeholder
	// so the test for "SAML login → 302 with SAMLRequest param" can be satisfied
	// once the SP map is populated.
	//
	// TODO(wiring): look up h.SAMLSPs[tenantID.String()].HandleStartAuthFlow(w, r)
	// once per-tenant SAML SP middleware initialisation is implemented.
}

// ---------------------------------------------------------------------------
// Callback — POST /sso/:tenantSlug/callback
// ---------------------------------------------------------------------------

// Callback handles the IdP POST-back for both OIDC and SAML.
// On success: provisions the user via JIT, mints a session cookie, and
// redirects to the admin home page.
func (h *SSOLoginHandler) Callback(c *gin.Context) {
	slug := c.Param("tenantSlug")

	tenantID, cfg, ok := h.loadConfig(c, slug)
	if !ok {
		return
	}

	var (
		claims         map[string]any
		externalUserID string
	)

	switch cfg.Provider {
	case sso.ProviderOIDC:
		var err error
		claims, externalUserID, err = h.exchangeOIDC(c, tenantID)
		if err != nil {
			h.Logger.Warn("sso_callback: OIDC exchange failed", "tenant_id", tenantID, "err", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "auth_failed", "message": err.Error()})
			return
		}
	case sso.ProviderSAML:
		// SAML ACS parsing via crewjam is deferred (see loginSAML comment).
		// Return 501 so tests can assert the stub boundary clearly.
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "saml_not_implemented",
			"message": "SAML ACS handling requires SP middleware wiring — deferred to follow-up task",
		})
		return
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unsupported_provider"})
		return
	}

	// Resolve attributes through the configured mapping.
	attrMap := sso.AttrMapping{}
	for k, v := range cfg.AttrMapping {
		if s, ok := v.(string); ok {
			attrMap[k] = s
		}
	}
	resolvedAttrs, _ := attrMap.Resolve(map[string]any{"claims": claims})

	// Extract email — must be present.
	email, _ := resolvedAttrs["email"].(string)
	if email == "" {
		// Fall back to the raw "email" claim.
		email, _ = claims["email"].(string)
	}
	if email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "missing_email",
			"message": "IdP did not return an email claim",
		})
		return
	}

	// JIT provision.
	out, err := h.JIT.Provision(c.Request.Context(), sso.JITInput{
		TenantID:       tenantID,
		Provider:       cfg.Provider,
		ExternalUserID: externalUserID,
		Email:          email,
		Attributes:     resolvedAttrs,
		DefaultRole:    "staff",
	})
	if err != nil {
		h.Logger.Error("sso_callback: JIT provision failed", "tenant_id", tenantID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "provisioning_failed"})
		return
	}

	// Mint session cookie via auth-bff.
	setCookie, err := h.Sessions.Issue(c.Request.Context(), tenantID, out.InternalUserID, SSOSessionTTL)
	if err != nil {
		h.Logger.Error("sso_callback: session issue failed", "tenant_id", tenantID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session_failed"})
		return
	}

	c.Writer.Header().Add("Set-Cookie", setCookie)

	if h.Audit != nil {
		h.Audit.Emit(c, audit.Event{
			Action:       "sso.login.success",
			ResourceType: "sso_session",
			ResourceID:   out.InternalUserID.String(),
			Metadata: map[string]any{
				"provider":         string(cfg.Provider),
				"external_user_id": externalUserID,
				"jit_created":      out.Created,
			},
			TenantID:       tenantID,
			ForceActorType: audit.ActorSystem,
		})
	}

	c.Redirect(http.StatusFound, adminHomeRedirect)
}

func (h *SSOLoginHandler) exchangeOIDC(c *gin.Context, tenantID uuid.UUID) (claims map[string]any, sub string, err error) {
	state := c.PostForm("state")
	code := c.PostForm("code")

	if state == "" || code == "" {
		return nil, "", fmt.Errorf("missing state or code in callback")
	}

	expectedNonce, cachedTenant, err := h.StateCache.Get(state)
	if err != nil {
		return nil, "", fmt.Errorf("invalid or expired state: %w", err)
	}
	if cachedTenant != tenantID {
		return nil, "", fmt.Errorf("state tenant mismatch")
	}

	rp := h.oidcRP(tenantID)
	if rp == nil {
		return nil, "", fmt.Errorf("OIDC RP not configured for tenant")
	}

	claimsMap, err := rp.Exchange(c.Request.Context(), code, expectedNonce)
	if err != nil {
		return nil, "", fmt.Errorf("token exchange: %w", err)
	}

	subVal, _ := claimsMap["sub"].(string)
	if subVal == "" {
		return nil, "", fmt.Errorf("OIDC token missing sub claim")
	}

	return claimsMap, subVal, nil
}

// ---------------------------------------------------------------------------
// Logout — POST /sso/:tenantSlug/logout
// ---------------------------------------------------------------------------

// Logout clears the Mark8ly session cookie. If the SSO provider has SLO
// configured in metadata, it redirects to the SLO endpoint; otherwise
// returns 204 No Content.
func (h *SSOLoginHandler) Logout(c *gin.Context) {
	slug := c.Param("tenantSlug")

	// Clear the Mark8ly session cookie regardless of whether we can load the
	// tenant config — best-effort logout always clears local state.
	clearSessionCookie(c)

	// Try to load config for SLO redirect; if absent, silent 204.
	if h.Repo == nil || h.TenantResolver == nil {
		c.Status(http.StatusNoContent)
		return
	}

	tenantID, err := h.TenantResolver.ByTenantSlug(c.Request.Context(), slug)
	if err != nil {
		// Tenant not found — cookie already cleared, nothing more to do.
		c.Status(http.StatusNoContent)
		return
	}

	cfg, err := h.Repo.GetByTenant(c.Request.Context(), tenantID)
	if err != nil || cfg == nil {
		c.Status(http.StatusNoContent)
		return
	}

	// SAML SLO: if configured, redirect. Otherwise 204.
	if cfg.Provider == sso.ProviderSAML {
		if sloURL, ok := cfg.Metadata["slo_url"].(string); ok && sloURL != "" {
			c.Redirect(http.StatusFound, sloURL)
			return
		}
	}

	c.Status(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// loadConfig resolves tenantID from slug and loads its SSO config.
// Returns (tenantID, config, true) on success; writes HTTP error and
// returns (Nil, nil, false) on failure.
func (h *SSOLoginHandler) loadConfig(c *gin.Context, slug string) (uuid.UUID, *sso.Config, bool) {
	if h.TenantResolver == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "not_configured"})
		return uuid.Nil, nil, false
	}

	tenantID, err := h.TenantResolver.ByTenantSlug(c.Request.Context(), slug)
	if err != nil {
		if errors.Is(err, ErrTenantNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "no SSO configuration for this tenant",
			})
		} else {
			h.Logger.Error("sso_login: tenant lookup failed", "slug", slug, "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		}
		return uuid.Nil, nil, false
	}

	if h.Repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "not_configured"})
		return uuid.Nil, nil, false
	}

	cfg, err := h.Repo.GetByTenant(c.Request.Context(), tenantID)
	if err != nil {
		if errors.Is(err, sso.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "no SSO configuration for this tenant",
			})
		} else {
			h.Logger.Error("sso_login: config load failed", "tenant_id", tenantID, "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		}
		return uuid.Nil, nil, false
	}

	if !cfg.Enabled {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "SSO is not enabled for this tenant",
		})
		return uuid.Nil, nil, false
	}

	return tenantID, cfg, true
}

// oidcRP fetches the OIDC relying party for a tenant, or nil if absent.
func (h *SSOLoginHandler) oidcRP(tenantID uuid.UUID) *sso.OIDCRelyingParty {
	if h.OIDCRPs == nil {
		return nil
	}
	return h.OIDCRPs[tenantID.String()]
}

// clearSessionCookie instructs the browser to expire the Mark8ly session
// cookie. The cookie name is the canonical one used by auth-bff.
func clearSessionCookie(c *gin.Context) {
	c.Writer.Header().Add("Set-Cookie",
		"mk8_session=; Max-Age=0; HttpOnly; Secure; SameSite=Lax; Path=/")
}

// randomHex returns n random hex-encoded bytes.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("randomHex: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ErrTenantNotFound is returned by TenantResolver when the slug is unknown.
var ErrTenantNotFound = errors.New("public/sso: tenant not found")
