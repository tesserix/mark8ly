package webhook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DeliveryRepo struct{ db *gorm.DB }

func NewDeliveryRepo(db *gorm.DB) *DeliveryRepo { return &DeliveryRepo{db: db} }

// FanOut inserts delivery rows in ONE statement, ignoring any that already
// exist.
//
// ON CONFLICT DO NOTHING against idx_webhook_deliveries_event_sub is what
// makes dispatch idempotent, and therefore what lets the dispatcher run
// OUTSIDE the outbox publisher's transaction without risking duplicate
// deliveries. Re-reading the same outbox rows is harmless — which is also
// what pays for the dispatcher's lookback window.
//
// next_attempt_at is SQL now(), not the caller's clock. Every timestamp in
// this table is written by the database so that ClaimDue's `next_attempt_at
// <= now()` compares two readings of the same clock; a Delivery's
// NextAttemptAt field is ignored on insert for that reason.
func (r *DeliveryRepo) FanOut(ctx context.Context, rows []Delivery) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	var b strings.Builder
	b.WriteString(`INSERT INTO webhook_deliveries
		(subscription_id, outbox_event_id, event_type, aggregate_id, status, next_attempt_at)
		VALUES `)
	args := make([]any, 0, len(rows)*5)
	for i, d := range rows {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("(?,?,?,?,?,now())")
		args = append(args, d.SubscriptionID, d.OutboxEventID, d.EventType, d.AggregateID, d.Status)
	}
	b.WriteString(` ON CONFLICT (outbox_event_id, subscription_id) DO NOTHING`)

	res := r.db.WithContext(ctx).Exec(b.String(), args...)
	if res.Error != nil {
		return 0, fmt.Errorf("webhook: fan out deliveries: %w", res.Error)
	}
	return int(res.RowsAffected), nil
}

