package platformadmin

// email_templates.go — the transactional email template registry on the
// platform admin contract surface (tesserix-home#588).
//
// This replaces the console's cross-DB path. tesserix-home authored these
// rows by connecting straight to mark8ly's database over the
// mark8ly_platform_admin grant and then pinging
// POST /internal/templates/refresh to evict the send path's cache. The new
// console has no database credential and reaches every product the same
// way — HMAC-signed federation into this package — so the registry moves
// onto the same rails as /admin/notifications and /admin/email-sends.
//
// /internal/templates/* is left mounted and unchanged. Retiring it is
// separate work, once nothing calls it.
//
// Scope: this is marketplace-api's half of the registry only —
// orderdoc_*, giftcard_delivery and the twelve billing keys. The auth
// templates (welcome, email_verification, invitation, password_reset,
// login_otp, new_device_login) live in mark8ly's platform-api, which has
// no contract surface at all; federating it is mark8ly#720. A console
// listing this endpoint's keys must say so — a list that silently omits
// password_reset is worse than no list.

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

	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
	"github.com/mark8ly/marketplace-api/internal/idempotency"
)

// EmailTemplateStore is the subset of emailtemplates.Store this handler
// needs. Narrowed and declared locally for the same reason as
// NotificationLister and EmailSendLister — the handler is stubbable
// without a database.
type EmailTemplateStore interface {
	List(ctx context.Context) ([]emailtemplates.Row, error)
	Get(ctx context.Context, key string) (emailtemplates.Row, bool, error)
	Upsert(ctx context.Context, in emailtemplates.UpsertInput) (emailtemplates.Row, error)
}

// EmailTemplateRegistry is the subset of *emailtemplates.Loader this
// handler needs: the in-memory registry of embedded defaults, the cache
// eviction, and a render for the test send.
//
// Reading the registry is what makes a registered-but-unseeded key
// visible. Twelve billing keys are registered and deliberately never
// seeded (cmd/marketplace-api/main.go, the block after SeedFromEmbedded:
// seeding them would let the first boot win forever, because
// SeedFromEmbedded is ON CONFLICT DO NOTHING and Render prefers a
// published row). A list built from database rows alone therefore cannot
// see them at all — mark8ly#717.
type EmailTemplateRegistry interface {
	RegisteredKeys() []string
	Fallback(key string) (emailtemplates.EmbeddedFallback, bool)
	Invalidate(key string)
	Render(ctx context.Context, key string, vars any) (emailtemplates.Rendered, error)
}

// Template row states, as seen by an operator.
const (
	// stateUnauthored: the key is registered in the loader but has no
	// database row. The embedded default compiled into the binary is what
	// sends. Editing it creates the override.
	stateUnauthored = "unauthored"
	// stateDraft: a row exists but the send path ignores it — Loader.load
	// filters status = 'published' — so the embedded default still sends.
	// This is the least obvious state on the surface and the one a console
	// must spell out.
	stateDraft = "draft"
	// statePublished: the row is what sends.
	statePublished = "published"
)

// Where a send actually gets its copy from, given the state above and
// whether an embedded default is registered.
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
// mounted and answering 503 not_configured, matching how a missing
// MARKETPLACE_PLATFORM_ADMIN_SECRET leaves this whole surface mounted but
// inert rather than absent. writable gates the PUT; see Register.
//
// db backs Idempotency-Key replay on the PUT (#730). It is a separate
// parameter from writable even though production derives both from the same
// deps.DB, because the unit tests exercise a writable handler with no
// database: a nil db there means the write happens but is not replayable,
// which is the same nil-db handling BillingTrialExtendHandler has.
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
// built writable — see routes.go for what that requires and why.
func (h *EmailTemplatesHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/email-templates", h.List)
	g.GET("/admin/email-templates/:key", h.Get)
	g.POST("/admin/email-templates/:key/test-send", h.TestSend)
	if h.writable {
		g.PUT("/admin/email-templates/:key", h.Upsert)
	}
}

// emailTemplateRow is the pinned list shape.
//
// `state` and `sends_from` are two different questions and both are
// answered, because neither implies the other on its own: a draft row and
// an absent row are different things to an operator (one is work in
// progress, one has never been touched) and yet both send the embedded
// default. Collapsing them into one field is how a console ends up
// showing a saved draft as though it were live.
//
// `subject` is present because it is the operator's only handle on which
// template this is — it is the RAW template source, not an interpolated
// line, so unlike the subject deliberately withheld from /admin/email-sends
// it carries no customer detail. The bodies are not here; they are on the
// single-key read, because a list of twenty full HTML bodies is a payload,
// not a list.
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
//
// For an unauthored key the bodies are the EMBEDDED default, not empty
// strings. That is what is sending right now, so it is what an editor must
// open with; handing back blanks would invite an operator to publish a row
// that silently replaced working copy with nothing.
type emailTemplateDetail struct {
	emailTemplateRow
	HTMLBody  string                    `json:"html_body"`
	TextBody  string                    `json:"text_body"`
	Variables []emailtemplates.Variable `json:"variables"`
}

