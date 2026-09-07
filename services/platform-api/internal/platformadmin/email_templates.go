package platformadmin

// email_templates.go — the auth/onboarding half of the transactional
// email template registry on the platform admin contract surface
// (mark8ly#720 Task 5).
//
// marketplace-api already serves its half of this registry
// (services/marketplace-api/internal/handlers/platformadmin/
// email_templates.go, tesserix-home#588) — orderdoc_*, giftcard_delivery
// and the twelve billing keys. This file serves the other half: welcome,
// email_verification, invitation, password_reset, login_otp,
// new_device_login — migration 0013's table, which marketplace-api's
// doc comment names as the gap this closes. Routes, request/response
// shapes and the error envelope are copied from that file field for
// field so the console can call both services' endpoints identically; see
// this file's doc comments only where platform-api's actual
// infrastructure differs from what that file could assume.
//
// mark8ly#730 fixed a bug where this endpoint's PUT ignored its
// Idempotency-Key header entirely. This file is written directly against
// the FIXED behaviour (reserve-then-complete via internal/idempotency,
// requiring the header, replaying a stored response) — there was never a
// version of this handler here with the bug to inherit.

import (
	"context"
	"encoding/json"
	"errors"
	htmltpl "html/template"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	texttpl "text/template"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/emailtemplates"
	"github.com/mark8ly/platform-api/internal/idempotency"
	"github.com/mark8ly/platformauth"
)

// EmailTemplateStore is the subset of *emailtemplates.Store this handler
// needs. Narrowed and declared locally, matching marketplace-api's
// pattern, so the handler is stubbable in tests without a database.
type EmailTemplateStore interface {
	List(ctx context.Context) ([]emailtemplates.Row, error)
	Get(ctx context.Context, key string) (emailtemplates.Row, bool, error)
	Upsert(ctx context.Context, in emailtemplates.UpsertInput) (emailtemplates.Row, error)
}

// EmailTemplateRegistry is the subset of *emailtemplates.Registry this
// handler needs: the fixed embedded-key list, the cache eviction (which
// reaches the real *notification.Loader — see registry.go's doc comment),
// and a render for the test send.
type EmailTemplateRegistry interface {
	RegisteredKeys() []string
	Fallback(key string) (emailtemplates.EmbeddedFallback, bool)
	Invalidate(key string)
	Render(ctx context.Context, key string, vars any) (emailtemplates.Rendered, error)
}

// Template row states, as seen by an operator. Identical vocabulary to
// marketplace-api's surface — see that file's doc comments for the full
// rationale, which is unchanged here.
const (
	stateUnauthored = "unauthored"
	stateDraft      = "draft"
	statePublished  = "published"
)

const (
	sendsFromRow      = "row"
	sendsFromEmbedded = "embedded"
	sendsFromNothing  = "nothing"
)

// EmailTemplatesHandler serves /admin/email-templates.
type EmailTemplatesHandler struct {
	store    EmailTemplateStore
	registry EmailTemplateRegistry
	sender   emailtemplates.TestSender
	writable bool
	db       *gorm.DB
	logger   *slog.Logger
}

// NewEmailTemplatesHandler constructs the handler.
//
// logger and sender may be nil — a nil sender leaves the test-send route
// mounted and answering 503 not_configured, matching how an unset
// PLATFORM_API_PLATFORM_ADMIN_SECRET leaves this whole surface mounted but
// inert. writable gates the PUT; see Register in routes.go for what that
// requires.
//
// db backs Idempotency-Key replay on the PUT. It is a separate parameter
// from writable even though production derives both from the same
// deps.DB, matching marketplace-api's BillingTrialExtendHandler /
// EmailTemplatesHandler pattern: the unit tests below exercise a writable
// handler with no database, where a nil db means the write happens but is
// not replayable.
func NewEmailTemplatesHandler(
	store EmailTemplateStore,
	registry EmailTemplateRegistry,
	sender emailtemplates.TestSender,
	writable bool,
	db *gorm.DB,
	logger *slog.Logger,
) *EmailTemplatesHandler {
	return &EmailTemplatesHandler{
		store: store, registry: registry, sender: sender,
		writable: writable, db: db, logger: logger,
	}
}

