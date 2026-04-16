package audit

import (
	"context"
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
// the emitter; missing tenant or store causes the event to be dropped
// with a warning rather than written as an unscoped row.
type Event struct {
	Action       string         // "order.cancelled", "product.updated", ...
	ResourceType string         // "order" | "product" | "domain" | ...
	ResourceID   string         // optional; "" -> NULL
	Status       Status         // defaults to StatusSuccess
	Severity     Severity       // defaults to SeverityInfo
	Metadata     map[string]any // optional flat blob
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
func NewEmitter(cfg EmitterConfig) *Emitter {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1000
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 1
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
	return e
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
		// buildEntry already logged the reason (missing tenant/store).
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
			"store_id", entry.StoreID)
	}
}

// buildEntry assembles a row from the gin context + event. Returns nil
// (with a logged warning) when the request is missing tenant/store —
// audit must never write an unscoped row.
func buildEntry(c *gin.Context, ev Event) *Entry {
	tenantID, storeID, ok := scopeFromContext(c)
	if !ok {
		return nil
	}

	entry := &Entry{
		TenantID:     tenantID,
		StoreID:      storeID,
		ActorType:    ActorSystem,
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

// scopeFromContext pulls the tenant + store IDs that scope every audit
// row. Both must parse as valid UUIDs or the event is dropped.
func scopeFromContext(c *gin.Context) (tenantID, storeID uuid.UUID, ok bool) {
	if c == nil {
		return uuid.Nil, uuid.Nil, false
	}
	tid, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	sid, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	return tid, sid, true
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