// List serves GET /admin/email-templates.
//
// The response has no pagination envelope, unlike every other list on this
// surface. The key set is CLOSED and owned by code — a key exists because a
// Go call site renders it — so it is a few dozen entries that cannot grow
// at runtime. A pagination block over a fixed set would be furniture, and
// a console would build paging controls for a page that can never have a
// second one.
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

	// Allocate before appending: a nil slice marshals to null, which
	// defeats a caller's `?? []` exactly when there is nothing to show.
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
		// Neither a row nor a call site. Reporting this as an empty
		// template would invite an operator to author copy that nothing
		// would ever send.
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
		// An embedded default declares no variable schema — the registry
		// holds three template strings and nothing else. Empty is the
		// honest answer; a console must not present it as "this template
		// takes no variables".
		Variables: []emailtemplates.Variable{},
	}})
}

// upsertRequest is the PUT wire shape. snake_case throughout, matching
// every other body on this surface — tesserix-home's cross-DB route used
// camelCase (htmlBody/textBody) and that spelling does not survive the
// move.
type upsertRequest struct {
	Subject   string                    `json:"subject"`
	HTMLBody  string                    `json:"html_body"`
	TextBody  string                    `json:"text_body"`
	Variables []emailtemplates.Variable `json:"variables"`
	Status    string                    `json:"status"`
}