// Register mounts the routes. The PUT mounts only when the handler was
// built writable — see routes.go.
func (h *EmailTemplatesHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/email-templates", h.List)
	g.GET("/admin/email-templates/:key", h.Get)
	g.POST("/admin/email-templates/:key/test-send", h.TestSend)
	if h.writable {
		g.PUT("/admin/email-templates/:key", h.Upsert)
	}
}

// emailTemplateRow is the pinned list shape — identical to
// marketplace-api's, field for field, so the console's list rendering
// needs no per-service branch.
type emailTemplateRow struct {
	Key                string  `json:"key"`
	State              string  `json:"state"`
	SendsFrom          string  `json:"sends_from"`
	HasEmbeddedDefault bool    `json:"has_embedded_default"`
	Subject            string  `json:"subject"`
	Version            *int    `json:"version,omitempty"`
	UpdatedAt          *string `json:"updated_at,omitempty"`
	UpdatedBy          *string `json:"updated_by,omitempty"`
}

// emailTemplateDetail is the single-key shape: the list row plus the
// bodies and the declared variable schema.
type emailTemplateDetail struct {
	emailTemplateRow
	HTMLBody  string                    `json:"html_body"`
	TextBody  string                    `json:"text_body"`
	Variables []emailtemplates.Variable `json:"variables"`
}

// List serves GET /admin/email-templates.
//
// No pagination envelope — matching marketplace-api's surface, for the
// same reason: platform-api's key set is CLOSED at six entries, all owned
// by internal/notification's embedded templates, and cannot grow at
// runtime.
func (h *EmailTemplatesHandler) List(c *gin.Context) {
	stored, err := h.store.List(c.Request.Context())
	if err != nil {
		h.logError("platform email templates list failed", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not read the template registry",
		})
		return
	}

	byKey := make(map[string]emailtemplates.Row, len(stored))
	keys := make([]string, 0, len(stored))
	for _, r := range stored {
		byKey[r.Key] = r
		keys = append(keys, r.Key)
	}
	for _, k := range h.registeredKeys() {
		if _, ok := byKey[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	rows := make([]emailTemplateRow, 0, len(keys))
	for _, k := range keys {
		row, ok := byKey[k]
		if !ok {
			rows = append(rows, h.unauthoredRow(k))
			continue
		}
		rows = append(rows, h.storedRow(row))
	}

	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// Get serves GET /admin/email-templates/:key.
func (h *EmailTemplatesHandler) Get(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_key", "message": "key is required", "field": "key",
		})
		return
	}

	row, found, err := h.store.Get(c.Request.Context(), key)
	if err != nil {
		h.logError("platform email template read failed", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not read the template",
		})
		return
	}
	if found {
		c.JSON(http.StatusOK, gin.H{"data": emailTemplateDetail{
			emailTemplateRow: h.storedRow(row),
			HTMLBody:         row.HTMLBody,
			TextBody:         row.TextBody,
			Variables:        variablesOrEmpty(row.Variables),
		}})
		return
	}

	fb, registered := h.fallback(key)
	if !registered {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "unknown_key",
			"message": "no template is stored or registered under this key",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": emailTemplateDetail{
		emailTemplateRow: h.unauthoredRow(key),
		HTMLBody:         fb.HTMLBody,
		TextBody:         fb.TextBody,
		Variables:        []emailtemplates.Variable{},
	}})
}

// upsertRequest is the PUT wire shape — snake_case, matching
// marketplace-api's surface.
type upsertRequest struct {
	Subject   string                    `json:"subject"`
	HTMLBody  string                    `json:"html_body"`
	TextBody  string                    `json:"text_body"`
	Variables []emailtemplates.Variable `json:"variables"`
	Status    string                    `json:"status"`
}

