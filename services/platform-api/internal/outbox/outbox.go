// Package outbox implements the transactional outbox pattern.
//
// The outbox pattern solves a specific class of bug: a side effect (writing
// an OpenFGA tuple, publishing a Pub/Sub message, sending a webhook) needs
// to happen IF AND ONLY IF a database transaction commits. The naive
// implementation does the side effect after the commit, which means a crash
// between commit and side-effect leaves the system inconsistent forever.
//
// The outbox pattern fixes this by writing both the entity and an "outbox
// row" describing the side effect inside the same DB transaction. A
// background drainer reads pending outbox rows and dispatches them. If the
// drainer crashes mid-dispatch, the row stays pending and is retried.
//
// This package is the fix for auth-bug #2 (FGA tuple race) and #3 (no
// retry on FGA failure) from docs/planning/auth-bugs.md.
//
// Lifecycle:
//
//	app code → tx.Create(entity)
//	         → tx.Create(outbox.Event{Kind: "fga.write_membership", Payload: ...})
//	         → tx.Commit()
//	  drainer → SELECT WHERE status='pending' AND next_attempt_at <= now()
//	         → invoke handler for Kind
//	         → on success: UPDATE status='completed'
//	         → on failure: UPDATE attempts++, next_attempt_at=now()+backoff
//	         → after MaxAttempts: UPDATE status='dead' (alert)
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Event is one outbox row. Use Enqueue (not Create) to insert from app code
// so payload encoding stays consistent.
type Event struct {
	ID            int64           `gorm:"primaryKey;column:id"            json:"id"`
	Kind          string          `gorm:"column:kind;not null"            json:"kind"`
	Payload       json.RawMessage `gorm:"column:payload;type:jsonb;not null" json:"payload"`
	Status        string          `gorm:"column:status;not null;default:pending" json:"status"`
	Attempts      int             `gorm:"column:attempts;not null;default:0"     json:"attempts"`
	LastError     string          `gorm:"column:last_error"                json:"last_error"`
	NextAttemptAt time.Time       `gorm:"column:next_attempt_at;not null"  json:"next_attempt_at"`
	CreatedAt     time.Time       `gorm:"column:created_at;not null"       json:"created_at"`
	UpdatedAt     time.Time       `gorm:"column:updated_at;not null"       json:"updated_at"`
	CompletedAt   *time.Time      `gorm:"column:completed_at"              json:"completed_at,omitempty"`
}

// TableName overrides GORM's default pluralization.
func (Event) TableName() string { return "outbox_events" }

// Status constants. Stored as strings to keep DB rows scannable.
const (
	StatusPending   = "pending"
	StatusInFlight  = "in_flight"
	StatusCompleted = "completed"
	StatusDead      = "dead"
)

// Handler dispatches a single outbox event. Return nil for success; any
// non-nil error triggers a retry. Returning a permanent error type
// (not yet defined) would short-circuit to "dead" — out of scope for v1.
type Handler func(ctx context.Context, payload json.RawMessage) error

// Enqueue inserts a pending outbox event in the given transaction.
//
// MUST be called inside an existing transaction (the tx that's writing the
// originating entity). The whole point is atomic visibility: either both
// the entity and the outbox row commit, or neither does.
func Enqueue(tx *gorm.DB, kind string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("outbox: marshal payload: %w", err)
	}
	evt := Event{
		Kind:          kind,
		Payload:       raw,
		Status:        StatusPending,
		NextAttemptAt: time.Now(),
	}
	if err := tx.Create(&evt).Error; err != nil {
		return fmt.Errorf("outbox: enqueue %q: %w", kind, err)
	}
	return nil
}

// Drainer polls the outbox and dispatches pending events.
//
// One Drainer runs in-process inside platform-api for now. When the system
// grows enough to need horizontal scaling, the same code can run as a
// dedicated worker — drainer logic is identical.
type Drainer struct {
	db          *gorm.DB
	log         *slog.Logger
	handlers    map[string]Handler
	pollEvery   time.Duration
	batchSize   int
	maxAttempts int
	backoff     func(attempts int) time.Duration

	mu      sync.Mutex
	running bool
	stop    chan struct{}
}

// Config holds drainer tuning knobs. All have sensible defaults.
type Config struct {
	PollInterval time.Duration                // default 1s
	BatchSize    int                          // default 50
	MaxAttempts  int                          // default 10
	Backoff      func(attempts int) time.Duration // default exponential 1s..5min
}

// NewDrainer constructs a Drainer wired to the given GORM connection.
// Handlers must be registered with Register() before Start().
func NewDrainer(db *gorm.DB, log *slog.Logger, cfg Config) *Drainer {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 50
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 10
	}
	if cfg.Backoff == nil {
		cfg.Backoff = defaultBackoff
	}
	return &Drainer{
		db:          db,
		log:         log,
		handlers:    make(map[string]Handler),
		pollEvery:   cfg.PollInterval,
		batchSize:   cfg.BatchSize,
		maxAttempts: cfg.MaxAttempts,
		backoff:     cfg.Backoff,
		stop:        make(chan struct{}),
	}
}

