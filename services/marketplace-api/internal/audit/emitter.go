package audit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Event is the input to Emitter.Emit. Required fields are validated by
// the emitter; a missing tenant causes the event to be dropped with a
// warning rather than written as a tenant-unscoped row. Store is optional
// — platform-originated writes are tenant-scoped only.
type Event struct {
	Action       string         // "order.cancelled", "product.updated", ...
	ResourceType string         // "order" | "product" | "domain" | ...
	ResourceID   string         // optional; "" -> NULL
	Status       Status         // defaults to StatusSuccess
	Severity     Severity       // defaults to SeverityInfo
	Metadata     map[string]any // optional flat blob

	// TenantID and StoreID are normally pulled from the gin context
	// (set by HeaderTrustAuth + the :storeId path param). Storefront
	// routes don't carry either — they use :storeSlug and resolve the
	// store via middleware — so storefront callers populate these
	// explicitly. When set, these win over context lookup. StoreID is
	// optional: leave it uuid.Nil for tenant-scoped platform writes
	// (tenant suspend, trial extend, purge) that have no store — the
	// resulting audit row gets a nil store_id rather than being dropped.
	TenantID uuid.UUID
	StoreID  uuid.UUID

	// ForceActorType overrides the actor classification. Storefront
	// emitters pass ActorSystem here so audit rows for customer-driven
	// actions (signup, checkout) don't get tagged as the merchant.
	ForceActorType ActorType
}

// Emitter is an async, fire-and-forget audit log writer. Emit() is
// non-blocking: events are dropped (with a warning) if the worker is
// behind. Audit must never gate a business request.
type Emitter struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger

	queue chan Entry
	wg    sync.WaitGroup
	stop  chan struct{}
}

// EmitterConfig groups Emitter construction params.
type EmitterConfig struct {
	DB     *gorm.DB
	Repo   Repository
	Logger *slog.Logger
	// QueueSize bounds the in-memory backlog. Once full, Emit drops new
	// events and logs a warning. Default 1000.
	QueueSize int
	// Workers is the number of goroutines that drain the queue.
	// Default 1 — DB inserts are cheap and a single writer keeps log
	// ordering naturally close to wall-clock order.
	Workers int
}

// NewEmitter constructs an Emitter and starts its worker goroutines.
// Call Stop() at process shutdown to drain the queue.
//
// cfg.Repo is required: it is dereferenced on every write (Emit's worker
// and EmitSync both call repo.Create). A nil Repo returns an error instead
// of a broken Emitter. If auditing is intentionally disabled — e.g. in a
// unit test — do not pass a nil Repo here; use a nil *Emitter instead
// (var em *audit.Emitter). Every exported method (all eleven — see
// nil_repo_test.go's TestNilEmitter_AllExportedMethodsAreSafe) is
// nil-receiver-safe, so that is the supported opt-out, not a nil Repo.
//
// cfg.DB may legitimately be nil: the Emitter never dereferences it
// itself — it is opaque data forwarded to Repo.Create on each write, so
// whether nil is safe depends on the Repository implementation (a test
// double may not need it at all). Note that the shipped Repository —
// audit.NewRepository()'s gormRepository — DOES dereference it
// (db.WithContext(ctx).Create(e)); pairing a real Repo from
// NewRepository() with a nil DB is a wiring error. gormRepository.Create
// returns an error for a nil db rather than panicking, so that wiring
// error surfaces as a logged insert failure instead of a worker-goroutine
// panic, but it is still a bug to fix, not a supported configuration.
func NewEmitter(cfg EmitterConfig) (*Emitter, error) {
	if cfg.Repo == nil {
		return nil, errors.New("audit.NewEmitter: cfg.Repo is nil; to disable auditing, use a nil *audit.Emitter instead of NewEmitter with a nil Repo")
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1000
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	e := &Emitter{
		db:     cfg.DB,
		repo:   cfg.Repo,
		logger: cfg.Logger,
		queue:  make(chan Entry, cfg.QueueSize),
		stop:   make(chan struct{}),
	}
	for i := 0; i < cfg.Workers; i++ {
		e.wg.Add(1)
		go e.worker()
	}
	return e, nil
}

// Emit enqueues an event for async write. Returns immediately. Safe to
// call from request handlers. If c is nil the event is recorded as a
// system actor with no IP/UA/email context — useful for background jobs.
func (e *Emitter) Emit(c *gin.Context, ev Event) {
	if e == nil {
		return // safe to call when wiring opted out (e.g. unit tests)
	}
	if ev.Action == "" || ev.ResourceType == "" {
		e.logger.Warn("audit.Emit: action and resource_type are required",
			"action", ev.Action, "resource_type", ev.ResourceType)
		return
	}

	entry := buildEntry(c, ev)
	if entry == nil {
		e.logger.Warn("audit.Emit: dropping event, no tenant in scope",
			"action", ev.Action,
			"resource_type", ev.ResourceType)
		return
	}

	select {
	case e.queue <- *entry:
		// queued
	default:
		e.logger.Warn("audit.Emit: queue full, dropping event",
			"action", ev.Action,
			"resource_type", ev.ResourceType,
			"queue_size", cap(e.queue))
	}
}

// EmitSync writes an audit row on the CALLER's goroutine and returns the
// outcome. It is the exception to this package's fire-and-forget rule, and
// it exists for exactly one situation: an action whose own effect can
// destroy the audit row recording it.
//
// The tenant purge (#288) deletes `audit_logs WHERE tenant_id = ?`. Emit
// hands the row to a background worker, so an Emit on that path races its
// own DELETE — the row may land before the purge and be destroyed, after
// it and survive, or (queue full) never be written at all. None of those
// is an audit trail.
//
// Use Emit everywhere else. Audit must never gate a business request, and
// this function does exactly that.
//
// buildEntry is REUSED rather than an Entry assembled here: a second
// derivation of actor type, operator id, capability and scope would drift
// the moment either path gained a field.
func (e *Emitter) EmitSync(c *gin.Context, ev Event) error {
	if e == nil {
		return errors.New("audit.EmitSync: nil emitter")
	}
	if ev.Action == "" || ev.ResourceType == "" {
		return errors.New("audit.EmitSync: action and resource_type are required")
	}

	entry := buildEntry(c, ev)
	if entry == nil {
		return errors.New("audit.EmitSync: no tenant in scope, refusing to write a tenant-unscoped row")
	}

	// Use a fresh background context with a timeout rather than the
	// request's — matching write(). A client disconnecting mid-purge must
	// not cancel the record of what was destroyed.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.repo.Create(ctx, e.db, entry); err != nil {
		return fmt.Errorf("audit.EmitSync: insert: %w", err)
	}
	return nil
}

