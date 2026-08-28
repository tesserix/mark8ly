package emailevents

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Event is one provider delivery event, already verified and parsed.
type Event struct {
	// EventID is the provider's own id for this delivery. It is the
	// idempotency key: webhooks are at-least-once, so the same event arrives
	// again whenever a 2xx is missed.
	EventID string
	// SendID is our email_sends row, recovered from the custom arg piece A
	// injected. Ours, not the provider's — see internal/emaillog.
	SendID uuid.UUID
	Type   string
	At     time.Time
}

// statusRank orders the send lifecycle so a late event cannot regress a row.
//
// Events do not arrive in order, and a `delivered` landing after a `bounced`
// must not un-bounce the row: it would tell an operator mail arrived when it
// did not. Rank, not recency, decides.
var statusRank = map[string]int{
	"sending":    0,
	"sent":       1,
	"delivered":  2,
	"failed":     3,
	"complained": 3,
	"bounced":    3,
}

// eventStatus maps a provider event to the status it implies.
//
// `email.delivery_delayed` is deliberately absent: the message is still in
// flight, so any terminal status would be wrong. Unmapped types are ignored,
// not rejected — a provider adding an event type must not start a retry storm
// against us.
var eventStatus = map[string]string{
	"email.delivered":  "delivered",
	"email.bounced":    "bounced",
	"email.complained": "complained",
}

// Applier writes provider events onto email_sends rows.
type Applier struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewApplier constructs an Applier. logger may be nil.
func NewApplier(db *gorm.DB, logger *slog.Logger) *Applier {
	if logger == nil {
		logger = slog.Default()
	}
	return &Applier{db: db, logger: logger}
}

// Apply records the event and advances the send's status if the event implies
// a further state.
//
// Returns nil for events it deliberately ignores — an unknown send, an
// unmapped type, a replay. Each is a legitimate outcome, and returning an
// error would make the provider redeliver forever against a request that will
// never succeed.
func (a *Applier) Apply(ctx context.Context, ev Event) error {
	first, err := a.claim(ctx, ev)
	if err != nil {
		return err
	}
	if !first {
		// Already applied. Not an error: at-least-once delivery means this is
		// the expected path whenever our 2xx was lost.
		return nil
	}

	next, mapped := eventStatus[ev.Type]
	if !mapped {
		a.logger.Info("emailevents: ignoring unmapped event type",
			"type", ev.Type, "send_id", ev.SendID, "event_id", ev.EventID)
		return nil
	}

	// The rank comparison lives in SQL so it is atomic with the write: two
	// events for one send can arrive concurrently, and a read-then-write here
	// would let the loser overwrite the winner.
	res := a.db.WithContext(ctx).Exec(`
		UPDATE email_sends
		SET status = ?, event_at = ?, last_event_id = ?
		WHERE id = ?
		  AND COALESCE(?, 0) > CASE status
		        WHEN 'sending'    THEN 0
		        WHEN 'sent'       THEN 1
		        WHEN 'delivered'  THEN 2
		        WHEN 'failed'     THEN 3
		        WHEN 'complained' THEN 3
		        WHEN 'bounced'    THEN 3
		        ELSE 0 END`,
		next, ev.At, ev.EventID, ev.SendID, statusRank[next])
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// Either the send is unknown to us, or the row already holds an
		// outcome at least as final. Both are fine; neither is retryable.
		a.logger.Info("emailevents: event did not advance the row",
			"type", ev.Type, "send_id", ev.SendID, "event_id", ev.EventID)
	}
	return nil
}

// claim records the event id, reporting whether this is its first arrival.
// The primary key IS the idempotency check — no read-then-write race to lose.
func (a *Applier) claim(ctx context.Context, ev Event) (bool, error) {
	res := a.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Exec(`INSERT INTO email_send_events (event_id, send_id, event_type, received_at)
		      VALUES (?, ?, ?, now()) ON CONFLICT (event_id) DO NOTHING`,
			ev.EventID, ev.SendID, ev.Type)
	if res.Error != nil {
		return false, errors.New("emailevents: claim event: " + res.Error.Error())
	}
	return res.RowsAffected == 1, nil
}
