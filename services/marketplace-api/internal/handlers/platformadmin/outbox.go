package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/outbox"
)

// OutboxLister is the subset of the outbox platform read this handler
// needs. Narrowed to one method for the same reason as NotificationLister
// in notifications.go and TicketLister in tickets.go.
type OutboxLister interface {
	ListPlatform(ctx context.Context, db *gorm.DB, f outbox.PlatformListFilter,
		asOf time.Time) (outbox.PlatformListResult, error)
}

// OutboxListerFunc adapts a plain function to OutboxLister, so
// outbox.ListPlatform — which is a package function, not a method — can be
// wired directly in main.go. Same pattern as TrialListerFunc.
type OutboxListerFunc func(ctx context.Context, db *gorm.DB, f outbox.PlatformListFilter,
	asOf time.Time) (outbox.PlatformListResult, error)

func (fn OutboxListerFunc) ListPlatform(ctx context.Context, db *gorm.DB,
	f outbox.PlatformListFilter, asOf time.Time) (outbox.PlatformListResult, error) {
	return fn(ctx, db, f, asOf)
}

// OutboxWriter is the subset of outbox's write operations this handler
// needs (#405): requeue (single + batch) and dead-letter. Narrowed to
// these three methods for the same reason as OutboxLister above.
//
// Implemented directly by outbox.WriterFuncs in production, wired via
// Deps.OutboxWriter — nil there leaves these three routes unmounted while
// List (#331) keeps working, matching the nil-safe pattern this surface
// uses everywhere else.
type OutboxWriter interface {
	RequeueOne(ctx context.Context, db *gorm.DB, id string) (outbox.RequeueResult, error)
	RequeueBatch(ctx context.Context, db *gorm.DB, ids []string) []outbox.RequeueOutcome
	DeadLetterOne(ctx context.Context, db *gorm.DB, id, reason string) (outbox.DeadLetterResult, error)
}

// outboxAuditFunc records a platform-operator action against the row's
// OWN tenant. Aliased to lifecycleAuditFunc — not a second independent
// named type — for the same reason trialExtendAuditFunc is: Go does not
// implicitly convert between two distinct named function types even when
// their underlying signatures match, and NewOperatorActionAuditFunc
// returns exactly this signature.
type outboxAuditFunc = lifecycleAuditFunc

// OutboxHandler serves GET /admin/outbox to the platform console — a
// cross-tenant read of outbox_events answering "what is stuck, what failed,
// and why" (#331).
//
// This surface exists because a row with published_at IS NULL and a non-null
// error, hours old, means a downstream integration is silently not
// happening, and nothing outside mark8ly could see it. Before #336 the
// `failed` state could not occur at all: nothing wrote outbox_events.error.
type OutboxHandler struct {
	db     *gorm.DB
	repo   OutboxLister
	writer OutboxWriter
	audit  outboxAuditFunc
	logger *slog.Logger
	now    func() time.Time
}

// NewOutboxHandler constructs the handler. logger may be nil. writer and
// auditFn may both be nil — Register then mounts List only, matching the
// nil-safe pattern used across this surface: a write endpoint that cannot
// be attributed to an operator (#287, F1) must not exist, so requeue and
// dead-letter only mount when BOTH are non-nil. See Register.
func NewOutboxHandler(db *gorm.DB, repo OutboxLister, writer OutboxWriter, auditFn outboxAuditFunc, logger *slog.Logger) *OutboxHandler {
	return &OutboxHandler{db: db, repo: repo, writer: writer, audit: auditFn, logger: logger, now: time.Now}
}

// Register mounts the routes on the supplied group. GET /admin/outbox
// (#331) always mounts when the handler exists at all. The three write
// routes (#405) mount only when both writer and audit are non-nil — see
// NewOutboxHandler.
func (h *OutboxHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/outbox", h.List)
	if h.writer != nil && h.audit != nil {
		g.POST("/admin/outbox/:id/requeue", h.RequeueSingle)
		g.POST("/admin/outbox/requeue", h.RequeueMany)
		g.POST("/admin/outbox/:id/dead-letter", h.DeadLetter)
	}
}