// Stop signals workers to drain the queue and exit. Safe to call once;
// further Emit calls after Stop will block briefly until workers exit
// then drop. Intended to be called from main on shutdown signal.
func (e *Emitter) Stop(ctx context.Context) {
	if e == nil {
		return
	}
	close(e.stop)
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		e.logger.Warn("audit.Emitter: shutdown timed out, in-flight events may be lost")
	}
}

func (e *Emitter) worker() {
	defer e.wg.Done()
	for {
		select {
		case entry := <-e.queue:
			e.write(entry)
		case <-e.stop:
			// drain remaining queued entries before exiting.
			for {
				select {
				case entry := <-e.queue:
					e.write(entry)
				default:
					return
				}
			}
		}
	}
}

func (e *Emitter) write(entry Entry) {
	// Use a fresh background context with a short timeout so a slow DB
	// can't pin the worker indefinitely. Audit writes are ephemeral —
	// dropping one on timeout is preferable to backing up the queue.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.repo.Create(ctx, e.db, &entry); err != nil {
		e.logger.Error("audit.Emitter: insert failed",
			"err", err,
			"action", entry.Action,
			"resource_type", entry.ResourceType,
			"tenant_id", entry.TenantID,
			"store_id", storeIDForLog(entry.StoreID))
	}
}