// Upsert serves PUT /admin/email-templates/:key.
func (h *EmailTemplatesHandler) Upsert(c *gin.Context) {
	// Checked FIRST, before the key or the body — matching
	// marketplace-api's ordering (mark8ly#730): the console's
	// federation.Client refuses to make a write without this header, so
	// every real caller already sends one.
	idemKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idemKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "idempotency_key_required",
			"message": "the Idempotency-Key header is required for this endpoint",
		})
		return
	}

	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_key", "message": "key is required", "field": "key",
		})
		return
	}

	var req upsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "request body could not be parsed",
		})
		return
	}

	status, ok := normaliseStatus(req.Status)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_status",
			"message": "status must be draft or published",
			"field":   "status",
			"allowed": []string{emailtemplates.StatusDraft, emailtemplates.StatusPublished},
		})
		return
	}

	// The key must already be known: either a row exists, or it is one of
	// the six embedded keys. Keys are owned by code (internal/notification's
	// embedded templates), not by the console, so creating a new one out of
	// thin air is out of scope — matching marketplace-api's rule.
	_, exists, err := h.store.Get(c.Request.Context(), key)
	if err != nil {
		h.logError("platform email template read failed", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not read the template",
		})
		return
	}
	if _, registered := h.fallback(key); !exists && !registered {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "unknown_key",
			"message": "no template is stored or registered under this key",
		})
		return
	}

	if problems := validateTemplateBody(req); len(problems) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "invalid_template",
			"message":  strings.Join(problems, "; "),
			"problems": problems,
		})
		return
	}

	operator := strings.TrimSpace(c.GetString(platformauth.CtxOperatorID))
	if operator == "" {
		// RequirePlatformAuth requires a signed operator on every write, so
		// this is unreachable through the mounted surface. Checked anyway
		// because it is the field the whole attribution rests on.
		h.logError("platform email template write without an operator id", errors.New("missing operator"))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not attribute the change",
		})
		return
	}

	// Namespaced so a key can never replay across template keys or across
	// endpoints — idempotency_keys.key is a bare primary key shared by the
	// whole service.
	scopedKey := "email_template_upsert:" + key + ":" + idemKey

	// Reserved AFTER every validation above and immediately before the
	// write, matching marketplace-api's ordering: validation is
	// deterministic, so two callers with the same key either both pass it
	// or both fail it, and only then race.
	if h.db != nil {
		claimed, err := idempotency.Reserve(c.Request.Context(), h.db, scopedKey,
			estateWideTenant, time.Now().UTC(), idempotency.DefaultTTL)
		if err != nil {
			// Fail CLOSED — a caller that cannot be told whether this key
			// was already used must not be let through to a second UPSERT.
			h.logError("platform email template idempotency reserve failed", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "unavailable", "message": "could not verify idempotency key",
			})
			return
		}
		if !claimed {
			stored, ok, err := idempotency.Lookup(c.Request.Context(), h.db, scopedKey)
			if err != nil {
				h.logError("platform email template idempotency lookup failed", err)
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": "unavailable", "message": "could not verify idempotency key",
				})
				return
			}
			if ok {
				// A replay: the stored bytes verbatim, no second UPSERT, no
				// version bump, no extra revision row.
				c.Data(http.StatusOK, "application/json; charset=utf-8", stored)
				return
			}
			c.JSON(http.StatusConflict, gin.H{
				"error": "in_progress", "message": "a request with this Idempotency-Key is already in flight",
			})
			return
		}
	}

	saved, err := h.store.Upsert(c.Request.Context(), emailtemplates.UpsertInput{
		Key:        key,
		Subject:    req.Subject,
		HTMLBody:   req.HTMLBody,
		TextBody:   req.TextBody,
		Variables:  variablesOrEmpty(req.Variables),
		Status:     status,
		UpdatedBy:  operator,
		Capability: strings.TrimSpace(c.GetString(platformauth.CtxCapability)),
	})
	if err != nil {
		h.releaseReservation(c, scopedKey)
		h.logError("platform email template write failed", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not save the template",
		})
		return
	}

	// Cache eviction, in-process, on the SAME *notification.Loader the
	// send path reads — see emailtemplates.Registry's doc comment. No HTTP
	// round trip to another process is needed; there never was one for
	// platform-api's own templates the way tesserix-home's old cross-DB
	// path needed for marketplace-api's.
	if h.registry != nil {
		h.registry.Invalidate(key)
	}

	body, err := json.Marshal(gin.H{"data": emailTemplateDetail{
		emailTemplateRow: h.storedRow(saved),
		HTMLBody:         saved.HTMLBody,
		TextBody:         saved.TextBody,
		Variables:        variablesOrEmpty(saved.Variables),
	}})
	if err != nil {
		// The write SUCCEEDED — this is not a failed request, but the
		// caller cannot be given a body and the key must not be left
		// claimed with an empty response.
		h.releaseReservation(c, scopedKey)
		h.logError("platform email template response encode failed", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "the template was saved but the response could not be encoded",
		})
		return
	}

	if h.db != nil {
		if err := idempotency.Complete(c.Request.Context(), h.db, scopedKey, body); err != nil {
			h.logError("platform email template idempotency complete failed", err)
		}
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// estateWideTenant is the tenant_id recorded against an email-template
// reservation. idempotency_keys.tenant_id is NOT NULL (migration 0017)
// and this registry is estate-wide, so the nil uuid is the honest value —
// matching marketplace-api's identical constant.
var estateWideTenant = uuid.Nil.String()

func (h *EmailTemplatesHandler) releaseReservation(c *gin.Context, scopedKey string) {
	if h.db == nil {
		return
	}
	if err := idempotency.Release(c.Request.Context(), h.db, scopedKey); err != nil {
		h.logError("platform email template idempotency release failed", err)
	}
}

// testSendRequest is the test-send wire shape.
type testSendRequest struct {
	To   string         `json:"to"`
	Vars map[string]any `json:"vars"`
}

// TestSend serves POST /admin/email-templates/:key/test-send.
//
// It renders through registry.Render, which — via emailtemplates.Registry
// — reaches into the SAME *notification.Loader the real send path uses,
// so it exercises exactly what is live for the key.
func (h *EmailTemplatesHandler) TestSend(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_key", "message": "key is required", "field": "key",
		})
		return
	}

	var req testSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "request body could not be parsed",
		})
		return
	}
	if strings.TrimSpace(req.To) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_recipient", "message": "to is required", "field": "to",
		})
		return
	}

	if h.registry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "not_configured", "message": "the template registry is not wired",
		})
		return
	}

	vars := req.Vars
	if vars == nil {
		vars = map[string]any{}
	}
	rendered, err := h.registry.Render(c.Request.Context(), key, vars)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "render_failed", "message": err.Error(),
		})
		return
	}

	if h.sender == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "not_configured",
			"message": "no test sender is configured",
		})
		return
	}
	if err := h.sender.SendTest(c.Request.Context(), req.To, rendered); err != nil {
		h.logError("platform email template test send failed", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "send_failed", "message": "the provider rejected the test send",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"key": key, "to": req.To, "sent": true,
	}})
}

