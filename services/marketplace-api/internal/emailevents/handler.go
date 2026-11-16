package emailevents

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/emaillog"
)

// Signature headers. Named by the provider's webhook layer, not by Resend
// itself, which is why they are not `resend-*`.
const (
	HeaderID        = "svix-id"
	HeaderTimestamp = "svix-timestamp"
	HeaderSignature = "svix-signature"
)

// maxBody caps what an unauthenticated caller can make us read. The signature
// is computed over the body, so it cannot be checked until the body is read —
// which means this limit is the only thing bounding that work.
const maxBody = 1 << 20

// Handler receives provider delivery events (#348B).
type Handler struct {
	applier *Applier
	secret  string
	logger  *slog.Logger
	now     func() time.Time
}

// NewHandler constructs the handler. An empty secret leaves the endpoint
// mounted but inert — every request answers 503. That is deliberate and
// visible, unlike an unmounted route that 404s and looks like a wrong URL
// (#280 shipped exactly that failure).
func NewHandler(applier *Applier, secret string, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{applier: applier, secret: strings.TrimSpace(secret),
		logger: logger, now: time.Now}
}

// Configured reports whether a signing secret is present.
func (h *Handler) Configured() bool { return h.secret != "" }

func (h *Handler) Register(g *gin.RouterGroup) {
	g.POST("/webhooks/resend", h.receive)
}

// resendEvent is the subset of the provider payload this needs.
//
// Parsed leniently on purpose: the field layout is the one thing here not
// pinned by a specification, so an unexpected shape must degrade to "recorded
// and ignored" rather than to a rejection the provider retries forever.
type resendEvent struct {
	Type      string          `json:"type"`
	CreatedAt string          `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

func (h *Handler) receive(c *gin.Context) {
	if !h.Configured() {
		// 503, not 401: nothing is wrong with the caller. Saying so plainly
		// is what makes a missing secret diagnosable instead of looking like
		// a signature bug.
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "not_configured",
			"message": "no webhook signing secret is configured",
		})
		return
	}

	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBody))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unreadable_body"})
		return
	}

	// Verified over the RAW bytes, before any parsing. Re-marshalling parsed
	// JSON changes key order and whitespace, and the signature is over the
	// exact bytes as sent.
	if err := Verify(raw, c.GetHeader(HeaderID), c.GetHeader(HeaderTimestamp),
		c.GetHeader(HeaderSignature), h.secret, h.now()); err != nil {
		// One opaque answer for every rejection reason. Distinguishing them
		// tells an attacker which half of the check they passed; the detail
		// goes to our logs instead — the same discipline as
		// platformadmin.reject.
		h.logger.Warn("emailevents: rejected delivery", "reason", err.Error(),
			"event_id", c.GetHeader(HeaderID))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_signature"})
		return
	}

	var ev resendEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		// Signed but unparseable. Accepted, because a 4xx makes the provider
		// redeliver a payload that will never parse.
		h.logger.Warn("emailevents: signed payload did not parse",
			"event_id", c.GetHeader(HeaderID), "err", err)
		c.Status(http.StatusOK)
		return
	}

	sendID, ok := sendIDFrom(ev.Data)
	if !ok {
		// Expected for mail other services send (invites, sign-in codes).
		// The key names (never values: they hold addresses) show where a
		// send id would be if the provider moved it.
		h.logger.Info("emailevents: event carries no send id; ignoring",
			"type", ev.Type, "event_id", c.GetHeader(HeaderID),
			"data_keys", keysOf(ev.Data), "tag_keys", keysOf(tagField(ev.Data)))
		c.Status(http.StatusOK)
		return
	}

	if err := h.applier.Apply(c.Request.Context(), Event{
		EventID: c.GetHeader(HeaderID),
		SendID:  sendID,
		Type:    ev.Type,
		At:      parseEventTime(ev.CreatedAt, h.now()),
	}); err != nil {
		// The only retryable case: our database refused. A 5xx asks the
		// provider to redeliver, which is what we want here and nowhere else.
		h.logger.Error("emailevents: apply failed", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "apply_failed"})
		return
	}
	c.Status(http.StatusOK)
}

// sendIDFrom recovers our send id from the event payload.
//
// Piece A injects it as a custom arg; providers echo those back under
// different names, so several shapes are tried. This is the lenient part by
// design — a miss means "recorded and ignored", never a rejection.
func sendIDFrom(data json.RawMessage) (uuid.UUID, bool) {
	if len(data) == 0 {
		return uuid.Nil, false
	}
	var payload struct {
		Tags       json.RawMessage   `json:"tags"`
		CustomArgs map[string]string `json:"custom_args"`
		Headers    map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return uuid.Nil, false
	}
	for _, m := range []map[string]string{tagMap(payload.Tags), payload.CustomArgs, payload.Headers} {
		if raw, ok := m[emaillog.CustomArgSendID]; ok {
			if id, err := uuid.Parse(raw); err == nil {
				return id, true
			}
		}
	}
	return uuid.Nil, false
}

// keysOf lists an object's keys, or describes a non-object.
func keysOf(raw json.RawMessage) []string {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		if len(raw) > 0 && raw[0] == '[' {
			var list []struct{ Name string }
			if json.Unmarshal(raw, &list) == nil {
				out := []string{"(list)"}
				for _, t := range list {
					out = append(out, t.Name)
				}
				return out
			}
		}
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func tagField(data json.RawMessage) json.RawMessage {
	var p struct {
		Tags json.RawMessage `json:"tags"`
	}
	_ = json.Unmarshal(data, &p)
	return p.Tags
}

// tagMap reads tags in either shape Resend echoes them: an object, or a
// list of {name, value} (the shape we send them in). Anything else is empty.
func tagMap(raw json.RawMessage) map[string]string {
	var m map[string]string
	if json.Unmarshal(raw, &m) == nil {
		return m
	}
	var list []struct{ Name, Value string }
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	m = make(map[string]string, len(list))
	for _, t := range list {
		m[t.Name] = t.Value
	}
	return m
}

// parseEventTime falls back to now for an absent or unparseable timestamp.
// The event time is informational; losing it must not drop the event.
func parseEventTime(s string, fallback time.Time) time.Time {
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(s)); err == nil {
		return t
	}
	return fallback
}