// outboxRow is the pinned contract shape.
//
// `payload` is DELIBERATELY absent, and absent by CONSTRUCTION: this struct
// is populated field by field from outbox.PlatformRow, which has no payload
// field either, so a column added to the model tomorrow cannot leak. It is
// arbitrary JSONB that may carry customer data, and a governance surface
// listing stuck events does not need it. Same reasoning that keeps
// `message` out of #332 and `description` out of #329.
//
// `error` is emitted as an OPAQUE string. outbox_events.error has no CHECK
// constraint and the operator requeue path is a raw UPDATE, so the codes
// this service writes are not the only values a consumer can observe. The
// console must render it with an unknown-value fallback, never a switch.
//
// `age_seconds` is absent for a published row: that row is settled and has
// no waiting time. A number that grew forever there would read as "stuck"
// beside a genuinely stuck row.
type outboxRow struct {
	ID          string  `json:"id"`
	TenantID    string  `json:"tenant_id"`
	Aggregate   string  `json:"aggregate"`
	AggregateID string  `json:"aggregate_id"`
	EventType   string  `json:"event_type"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	AgeSeconds  *int64  `json:"age_seconds,omitempty"`
	PublishedAt *string `json:"published_at,omitempty"`
	Error       *string `json:"error,omitempty"`
}

type outboxListResponse struct {
	Data       []outboxRow `json:"data"`
	Pagination pagination  `json:"pagination"`
}

// List handles GET /admin/outbox.
func (h *OutboxHandler) List(c *gin.Context) {
	// One instant for both the age and the older_than_minutes cutoff, so a
	// rendered age can never disagree with the filter that selected it.
	asOf := h.now().UTC()
	filter := h.parseFilter(c, asOf)

	result, err := h.repo.ListPlatform(c.Request.Context(), h.db, filter, asOf)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("platform outbox list", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "could not read outbox events",
		})
		return
	}

	// Allocate before appending: a nil slice marshals to null, which
	// defeats a caller's `?? []` precisely when there is no data.
	rows := make([]outboxRow, 0, len(result.Rows))
	for _, r := range result.Rows {
		rows = append(rows, toOutboxRow(r))
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = outbox.DefaultPlatformPageSize
	}
	if limit > outbox.MaxPlatformPageSize {
		limit = outbox.MaxPlatformPageSize
	}

	c.JSON(http.StatusOK, outboxListResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(filter.Page, 1),
			Limit: limit,
			Total: result.Total,
		},
	})
}

// toOutboxRow maps a query row to the pinned contract shape, FIELD BY
// FIELD. Nothing iterates the source struct, so the absence of payload is a
// property of this projection rather than of what the query happened to
// select.
func toOutboxRow(r outbox.PlatformRow) outboxRow {
	row := outboxRow{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Aggregate:   r.Aggregate,
		AggregateID: r.AggregateID,
		EventType:   r.EventType,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt.UTC().Format(time.RFC3339),
		AgeSeconds:  r.AgeSeconds,
		Error:       r.Error,
	}
	if r.PublishedAt != nil {
		s := r.PublishedAt.UTC().Format(time.RFC3339)
		row.PublishedAt = &s
	}
	return row
}

// parseFilter never returns an error. A missing parameter takes the
// default, an unparseable one is ignored, and an oversized limit clamps
// downstream rather than refusing — matching audit logs (#276), tickets
// (#329) and notifications (#332). It takes asOf rather than reading the
// clock so that since_hours, older_than_minutes and age_seconds are all
// measured from the SAME instant — a response whose rendered age disagreed
// with the window that selected it would be quietly wrong.
func (h *OutboxHandler) parseFilter(c *gin.Context, asOf time.Time) outbox.PlatformListFilter {
	f := outbox.PlatformListFilter{
		Status:    strings.TrimSpace(c.Query("status")),
		EventType: strings.TrimSpace(c.Query("event_type")),
		Page:      1,
	}

	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("older_than_minutes")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.OlderThanMinutes = n
		}
	}
	if v := strings.TrimSpace(c.Query("since_hours")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			from := asOf.Add(-time.Duration(n) * time.Hour)
			f.From = &from
		}
	}
	// tenant_id NARROWS rather than scopes — this endpoint is cross-tenant
	// by design.
	if v := strings.TrimSpace(c.Query("tenant_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.TenantID = &id
		}
	}
	return f
}

// requeueResponse is the wire shape for a single successful requeue.
type requeueResponse struct {
	ID                string `json:"id"`
	OriginalCreatedAt string `json:"original_created_at"`
}

// RequeueSingle handles POST /admin/outbox/:id/requeue.
//
// This is the operation the whole ticket exists to guard: requeue MUST
// refuse any row whose published_at is non-nil (h.writer.RequeueOne
// enforces this — see outbox.ErrAlreadyPublished), because clearing error
// on an already-published row would hand it back to the publisher and
// double-publish it, converting a delivery failure into a data-corruption
// problem.
func (h *OutboxHandler) RequeueSingle(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_id", "message": "id is not a valid uuid", "field": "id",
		})
		return
	}

	res, err := h.writer.RequeueOne(c.Request.Context(), h.db, id)
	if err != nil {
		h.respondOutboxWriteErr(c, "requeue", err)
		return
	}

	h.emitRequeueAudit(c, res.ID, res.TenantID, res.OriginalCreatedAt)

	c.JSON(http.StatusOK, requeueResponse{
		ID:                res.ID,
		OriginalCreatedAt: res.OriginalCreatedAt.UTC().Format(time.RFC3339),
	})
}

// requeueBatchRequest is the wire shape POST /admin/outbox/requeue accepts.
type requeueBatchRequest struct {
	IDs []string `json:"ids"`
}

// requeueOutcomeRow is one row's outcome in a batch requeue response — so
// one bad id does not fail the whole set.
type requeueOutcomeRow struct {
	ID                string `json:"id"`
	OK                bool   `json:"ok"`
	OriginalCreatedAt string `json:"original_created_at,omitempty"`
	Error             string `json:"error,omitempty"`
}

type requeueBatchResponse struct {
	Results []requeueOutcomeRow `json:"results"`
}

// RequeueMany handles POST /admin/outbox/requeue — batch requeue. Each id
// is requeued independently (h.writer.RequeueBatch runs each in its own
// transaction), so one bad id — not found, or already published — cannot
// fail the rest of the set; the response carries a per-row outcome rather
// than one overall status.
func (h *OutboxHandler) RequeueMany(c *gin.Context) {
	var req requeueBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "request body could not be parsed",
		})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "ids must be a non-empty list", "field": "ids",
		})
		return
	}

	outcomes := h.writer.RequeueBatch(c.Request.Context(), h.db, req.IDs)

	// Allocate before appending: a nil slice marshals to null, matching the
	// convention this surface uses for List's data array.
	rows := make([]requeueOutcomeRow, 0, len(outcomes))
	for _, o := range outcomes {
		row := requeueOutcomeRow{ID: o.ID, OK: o.OK, Error: o.Err}
		if o.OK {
			row.OriginalCreatedAt = o.OriginalCreatedAt.UTC().Format(time.RFC3339)
			h.emitRequeueAudit(c, o.ID, o.TenantID, o.OriginalCreatedAt)
		}
		rows = append(rows, row)
	}

	c.JSON(http.StatusOK, requeueBatchResponse{Results: rows})
}

// emitRequeueAudit records one outbox.requeued audit event. The event
// carries the row's ORIGINAL created_at, BEFORE requeue overwrote it —
// this is the only place that value survives, since requeue does not add
// a column to preserve it. A row whose tenant_id fails to parse as a UUID
// drops the audit event (logged, not surfaced): the requeue itself already
// happened, and failing the response would make the caller retry a write
// that succeeded.
func (h *OutboxHandler) emitRequeueAudit(c *gin.Context, id, tenantIDStr string, originalCreatedAt time.Time) {
	if h.audit == nil {
		return
	}
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("outbox requeue: row tenant_id is not a uuid, audit event dropped",
				"id", id, "err", err)
		}
		return
	}
	ev := audit.Event{
		Action:       "outbox.requeued",
		ResourceType: "outbox_event",
		ResourceID:   id,
		Metadata: map[string]any{
			// The ORIGINAL created_at, not the bumped one — see the doc
			// comment above.
			"original_created_at": originalCreatedAt.UTC().Format(time.RFC3339),
		},
	}
	if err := h.audit(c, tenantID, ev); err != nil {
		if h.logger != nil {
			h.logger.Error("outbox requeue: audit emit failed", "id", id, "err", err)
		}
	}
}

// deadLetterRequest is the wire shape POST /admin/outbox/:id/dead-letter
// accepts. Reason is REQUIRED — an empty (after trimming) reason is
// rejected before h.writer is ever called.
type deadLetterRequest struct {
	Reason string `json:"reason"`
}

type deadLetterResponse struct {
	ID             string `json:"id"`
	DeadLetteredAt string `json:"dead_lettered_at"`
	Reason         string `json:"reason"`
}

// DeadLetter handles POST /admin/outbox/:id/dead-letter. Like requeue, it
// refuses any row whose published_at is non-nil (h.writer.DeadLetterOne
// enforces this) — a delivered row cannot be dead-lettered.
func (h *OutboxHandler) DeadLetter(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_id", "message": "id is not a valid uuid", "field": "id",
		})
		return
	}

	var req deadLetterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "request body could not be parsed",
		})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "reason_required", "message": "reason is required and must not be empty", "field": "reason",
		})
		return
	}

	res, err := h.writer.DeadLetterOne(c.Request.Context(), h.db, id, reason)
	if err != nil {
		h.respondOutboxWriteErr(c, "dead-letter", err)
		return
	}

	if h.audit != nil {
		tenantID, perr := uuid.Parse(res.TenantID)
		if perr != nil {
			if h.logger != nil {
				h.logger.Error("outbox dead-letter: row tenant_id is not a uuid, audit event dropped",
					"id", res.ID, "err", perr)
			}
		} else {
			ev := audit.Event{
				Action:       "outbox.dead_lettered",
				ResourceType: "outbox_event",
				ResourceID:   res.ID,
				Metadata:     map[string]any{"reason": reason},
			}
			if aerr := h.audit(c, tenantID, ev); aerr != nil && h.logger != nil {
				h.logger.Error("outbox dead-letter: audit emit failed", "id", res.ID, "err", aerr)
			}
		}
	}

	c.JSON(http.StatusOK, deadLetterResponse{
		ID:             res.ID,
		DeadLetteredAt: res.DeadLetteredAt.UTC().Format(time.RFC3339),
		Reason:         reason,
	})
}

// respondOutboxWriteErr maps RequeueOne/DeadLetterOne's sentinel errors to
// distinct statuses, shared by both write endpoints. op names the
// operation for the internal-error log line only.
func (h *OutboxHandler) respondOutboxWriteErr(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, outbox.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "no outbox event with that id",
		})
	case errors.Is(err, outbox.ErrAlreadyPublished):
		c.JSON(http.StatusConflict, gin.H{
			"error":   "already_published",
			"message": "this event was already published; " + op + " would re-deliver it and is refused",
		})
	case errors.Is(err, outbox.ErrReasonRequired):
		// Defensive: the handler already validates reason before calling
		// h.writer, so this should be unreachable in production, but a
		// domain-level ErrReasonRequired must still map to something other
		// than a bare 500 if it ever fires.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "reason_required", "message": "reason is required and must not be empty", "field": "reason",
		})
	default:
		if h.logger != nil {
			h.logger.Error("outbox "+op+" failed", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not " + op + " event",
		})
	}
}
