package breakglass

import (
	"crypto/hmac"
	"crypto/sha256"
	"net"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// Event types emitted by this package. Pinned constants so observability
// dashboards in P17 reference exactly one name for each flow.
const (
	EventTypeLoginSuccess    = "break_glass.login.success"
	EventTypeLoginFailed     = "break_glass.login.failed"
	EventTypeRotationSuccess = "break_glass.rotation.success"
	EventTypeRotationFailed  = "break_glass.rotation.failed"
	EventTypeBootstrapFailed = "break_glass.bootstrap.failed"
)

// HMACKey is the shared secret used to fingerprint client IPs before
// they hit the audit row or the lockout table. Raw IPs must NEVER be
// persisted — this wrapper makes it hard to accidentally do so.
type HMACKey []byte

// HMACIPHash returns a stable 32-byte HMAC-SHA256 of ip under key.
// P8 ships a shared helper with the same shape; once that lands, the
// caller swaps the import. Until then we keep this local so audit
// events carry the right shape from day one.
func HMACIPHash(key HMACKey, ip string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ip))
	return mac.Sum(nil)
}

// AuditEmitter wraps the shared audit.Emitter with break-glass
// conventions: severity=critical on every event, ip_hash (never raw
// IP) in metadata, actor derived from tenant_id.
type AuditEmitter struct {
	e   *audit.Emitter
	key HMACKey
}

// NewAuditEmitter constructs an AuditEmitter. `key` is the HMAC key
// used to fingerprint IPs.
func NewAuditEmitter(e *audit.Emitter, key HMACKey) *AuditEmitter {
	return &AuditEmitter{e: e, key: key}
}

// EmitLogin records a break-glass login attempt with severity=critical.
// `success` true/false flips the event type; `reason` (optional)
// distinguishes failure modes for forensics without leaking to the
// client response.
func (a *AuditEmitter) EmitLogin(c *gin.Context, tenantID uuid.UUID, success bool, sessionID string, reason string) {
	evtType := EventTypeLoginSuccess
	if !success {
		evtType = EventTypeLoginFailed
	}

	var ua string
	if c != nil && c.Request != nil {
		ua = c.Request.UserAgent()
	}
	md := audit.Metadata{
		"tenant_id":  tenantID.String(),
		"success":    success,
		"ip_hash":    HMACIPHash(a.key, ClientIPFromRequest(c)),
		"user_agent": ua,
	}
	if sessionID != "" {
		md["session_id"] = sessionID
	}
	if reason != "" {
		md["reason"] = reason
	}

	a.e.Emit(c, audit.Event{
		Action:         evtType,
		ResourceType:   "break_glass_account",
		ResourceID:     tenantID.String(),
		Status:         statusOf(success),
		Severity:       audit.SeverityCritical,
		Metadata:       md,
		TenantID:       tenantID,
		ForceActorType: audit.ActorSystem,
	})
}

// EmitRotation records a rotation attempt (post-use or 90-day cron).
// Called from cron context, so no *gin.Context is available — pass nil.
func (a *AuditEmitter) EmitRotation(tenantID uuid.UUID, success bool, reason string) {
	evtType := EventTypeRotationSuccess
	if !success {
		evtType = EventTypeRotationFailed
	}
	md := audit.Metadata{
		"tenant_id": tenantID.String(),
		"success":   success,
	}
	if reason != "" {
		md["reason"] = reason
	}
	a.e.Emit(nil, audit.Event{
		Action:         evtType,
		ResourceType:   "break_glass_account",
		ResourceID:     tenantID.String(),
		Status:         statusOf(success),
		Severity:       audit.SeverityCritical,
		Metadata:       md,
		TenantID:       tenantID,
		ForceActorType: audit.ActorSystem,
	})
}

// EmitBootstrapFailed is the one warning-class event in this package:
// failure to provision a break-glass account at Pro+SSO signup must
// not silently pass, but must also not block the signup itself.
func (a *AuditEmitter) EmitBootstrapFailed(tenantID uuid.UUID, reason string) {
	a.e.Emit(nil, audit.Event{
		Action:         EventTypeBootstrapFailed,
		ResourceType:   "break_glass_account",
		ResourceID:     tenantID.String(),
		Status:         audit.StatusFailure,
		Severity:       audit.SeverityWarning,
		Metadata:       audit.Metadata{"tenant_id": tenantID.String(), "reason": reason},
		TenantID:       tenantID,
		ForceActorType: audit.ActorSystem,
	})
}

func statusOf(success bool) audit.Status {
	if success {
		return audit.StatusSuccess
	}
	return audit.StatusFailure
}

// ClientIPFromRequest resolves the best-effort client IP. Prefers
// X-Forwarded-For (Cloudflare populates this) over RemoteAddr; takes
// the leftmost entry since that's the originating client per RFC 7239.
func ClientIPFromRequest(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if xff := c.Request.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}