// storedRow projects a stored row FIELD BY FIELD, so a column added to
// email_templates tomorrow cannot leak through this endpoint.
func (h *EmailTemplatesHandler) storedRow(r emailtemplates.Row) emailTemplateRow {
	_, registered := h.fallback(r.Key)

	state := statePublished
	sendsFrom := sendsFromRow
	if r.Status != emailtemplates.StatusPublished {
		state = stateDraft
		sendsFrom = sendsFromEmbedded
		if !registered {
			sendsFrom = sendsFromNothing
		}
	}

	version := r.Version
	updatedAt := r.UpdatedAt.UTC().Format(time.RFC3339)
	row := emailTemplateRow{
		Key:                r.Key,
		State:              state,
		SendsFrom:          sendsFrom,
		HasEmbeddedDefault: registered,
		Subject:            r.Subject,
		Version:            &version,
		UpdatedAt:          &updatedAt,
	}
	if r.UpdatedBy != "" {
		by := r.UpdatedBy
		row.UpdatedBy = &by
	}
	return row
}

func (h *EmailTemplatesHandler) unauthoredRow(key string) emailTemplateRow {
	fb, registered := h.fallback(key)
	sendsFrom := sendsFromEmbedded
	if !registered {
		sendsFrom = sendsFromNothing
	}
	return emailTemplateRow{
		Key:                key,
		State:              stateUnauthored,
		SendsFrom:          sendsFrom,
		HasEmbeddedDefault: registered,
		Subject:            fb.Subject,
	}
}