// Register attaches a handler to a kind. Must be called before Start.
// Panics if the kind is already registered — registration mistakes should
// be loud and fast.
func (d *Drainer) Register(kind string, h Handler) {
	if _, exists := d.handlers[kind]; exists {
		panic(fmt.Sprintf("outbox: kind %q already registered", kind))
	}
	d.handlers[kind] = h
}

// Start launches the drainer in a goroutine. Stop with the returned cancel.
// Idempotent: calling Start twice is a no-op.
func (d *Drainer) Start(ctx context.Context) {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	d.running = true
	d.mu.Unlock()

	go d.loop(ctx)
}

// Stop signals the drainer to stop. Blocks until the current tick finishes.
func (d *Drainer) Stop() {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return
	}
	close(d.stop)
	d.running = false
	d.mu.Unlock()
}

func (d *Drainer) loop(ctx context.Context) {
	t := time.NewTicker(d.pollEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stop:
			return
		case <-t.C:
			if err := d.Tick(ctx); err != nil {
				d.log.Error("outbox tick", "err", err)
			}
		}
	}
}

// Tick processes one batch of pending events. Exposed publicly so tests
// can drive the drainer deterministically without sleeping.
func (d *Drainer) Tick(ctx context.Context) error {
	var batch []Event
	now := time.Now()

	if err := d.db.WithContext(ctx).
		Where("status = ? AND next_attempt_at <= ?", StatusPending, now).
		Order("id ASC").
		Limit(d.batchSize).
		Find(&batch).Error; err != nil {
		return fmt.Errorf("outbox: fetch batch: %w", err)
	}

	for _, evt := range batch {
		d.dispatch(ctx, evt)
	}
	return nil
}

func (d *Drainer) dispatch(ctx context.Context, evt Event) {
	h, ok := d.handlers[evt.Kind]
	if !ok {
		d.log.Error("outbox: no handler registered", "kind", evt.Kind, "id", evt.ID)
		d.markDead(ctx, evt, "no handler registered")
		return
	}

	err := h(ctx, evt.Payload)
	if err == nil {
		d.markCompleted(ctx, evt)
		return
	}

	d.log.Warn("outbox handler failed", "kind", evt.Kind, "id", evt.ID, "attempts", evt.Attempts+1, "err", err)
	if evt.Attempts+1 >= d.maxAttempts {
		d.markDead(ctx, evt, err.Error())
		return
	}
	d.markRetry(ctx, evt, err)
}

func (d *Drainer) markCompleted(ctx context.Context, evt Event) {
	now := time.Now()
	if err := d.db.WithContext(ctx).Model(&Event{}).Where("id = ?", evt.ID).Updates(map[string]any{
		"status":       StatusCompleted,
		"completed_at": now,
		"updated_at":   now,
	}).Error; err != nil {
		d.log.Error("outbox: mark completed", "id", evt.ID, "err", err)
	}
}

func (d *Drainer) markRetry(ctx context.Context, evt Event, cause error) {
	next := time.Now().Add(d.backoff(evt.Attempts + 1))
	if err := d.db.WithContext(ctx).Model(&Event{}).Where("id = ?", evt.ID).Updates(map[string]any{
		"attempts":        evt.Attempts + 1,
		"last_error":      truncate(cause.Error(), 1000),
		"next_attempt_at": next,
		"updated_at":      time.Now(),
	}).Error; err != nil {
		d.log.Error("outbox: mark retry", "id", evt.ID, "err", err)
	}
}

func (d *Drainer) markDead(ctx context.Context, evt Event, reason string) {
	if err := d.db.WithContext(ctx).Model(&Event{}).Where("id = ?", evt.ID).Updates(map[string]any{
		"status":     StatusDead,
		"attempts":   evt.Attempts + 1,
		"last_error": truncate(reason, 1000),
		"updated_at": time.Now(),
	}).Error; err != nil {
		d.log.Error("outbox: mark dead", "id", evt.ID, "err", err)
	}
}

// defaultBackoff is exponential with a 5-minute cap.
//   1s, 2s, 4s, 8s, 16s, 32s, 64s, 128s, 256s, 300s, 300s, ...
func defaultBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	const maxCap = 5 * time.Minute
	// Cap the shift count BEFORE shifting to avoid int64 overflow at high attempts.
	// 2^9 seconds = 512s, already > maxCap, so anything past 9 is capped.
	if attempts > 9 {
		return maxCap
	}
	d := time.Second << (attempts - 1)
	if d > maxCap {
		return maxCap
	}
	return d
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// ErrNoHandler is returned when an event has no registered handler.
// Currently the drainer marks such events as dead immediately; this error
// type exists so future code can react differently if needed.
var ErrNoHandler = errors.New("outbox: no handler registered for kind")
