package arbitrage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrNoOpenFlag is returned by AppealService.Submit when there is no ongoing
// arbitrage flag for the given store.
var ErrNoOpenFlag = errors.New("arbitrage: no open arbitrage flag for store")

// Publisher abstracts the billing-ops queue so tests can inject a spy.
// Production wires a NoOpPublisher until real Pub/Sub is enabled (deferred).
type Publisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

// NoOpPublisher logs the appeal payload and persists it to the audit row only.
// Real Pub/Sub wiring is deferred — billing-ops also polls the table directly
// via the `ongoing` partial index, so this is a safe intermediate state.
type NoOpPublisher struct{}

func (NoOpPublisher) Publish(_ context.Context, _ string, _ any) error { return nil }

// AppealInput is the merchant-submitted form (§18.8.1).
type AppealInput struct {
	TenantID      uuid.UUID
	StoreID       uuid.UUID
	Jurisdiction  string    // ISO-2 the merchant claims to operate from
	Justification string    // free text, trimmed to 1000 chars
	DocumentURL   string    // gs:// URI, optional
	ActorUserID   uuid.UUID // the admin user submitting the appeal
}

// AppealService updates the latest ongoing audit row in-place and queues a
// billing-ops review message. No new schema is required — it writes into the
// reviewed_by / reviewed_at / mismatch_reason fields of the existing row.
type AppealService struct {
	db        *gorm.DB
	publisher Publisher
	piiLogger PIILogger
}

// NewAppealService constructs an AppealService.
func NewAppealService(db *gorm.DB, pub Publisher, pii PIILogger) *AppealService {
	return &AppealService{db: db, publisher: pub, piiLogger: pii}
}

// Submit updates the latest ongoing audit row for the store and enqueues a
// billing-ops review. Resolution STAYS "ongoing" — only a billing-ops reviewer
// can move it to false_positive_cleared or reprice_developed.
//
// Returns ErrNoOpenFlag when the store has no ongoing arbitrage flag.
func (s *AppealService) Submit(ctx context.Context, in AppealInput) error {
	s.piiLogger.LogPIIAccess(ctx, PIIAccessEvent{
		Actor:     in.ActorUserID,
		StoreID:   in.StoreID,
		TenantID:  in.TenantID,
		Operation: "arbitrage_appeal_submit",
	})

	now := time.Now().UTC()
	var row SubscriptionArbitrageAudit
	q := s.db.WithContext(ctx).
		Where("tenant_id = ? AND store_id = ? AND resolution = 'ongoing'", in.TenantID, in.StoreID).
		Order("flagged_at DESC").
		Limit(1)
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNoOpenFlag
		}
		return fmt.Errorf("load audit row: %w", err)
	}

	justification := in.Justification
	if len(justification) > 1000 {
		justification = justification[:1000]
	}

	existingReason := ""
	if row.MismatchReason != nil {
		existingReason = *row.MismatchReason
	}
	appended := existingReason + "\n---\nMERCHANT_APPEAL jurisdiction=" + NormalizeCountry(in.Jurisdiction)
	if justification != "" {
		appended += " justification=" + justification
	}
	if in.DocumentURL != "" {
		appended += " doc=" + in.DocumentURL
	}

	actor := in.ActorUserID
	if err := s.db.WithContext(ctx).
		Model(&SubscriptionArbitrageAudit{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"reviewed_by":     &actor,
			"reviewed_at":     &now,
			"mismatch_reason": appended,
			// resolution stays "ongoing" until billing-ops closes it.
		}).Error; err != nil {
		// This path lost an appeal silently for as long as the column was too
		// narrow (#398): the merchant got a 500, but nothing logged the cause,
		// billing-ops was never notified (the Publish below is skipped), and
		// the PII log above already recorded a "submitted" appeal.
		slog.Default().Error("arbitrage appeal: update audit row failed",
			"audit_id", row.ID, "tenant_id", in.TenantID, "store_id", in.StoreID,
			"appended_len", len(appended), "err", err)
		return fmt.Errorf("update audit row: %w", err)
	}

	// Enqueue billing-ops review. Non-fatal if publish fails — billing-ops
	// also polls the table directly via the ongoing partial index.
	_ = s.publisher.Publish(ctx, "billing-ops.arbitrage-appeal", map[string]any{
		"audit_id":        row.ID,
		"subscription_id": row.SubscriptionID,
		"tenant_id":       row.TenantID,
		"store_id":        row.StoreID,
		"jurisdiction":    NormalizeCountry(in.Jurisdiction),
		"document_url":    in.DocumentURL,
		"submitted_at":    now,
	})
	return nil
}
