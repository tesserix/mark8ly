// Package internalsvc holds HTTP endpoints intended for service-to-service
// calls inside the cluster. These routes are NOT exposed to end-users and
// are gated by a shared X-Internal-Auth secret so external callers can't
// reach them even if Istio's authorization policy is misconfigured.
package internalsvc

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

var (
	errStoreIDInvalid   = errors.New("store_id must be a uuid")
	errStoreIDRequired  = errors.New("store_id is required when no store resolver is configured")
	errNoStoreForTenant = errors.New("no active store found for tenant; pass store_id explicitly")
)

// AuditIngestHandler accepts cross-service audit events posted by other
// services in the cluster (auth-bff for login/logout, platform-api for
// staff invites, etc.). The endpoint is async — events are enqueued on
// the local audit emitter and the caller gets a 202 immediately.
type AuditIngestHandler struct {
	emitter *audit.Emitter
	stores  stores.Repository
	logger  *slog.Logger
}

// NewAuditIngestHandler constructs an AuditIngestHandler.
//
// emitter is required. storeRepo is used to resolve a default store_id
// when the caller (auth-bff at login time) doesn't know which store to
// scope the event to — see resolveStoreID below. If nil, callers MUST
// always send store_id explicitly.
func NewAuditIngestHandler(emitter *audit.Emitter, storeRepo stores.Repository, logger *slog.Logger) *AuditIngestHandler {
	return &AuditIngestHandler{emitter: emitter, stores: storeRepo, logger: logger}
}

// auditEventRequest is the wire body for POST /internal/audit-events.
// Every field is sent as a string to keep cross-language clients trivial
// (auth-bff and platform-api are Go today but the contract should stay
// language-neutral). UUIDs are parsed inside the handler.
//
// store_id is OPTIONAL: when omitted, the handler resolves the tenant's
// first active store and scopes the event there. Login events out of
// auth-bff use this so the caller doesn't need to know about stores.
type auditEventRequest struct {
	TenantID     string         `json:"tenant_id"     binding:"required,uuid"`
	StoreID      string         `json:"store_id"`
	Action       string         `json:"action"        binding:"required,max=64"`
	ResourceType string         `json:"resource_type" binding:"required,max=32"`
	ResourceID   string         `json:"resource_id"`
	Status       string         `json:"status"`        // success | failure (default success)
	Severity     string         `json:"severity"`      // info | warning | critical (default info)
	ActorType    string         `json:"actor_type"`    // user | system | api (default system)
	ActorUserID  string         `json:"actor_user_id"` // optional uuid
	ActorEmail   string         `json:"actor_email"`
	IPAddress    string         `json:"ip_address"`
	UserAgent    string         `json:"user_agent"`
	Metadata     map[string]any `json:"metadata"`
}

// Ingest handles POST /internal/audit-events.
//
// The request gin context already carries the actor's IP/UA from the
// service-to-service hop (auth-bff calling marketplace-api), which is NOT
// what we want to record — we want the END USER's IP/UA. So we override
// the emitter's context-derived fields by stuffing the body's IP/UA back
// onto the gin context as the canonical client-IP and request UA before
// calling Emit.
func (h *AuditIngestHandler) Ingest(c *gin.Context) {
	var req auditEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": err.Error(),
		})
		return
	}

	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": "tenant_id must be a uuid"})
		return
	}
	storeID, err := h.resolveStoreID(c, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
		return
	}

	// The caller's IP and user-agent live in the body (the upstream
	// service has them; we don't, the gin request is intra-cluster).
	// Stuff them onto the gin context the same way HeaderTrustAuth does
	// for user identity, so audit.Emitter.buildEntry picks them up.
	if req.ActorEmail != "" {
		c.Set("user_email", req.ActorEmail)
	}
	if req.ActorUserID != "" {
		c.Set("user_id", req.ActorUserID)
	}
	// Override the request's user-agent header so the emitter records
	// the END user's UA, not auth-bff's outbound HTTP client UA.
	if req.UserAgent != "" {
		c.Request.Header.Set("User-Agent", req.UserAgent)
	}
	// IP override: gin.ClientIP reads X-Forwarded-For / X-Real-IP first.
	// Setting X-Real-IP makes the emitter pick up the body's IP.
	if req.IPAddress != "" {
		c.Request.Header.Set("X-Real-IP", req.IPAddress)
	}

	actorType := audit.ActorType(req.ActorType)
	if actorType == "" {
		actorType = audit.ActorSystem
	}

	h.emitter.Emit(c, audit.Event{
		TenantID:       tenantID,
		StoreID:        storeID,
		ForceActorType: actorType,
		Action:         req.Action,
		ResourceType:   req.ResourceType,
		ResourceID:     req.ResourceID,
		Status:         audit.Status(req.Status),
		Severity:       audit.Severity(req.Severity),
		Metadata:       req.Metadata,
	})

	c.JSON(http.StatusAccepted, gin.H{"accepted": true})
}

// resolveStoreID returns the store_id the event should be scoped to.
// Explicit body wins. When absent, falls back to the tenant's first
// active store via the local stores projection — login events out of
// auth-bff land there. Returns an error only when neither is available.
func (h *AuditIngestHandler) resolveStoreID(c *gin.Context, req auditEventRequest) (uuid.UUID, error) {
	if req.StoreID != "" {
		sid, err := uuid.Parse(req.StoreID)
		if err != nil {
			return uuid.Nil, errStoreIDInvalid
		}
		return sid, nil
	}
	if h.stores == nil {
		return uuid.Nil, errStoreIDRequired
	}
	rows, err := h.stores.ListForTenant(c.Request.Context(), req.TenantID)
	if err != nil || len(rows) == 0 {
		return uuid.Nil, errNoStoreForTenant
	}
	sid, err := uuid.Parse(rows[0].ID)
	if err != nil {
		return uuid.Nil, errNoStoreForTenant
	}
	return sid, nil
}

// Register mounts the audit ingest endpoint onto the supplied /internal
// route group (matching the existing vendorHandler/pushWebhook
// convention in cmd/marketplace-api/main.go).
//
// Other services in the cluster POST here as fire-and-forget audit
// events. The X-Internal-Auth gate uses the same shared secret as the
// admin trust headers — empty secret in dev makes the middleware
// permissive.
func (h *AuditIngestHandler) Register(group *gin.RouterGroup, internalSecret string) {
	group.POST("/audit-events", RequireInternalAuth(internalSecret), h.Ingest)
}

// RequireInternalAuth returns middleware that rejects requests missing
// the shared internal-auth header. Exported so other internal handlers
// can reuse it.
func RequireInternalAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.Next()
			return
		}
		if !constantTimeEqual(c.GetHeader("X-Internal-Auth"), secret) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "internal auth required",
			})
			return
		}
		c.Next()
	}
}

func constantTimeEqual(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	gotSum := sha256.Sum256([]byte(got))
	wantSum := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotSum[:], wantSum[:]) == 1
}