// Upsert serves PUT /admin/email-templates/:key.
//
// It keeps the version bump and the updated_by stamp of the cross-DB
// UPSERT it replaces (apps/web/lib/db/email-templates.ts), so moving the
// console onto this surface does not change what lands in the row. What it
// does NOT keep is the attribution source: updated_by comes from the
// SIGNED operator id on the request, never from the body.
func (h *EmailTemplatesHandler) Upsert(c *gin.Context) {
	// Checked FIRST, before the key or the body. platform-api's
	// federation.Client refuses to make a write without this header
	// (client.go:203), so every real caller already sends one; a request
	// that arrives without it is malformed, and saying so plainly beats a
	// validation error that sends the caller fixing the wrong thing.
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

	// The key must already be known: either a row exists, or a Go call site
	// registered a fallback for it. tesserix-home#588 rules creating a key
	// out of scope for the same reason — keys are owned by code, and a
	// console-created key with no call site sends nothing while looking
	// exactly like copy that does.
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

	operator := strings.TrimSpace(c.GetString(CtxOperatorID))
	if operator == "" {
		// RequirePlatformAuth requires a signed operator on every write, so
		// this is unreachable through the mounted surface. It is checked
		// anyway because it is the field the whole attribution rests on:
		// an empty updated_by would land a row nobody can be held to.
		h.logError("platform email template write without an operator id", errors.New("missing operator"))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not attribute the change",
		})
		return
	}

	// Namespaced so a key can never replay across template keys or across
	// endpoints: idempotency_keys.key is a bare primary key shared by the
	// whole service, and the caller's raw header is theirs to choose.
	scopedKey := "email_template_upsert:" + key + ":" + idemKey

	// Reserved AFTER every validation above and immediately before the
	// write, for the reason billing_trial_extend.go records: validation is
	// deterministic, so two callers with the same key either both pass it
	// or both fail it, and only then race. Reserving earlier would let a
	// malformed request leave the key claimed with an empty response for
	// the full TTL — turning a typo into a key that answers 409 for a day.
	if h.db != nil {
		claimed, err := idempotency.Reserve(c.Request.Context(), h.db, scopedKey,
			estateWideTenant, time.Now().UTC(), idempotency.DefaultTTL)
		if err != nil {
			// Fail CLOSED. A caller that cannot be told whether this key was
			// already used must not be let through to a second UPSERT, which
			// would bump the version again and append a second revision row
			// recording a change nobody made.
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
				// A replay: the stored bytes verbatim, with no second UPSERT,
				// no version bump and no extra revision row.
				c.Data(http.StatusOK, "application/json; charset=utf-8", stored)
				return
			}
			// Reserved but not completed — another caller, or another pod
			// handling the same retry, is still doing the work.
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
		Capability: strings.TrimSpace(c.GetString(CtxCapability)),
	})
	if err != nil {
		// Release the reservation so a corrected retry with the same key is
		// not answered 409 in_progress until the TTL expires.
		h.releaseReservation(c, scopedKey)
		h.logError("platform email template write failed", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not save the template",
		})
		return
	}

	// Cache eviction, in-process. The write and the send path's loader are
	// the SAME object in the same binary (cmd/marketplace-api/main.go builds
	// one Loader and hands it to both the mailers and this handler), so the
	// HTTP round trip to /internal/templates/refresh that tesserix-home had
	// to make after its cross-DB write is simply gone.
	//
	// It is not, and never was, cluster-wide: this evicts the replica that
	// served the request, and the old refresh ping evicted whichever single
	// replica the Service routed it to. Other replicas catch up within
	// emailtemplates.CacheTTL either way. Removing the ping loses nothing.
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
		// The write SUCCEEDED, so this is not a failed request — but the
		// caller cannot be given a body and the key must not be left
		// claimed with an empty response.
		h.releaseReservation(c, scopedKey)
		h.logError("platform email template response encode failed", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "the template was saved but the response could not be encoded",
		})
		return
	}

	// Completed AFTER the write, so a retry replays the same body. A failure
	// here is logged, not returned: the template is saved and the caller must
	// be told so. The cost is that a retry would write again — strictly better
	// than reporting a failure for a write that happened.
	if h.db != nil {
		if err := idempotency.Complete(c.Request.Context(), h.db, scopedKey, body); err != nil {
			h.logError("platform email template idempotency complete failed", err)
		}
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// estateWideTenant is the tenant_id recorded against an email-template
// reservation. idempotency_keys.tenant_id is NOT NULL (migration 000001) and
// this registry is estate-wide — a template key belongs to the product, not
// to a tenant — so the nil uuid is the honest value rather than borrowing an
// unrelated id to satisfy the constraint.
var estateWideTenant = uuid.Nil.String()

// releaseReservation drops a claim whose work did not complete. Best-effort:
// the caller is already being given an error, and the reservation expires on
// its own TTL regardless.
func (h *EmailTemplatesHandler) releaseReservation(c *gin.Context, scopedKey string) {
	if h.db == nil {
		return
	}
	if err := idempotency.Release(c.Request.Context(), h.db, scopedKey); err != nil {
		h.logError("platform email template idempotency release failed", err)
	}
}

// testSendRequest is the test-send wire shape. `to` is required: the
// console defaults it to the operator's own address, and a server-side
// default here would have to invent one.
type testSendRequest struct {
	To   string         `json:"to"`
	Vars map[string]any `json:"vars"`
}

// TestSend serves POST /admin/email-templates/:key/test-send.
//
// It renders through the SAME Loader the send path uses, so it exercises
// whatever is actually live for the key — a published row if there is one,
// the embedded default otherwise. A test send that rendered the submitted
// draft instead would answer a question nobody asked.
//
// The three failure codes are the ones /internal/templates/:key/test
// already returns and tesserix-home#588 pins each to its own operator
// sentence: 422 render_failed, 503 not_configured, 502 send_failed.
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
		// Includes the unknown-key case: Render returns ErrUnknownKey when
		// neither a published row nor an embedded default has the key.
		// Reported as a render failure rather than a 404 so the operator
		// gets the reason, which for the common case (a variable the
		// synthetic vars did not supply) is the whole answer.
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "render_failed", "message": err.Error(),
		})
		return
	}

	if h.sender == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "not_configured",
			"message": "no test sender is configured (SENDGRID_API_KEY is unset)",
		})
		return
	}
	if err := h.sender.SendTest(c.Request.Context(), req.To, rendered); err != nil {
		h.logError("platform email template test send failed", err)
		// The upstream provider's text is deliberately NOT echoed: it is a
		// third party's message and may carry the recipient back out. The
		// operator gets a stable sentence; the cause is in our logs.
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
		// A draft is invisible to the send path (Loader.load filters
		// status = 'published'), so what sends is the embedded default —
		// or nothing at all, if no call site registered one.
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

// unauthoredRow describes a registered key with no database row: the
// embedded default is what sends, and editing it creates the override.
// Version, updated_at and updated_by are OMITTED rather than zeroed — a
// version of 0 beside a template that is sending perfectly well reads as a
// broken row.
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

// normaliseStatus accepts an absent status as `published`, matching the
// UPSERT this replaces. An unrecognised value is REJECTED rather than
// coerced: silently treating "Published" or "live" as published would
// publish copy the operator did not mean to publish.
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

// validateTemplateBody checks all three template fields SERVER-SIDE.
//
// tesserix-home did this in its Next.js route (validateTemplateText) and
// commented that "real template-syntax validation happens on mark8ly when
// it tries to render". That is now here, so it is no longer deferred to a
// send: an unparseable published row makes Loader.Render return an error
// rather than fall back, which turns a typo into a stopped email rather
// than a stale one.
//
// The brace count is kept ALONGSIDE the parse, not replaced by it. It is
// the check that produces the message an operator can act on — "3 {{, 2
// }}" points at the typo, where text/template's own error for the same
// input is about an unexpected EOF.
//
// `subject` is itself a Go template — orderdoc interpolates the order
// number into the subject line — so it is validated exactly like a body.
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

// parseTemplate compiles the field with the SAME engine the send path uses
// for it — html/template for the HTML body, text/template for the subject
// and the plain-text body. Parsing with the wrong one would accept input
// the real render then rejects.
func parseTemplate(name, body string, html bool) error {
	if html {
		_, err := htmltpl.New(name).Parse(body)
		return err
	}
	_, err := texttpl.New(name).Parse(body)
	return err
}

// variablesOrEmpty never returns nil: a nil slice marshals to null, and a
// console reading `variables.map(...)` crashes on exactly the templates
// that declare none.
func variablesOrEmpty(in []emailtemplates.Variable) []emailtemplates.Variable {
	if in == nil {
		return []emailtemplates.Variable{}
	}
	return in
}