func (h *EmailTemplatesHandler) registeredKeys() []string {
	if h.registry == nil {
		return nil
	}
	return h.registry.RegisteredKeys()
}

func (h *EmailTemplatesHandler) fallback(key string) (emailtemplates.EmbeddedFallback, bool) {
	if h.registry == nil {
		return emailtemplates.EmbeddedFallback{}, false
	}
	return h.registry.Fallback(key)
}

func (h *EmailTemplatesHandler) logError(msg string, err error) {
	if h.logger != nil {
		h.logger.Error(msg, "err", err)
	}
}

// normaliseStatus accepts an absent status as `published`. An
// unrecognised value is REJECTED rather than coerced.
func normaliseStatus(raw string) (string, bool) {
	switch strings.TrimSpace(raw) {
	case "":
		return emailtemplates.StatusPublished, true
	case emailtemplates.StatusPublished:
		return emailtemplates.StatusPublished, true
	case emailtemplates.StatusDraft:
		return emailtemplates.StatusDraft, true
	default:
		return "", false
	}
}

// validateTemplateBody checks all three template fields SERVER-SIDE,
// matching marketplace-api's rules exactly, including the brace-count
// check kept alongside the parse for the same reason: it produces the
// message an operator can act on.
func validateTemplateBody(req upsertRequest) []string {
	var problems []string

	fields := []struct {
		name  string
		value string
		html  bool
	}{
		{"subject", req.Subject, false},
		{"html_body", req.HTMLBody, true},
		{"text_body", req.TextBody, false},
	}

	for _, f := range fields {
		if strings.TrimSpace(f.value) == "" {
			problems = append(problems, f.name+": must not be empty")
			continue
		}
		if opens, closes := strings.Count(f.value, "{{"), strings.Count(f.value, "}}"); opens != closes {
			problems = append(problems,
				f.name+": mismatched template braces ("+
					strconv.Itoa(opens)+" {{, "+strconv.Itoa(closes)+" }})")
			continue
		}
		if err := parseTemplate(f.name, f.value, f.html); err != nil {
			problems = append(problems, f.name+": "+err.Error())
		}
	}

	for i, v := range req.Variables {
		if strings.TrimSpace(v.Name) == "" {
			problems = append(problems, "variables["+strconv.Itoa(i)+"]: name must not be empty")
		}
	}

	return problems
}

// parseTemplate compiles the field with the SAME engine the send path
// uses for it — html/template for the HTML body, text/template for the
// subject and the plain-text body.
func parseTemplate(name, body string, html bool) error {
	if html {
		_, err := htmltpl.New(name).Parse(body)
		return err
	}
	_, err := texttpl.New(name).Parse(body)
	return err
}

// variablesOrEmpty never returns nil.
func variablesOrEmpty(in []emailtemplates.Variable) []emailtemplates.Variable {
	if in == nil {
		return []emailtemplates.Variable{}
	}
	return in
}