// buildEntry assembles a row from the gin context + event. Returns nil
// when the request is missing a tenant — audit must never write a
// tenant-unscoped row. Emit logs a warning when this happens. Store is
// optional: platform writes are tenant-scoped only. See resolveScope.
func buildEntry(c *gin.Context, ev Event) *Entry {
	tenantID, storeID, ok := resolveScope(c, ev)
	if !ok {
		return nil
	}

	actorType := ActorSystem
	if ev.ForceActorType != "" {
		actorType = ev.ForceActorType
	}

	entry := &Entry{
		TenantID:     tenantID,
		StoreID:      storeID,
		ActorType:    actorType,
		Action:       ev.Action,
		ResourceType: ev.ResourceType,
		Status:       defaultStatus(ev.Status),
		Severity:     defaultSeverity(ev.Severity),
		Metadata:     Metadata(ev.Metadata),
	}
	if ev.ResourceID != "" {
		v := ev.ResourceID
		entry.ResourceID = &v
	}

	if c != nil {
		// Context keys "platform_operator_id" / "platform_capability" are
		// set by Task 6's platform-admin middleware. They mirror the
		// exported constants platformadmin.CtxOperatorID / CtxCapability
		// defined there; audit intentionally does not import that package
		// (it's an HTTP handler package), so the literals are duplicated
		// here — keep both sides greppable if either changes.
		operatorID := strings.TrimSpace(c.GetString("platform_operator_id"))
		capability := strings.TrimSpace(c.GetString("platform_capability"))

		switch {
		case operatorID != "":
			// A platform console operator. The id is opaque and belongs to
			// the console — it must never be written to actor_user_id, even
			// when it happens to parse as a UUID. This case is evaluated
			// before, and (via switch's implicit no-fallthrough) mutually
			// exclusive with, the user-actor case below — an operator claim
			// can never fall through into ActorUserID.
			entry.ActorType = ActorOperator
			entry.ActorOperatorID = &operatorID
			if capability != "" {
				entry.Capability = &capability
			}

		case ev.ForceActorType == "":
			// Don't infer a user actor when the caller explicitly forced a
			// system/api classification — storefront events should stay as
			// system even though the request carries a customer session.
			if uid := strings.TrimSpace(c.GetString("user_id")); uid != "" {
				if parsed, err := uuid.Parse(uid); err == nil {
					entry.ActorUserID = &parsed
					entry.ActorType = ActorUser
				}
			}
			if email := strings.TrimSpace(c.GetString("user_email")); email != "" {
				v := email
				entry.ActorEmail = &v
			}
		}

		if ip := clientIP(c); ip != "" {
			v := ip
			entry.IPAddress = &v
		}
		if ua := c.Request.UserAgent(); ua != "" {
			v := ua
			entry.UserAgent = &v
		}
	}

	return entry
}

// resolveScope picks tenant + store IDs from the explicit Event fields
// first, then falls back to the gin context. The tenant must resolve;
// the store is optional and nil for tenant-scoped platform events.
func resolveScope(c *gin.Context, ev Event) (tenantID uuid.UUID, storeID *uuid.UUID, ok bool) {
	tenantID = ev.TenantID
	var store uuid.UUID = ev.StoreID
	if c != nil {
		if tenantID == uuid.Nil {
			if tid, err := uuid.Parse(c.GetString("tenant_id")); err == nil {
				tenantID = tid
			}
		}
		if store == uuid.Nil {
			if sid, err := uuid.Parse(c.Param("storeId")); err == nil {
				store = sid
			}
		}
	}
	if tenantID == uuid.Nil {
		return uuid.Nil, nil, false
	}
	if store == uuid.Nil {
		return tenantID, nil, true
	}
	return tenantID, &store, true
}

// storeIDForLog renders a nil store as "-" rather than a pointer address.
func storeIDForLog(id *uuid.UUID) string {
	if id == nil {
		return "-"
	}
	return id.String()
}

func defaultStatus(s Status) Status {
	if s == "" {
		return StatusSuccess
	}
	return s
}

func defaultSeverity(s Severity) Severity {
	if s == "" {
		return SeverityInfo
	}
	return s
}

// clientIP returns a Postgres-INET-friendly representation of the
// request's source IP. Strips the port so PG doesn't reject e.g.
// "10.0.0.1:54321". Returns "" when no parseable IP is available so
// the caller writes NULL.
func clientIP(c *gin.Context) string {
	raw := strings.TrimSpace(c.ClientIP())
	if raw == "" {
		return ""
	}
	// gin.ClientIP usually returns just the IP, but be defensive.
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	if net.ParseIP(raw) == nil {
		return ""
	}
	return raw
}

// StateTransition records a subscription state move for the audit trail.
// Populate Actor with either a user UUID string (merchant-driven) or a
// sentinel like "system:webhook:stripe" / "system:cron:trial_expiry"
// for automated transitions.
type StateTransition struct {
	StoreID       uuid.UUID
	TenantID      uuid.UUID // optional; pulled from gin ctx when zero
	From          string
	To            string
	Plan          string // current plan snapshot; aids cross-state dashboards
	Actor         string // "system:webhook:stripe" | "user:<uuid>" | ...
	Reason        string // "invoice.paid" | "merchant_cancelled" | ...
	StripeEventID string
}