// ClaimDue claims up to limit pending, due deliveries by taking a short
// LEASE, not a lock.
//
// FOR UPDATE SKIP LOCKED only holds its row lock while THIS function's own
// transaction is open, and that transaction commits here, before the caller
// ever makes the outbound HTTP call. Postgres releases the lock at commit —
// well before RecordOutcome moves status off pending, up to RequestTimeout
// later. If ClaimDue simply returned the claimed rows at that point, a
// second worker calling ClaimDue while the first is still mid-Send would
// see them as pending and unlocked, and send them again. Several replicas
// running this loop at once (Task 6 puts this on KEDA-scaled pods) is
// exactly the case that must not double-send.
//
// So within the same transaction that claims the rows, it immediately
// pushes their next_attempt_at forward by LeaseWindow. That's what actually
// keeps the rows from being claimed again: they are still status=pending,
// but no longer due. The HTTP send itself happens entirely OUTSIDE any
// transaction — holding a connection and row locks across a blocking
// outbound call to a merchant server is not an option on a 5-connection
// pool shared with the rest of the service. RecordOutcome then overwrites
// next_attempt_at with the real outcome (retry backoff, or dead-letter).
//
// If a worker dies mid-send, no RecordOutcome ever runs and the lease
// simply expires — the row becomes due again and some worker retries it.
// That's at-least-once delivery, which is what webhooks already assume:
// the signature and delivery id are what let a merchant dedupe.
func (r *DeliveryRepo) ClaimDue(ctx context.Context, limit int) ([]Delivery, error) {
	var out []Delivery
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`
			SELECT * FROM webhook_deliveries
			 WHERE status = ? AND next_attempt_at <= now()
			 ORDER BY next_attempt_at ASC
			 LIMIT ?
			 FOR UPDATE SKIP LOCKED`, StatusPending, limit).Scan(&out).Error; err != nil {
			return fmt.Errorf("webhook: claim deliveries: %w", err)
		}
		if len(out) == 0 {
			return nil
		}

		ids := make([]uuid.UUID, len(out))
		for i, d := range out {
			ids[i] = d.ID
		}
		// now() + interval, not a Go timestamp: the predicate above reads
		// SQL now(), so the lease has to be written against the same clock
		// or a pod running fast/slow shortens or extends every lease.
		var leased []struct {
			ID            uuid.UUID
			NextAttemptAt time.Time
		}
		if err := tx.Raw(`
			UPDATE webhook_deliveries
			   SET next_attempt_at = now() + ?::interval
			 WHERE id IN ?
			RETURNING id, next_attempt_at`, intervalArg(LeaseWindow), ids).Scan(&leased).Error; err != nil {
			return fmt.Errorf("webhook: lease deliveries: %w", err)
		}
		// Reflect the lease in what's returned too, so a caller reading
		// NextAttemptAt off these structs doesn't see stale pre-lease data.
		byID := make(map[uuid.UUID]time.Time, len(leased))
		for _, l := range leased {
			byID[l.ID] = l.NextAttemptAt
		}
		for i := range out {
			if t, ok := byID[out[i].ID]; ok {
				out[i].NextAttemptAt = t
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RecordOutcome writes the result of one attempt. retryIn is how far ahead
// of SQL now() the next attempt becomes due; pass 0 for a terminal outcome.
//
// Every timestamp here comes from the database clock, matching FanOut and
// ClaimDue. Mixing a pod clock into next_attempt_at while ClaimDue's
// predicate reads now() is the same class of assumption that produced the
// dispatcher's lost-event bug.
func (r *DeliveryRepo) RecordOutcome(ctx context.Context, id uuid.UUID, status string, code *int, errMsg *string, retryIn time.Duration) error {
	updates := map[string]any{
		"status":           status,
		"attempts":         gorm.Expr("attempts + 1"),
		"last_status_code": code,
		"last_error":       errMsg,
		"next_attempt_at":  gorm.Expr("now() + ?::interval", intervalArg(retryIn)),
	}
	if status == StatusDelivered {
		updates["delivered_at"] = gorm.Expr("now()")
	}
	if err := r.db.WithContext(ctx).Model(&Delivery{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("webhook: record outcome: %w", err)
	}
	return nil
}

// ListForSubscription returns the most recent deliveries for one
// subscription, most recent first, for the admin delivery log.
func (r *DeliveryRepo) ListForSubscription(ctx context.Context, subscriptionID uuid.UUID, limit int) ([]Delivery, error) {
	var out []Delivery
	err := r.db.WithContext(ctx).
		Where("subscription_id = ?", subscriptionID).
		Order("created_at DESC").
		Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("webhook: list deliveries: %w", err)
	}
	return out, nil
}

// Replay resets one delivery to pending, due now, so the worker's next poll
// picks it up. Scoped to subscriptionID — the caller must have already
// verified that subscription belongs to their tenant and store — so a
// deliveryID belonging to a different subscription is silently a no-op
// rather than a cross-tenant write. Reports whether a row matched.
//
// `status <> pending` is a guard, not a filter for tidiness: a pending row
// may be LEASED right now (see ClaimDue), mid-flight in some worker's
// outbound request. Resetting next_attempt_at under it would break the
// lease and let a second worker claim and send the same delivery — an admin
// button that manufactures a duplicate send. Only a settled delivery
// (delivered or failed) is replayable; a pending one is already going to be
// attempted.
func (r *DeliveryRepo) Replay(ctx context.Context, subscriptionID, deliveryID uuid.UUID) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE webhook_deliveries
		   SET status = ?, attempts = 0, next_attempt_at = now()
		 WHERE id = ? AND subscription_id = ? AND status <> ?`,
		StatusPending, deliveryID, subscriptionID, StatusPending)
	if res.Error != nil {
		return false, fmt.Errorf("webhook: replay delivery: %w", res.Error)
	}
	return res.RowsAffected > 0, nil
}

// Prune deletes delivery rows older than the retention window. 30 days on
// every plan, deliberately not tied to FeatureAuditRetentionDays: "forever"
// retention of delivery bodies on Pro is storage cost on a db-f1-micro with
// no matching merchant value.
func (r *DeliveryRepo) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	res := r.db.WithContext(ctx).Exec(
		`DELETE FROM webhook_deliveries WHERE created_at < now() - ?::interval`,
		fmt.Sprintf("%d hours", int(olderThan.Hours())))
	if res.Error != nil {
		return 0, fmt.Errorf("webhook: prune deliveries: %w", res.Error)
	}
	return res.RowsAffected, nil
}