// EmitStateTransition is the canonical hook for every subscription state
// change (§23.1). It builds a standard Event and delegates to Emit.
func (e *Emitter) EmitStateTransition(c *gin.Context, t StateTransition) {
	md := map[string]any{
		"from_status": t.From,
		"to_status":   t.To,
		"actor":       t.Actor,
	}
	if t.Plan != "" {
		md["plan"] = t.Plan
	}
	if t.Reason != "" {
		md["reason"] = t.Reason
	}
	if t.StripeEventID != "" {
		md["stripe_event_id"] = t.StripeEventID
	}

	severity := SeverityInfo
	switch t.To {
	case "expired", "store_closed", "pending_hard_delete", "hard_deleted", "payment_action_required":
		severity = SeverityWarning
	}

	e.Emit(c, Event{
		Action:         "subscription.state_transition",
		ResourceType:   "subscription",
		ResourceID:     t.StoreID.String(),
		Severity:       severity,
		Metadata:       md,
		TenantID:       t.TenantID,
		StoreID:        t.StoreID,
		ForceActorType: classifyActor(t.Actor),
	})
}

func classifyActor(actor string) ActorType {
	if strings.HasPrefix(actor, "system:") {
		return ActorSystem
	}
	return ActorUser
}

// APIKeyEvent records a lifecycle event on an enterprise API key (§18.4).
// Action is one of "created" / "rotated" / "revoked"; Severity defaults to
// info except "revoked" with reason="compromised" which the caller bumps to
// warning.
type APIKeyEvent struct {
	TenantID  uuid.UUID
	StoreID   uuid.UUID
	KeyID     uuid.UUID
	KeyPrefix string
	Action    string // "created" | "rotated" | "revoked"
	Reason    string // optional; merchant_revoked | compromised | scheduled_rotation
	ActorUser uuid.UUID
}

// EmitAPIKeyEvent is the canonical hook for API-key lifecycle audit. Mirrors
// the EmitStateTransition shape so dashboards can union the two streams.
func (e *Emitter) EmitAPIKeyEvent(c *gin.Context, ev APIKeyEvent) {
	md := map[string]any{
		"key_id":     ev.KeyID.String(),
		"key_prefix": ev.KeyPrefix,
		"action":     ev.Action,
	}
	if ev.Reason != "" {
		md["reason"] = ev.Reason
	}
	severity := SeverityInfo
	if ev.Action == "revoked" && ev.Reason == "compromised" {
		severity = SeverityWarning
	}
	actor := "user:" + ev.ActorUser.String()
	if ev.ActorUser == uuid.Nil {
		actor = "system:apikeys"
	}
	md["actor"] = actor

	e.Emit(c, Event{
		Action:         "api_key." + ev.Action,
		ResourceType:   "api_key",
		ResourceID:     ev.KeyID.String(),
		Severity:       severity,
		Metadata:       md,
		TenantID:       ev.TenantID,
		StoreID:        ev.StoreID,
		ForceActorType: classifyActor(actor),
	})
}

// PlanChange records a subscription plan and/or billing-period change for the
// audit trail. Populate Actor with either a user UUID ("user:<uuid>") for
// merchant-initiated changes or a system sentinel (e.g. "system:cron:downgrade_recheck").
type PlanChange struct {
	TenantID    uuid.UUID
	StoreID     uuid.UUID
	FromPlan    string
	ToPlan      string
	FromPeriod  string
	ToPeriod    string
	Subaction   string // "upgrade_committed" | "downgrade_scheduled" | "downgrade_committed" | "downgrade_blocked_over_quota" | "period_switch_committed"
	Actor       string
	Reason      string
	EffectiveAt time.Time
}

// EmitPlanChange is the canonical hook for merchant-initiated plan changes and
// cron-driven downgrade commits/blocks (§4.5). It wraps EmitStateTransition
// with a dedicated action constant and carries the v2.3 subaction + period
// fields in metadata.
func (e *Emitter) EmitPlanChange(c *gin.Context, p PlanChange) {
	md := map[string]any{
		"from_plan":   p.FromPlan,
		"to_plan":     p.ToPlan,
		"from_period": p.FromPeriod,
		"to_period":   p.ToPeriod,
		"subaction":   p.Subaction,
		"actor":       p.Actor,
	}
	if p.Reason != "" {
		md["reason"] = p.Reason
	}
	if !p.EffectiveAt.IsZero() {
		md["effective_at"] = p.EffectiveAt.UTC().Format(time.RFC3339)
	}

	severity := SeverityInfo
	if p.Subaction == "downgrade_blocked_over_quota" {
		severity = SeverityWarning
	}

	e.Emit(c, Event{
		Action:         "subscription.plan_changed",
		ResourceType:   "subscription",
		ResourceID:     p.StoreID.String(),
		Severity:       severity,
		Metadata:       md,
		TenantID:       p.TenantID,
		StoreID:        p.StoreID,
		ForceActorType: classifyActor(p.Actor),
	})
}
