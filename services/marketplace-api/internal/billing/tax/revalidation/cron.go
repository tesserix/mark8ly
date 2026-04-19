// Package revalidation implements the §19.5 quarterly tax-ID revalidation
// cron. Daily at 02:00 UTC the cron:
//  1. Re-runs the validator for every tax_id_validated=true subscription whose
//     last validation is >90 days old.
//  2. On a definitive failure (NotFound / InvalidFormat) — flips
//     tax_id_validated=false, sets tax_revalidation_started_at, emails the
//     merchant, and starts a 14-day grace window. Subscription status stays
//     `active` (billing continues — §19.5).
//  3. On the 14-day boundary, unpublishes the storefront. Billing keeps
//     running. The intentional design is "no perverse incentive": we don't
//     boot the merchant off billing for a tax registry hiccup; we just gate
//     the storefront until they fix it.
//
// Idempotency: the whole pass runs under a pg_advisory_xact_lock so multiple
// pods serialize. Per-row work uses a CAS sentinel on
// revalidation_attempted_at so a partial pass resumes cleanly.
package revalidation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	robcron "github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/notification"
)

// Spec — daily at 02:00 UTC. The 2-hour offset puts revalidation safely after
// midnight + the trial-ramp + lifecycle crons that run at 00:00 UTC.
const Spec = "CRON_TZ=UTC 0 2 * * *"

// Notifier is the narrow in-app notification contract the cron requires. The
// notification.Service satisfies it via Create(); production wiring passes
// the Service directly. Email delivery is a follow-up — the §19.5 spec
// requires merchant notification but the channel isn't pinned to email.
type Notifier interface {
	Create(ctx context.Context, n *notification.Notification) error
}

// Cron groups the dependencies needed by Run.
type Cron struct {
	DB       *gorm.DB
	Svc      *tax.Service
	Notifier Notifier
	Audit    *audit.Emitter
	Now      func() time.Time
	// BatchSize bounds the per-pass workload. Default 500.
	BatchSize int
}

// Register schedules the cron on the given scheduler.
func Register(scheduler *robcron.Cron, c Cron) (robcron.EntryID, error) {
	return scheduler.AddFunc(Spec, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := c.Run(ctx); err != nil {
			slog.Error("tax revalidation cron failed", "err", err)
		}
	})
}

// Run executes one revalidation pass. Safe to call manually for ops + tests.
func (c *Cron) Run(ctx context.Context) error {
	if c.Now == nil {
		c.Now = func() time.Time { return time.Now().UTC() }
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 500
	}

	return c.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext('tax_revalidation_cron'))`).Error; err != nil {
			return fmt.Errorf("revalidation: advisory lock: %w", err)
		}
		if err := c.recheckStaleValidations(ctx, tx); err != nil {
			return err
		}
		return c.unpublishAfter14Days(ctx, tx)
	})
}

type staleRow struct {
	TenantIDStr        string
	StoreIDStr         string
	TaxIDCountry       string
	ReverseChargeTaxID string
	BusinessName       string
}

func (c *Cron) recheckStaleValidations(ctx context.Context, tx *gorm.DB) error {
	rows, err := tx.WithContext(ctx).Raw(`
		SELECT tenant_id::text, store_id::text,
		       COALESCE(tax_id_country, ''),
		       COALESCE(reverse_charge_tax_id, ''),
		       ''  -- business_name not stored on the row; revalidation skips name match
		  FROM store_subscriptions
		 WHERE tax_id_validated = true
		   AND tax_id_validated_at < now() - INTERVAL '90 days'
		   AND (revalidation_attempted_at IS NULL OR revalidation_attempted_at < now() - INTERVAL '24 hours')
		 ORDER BY tax_id_validated_at ASC
		 LIMIT ?
	`, c.BatchSize).Rows()
	if err != nil {
		return fmt.Errorf("revalidation: select stale: %w", err)
	}
	defer rows.Close()

	var stale []staleRow
	for rows.Next() {
		var r staleRow
		if err := rows.Scan(&r.TenantIDStr, &r.StoreIDStr, &r.TaxIDCountry, &r.ReverseChargeTaxID, &r.BusinessName); err != nil {
			slog.Warn("revalidation: scan", "err", err)
			continue
		}
		stale = append(stale, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range stale {
		c.processOne(ctx, tx, r)
	}
	return nil
}

func (c *Cron) processOne(ctx context.Context, tx *gorm.DB, r staleRow) {
	tenantID, err1 := uuid.Parse(r.TenantIDStr)
	storeID, err2 := uuid.Parse(r.StoreIDStr)
	if err1 != nil || err2 != nil || r.TaxIDCountry == "" || r.ReverseChargeTaxID == "" {
		return
	}

	// CAS-like sentinel: stamp attempted_at first so retries skip this row.
	if err := tx.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		   SET revalidation_attempted_at = now()
		 WHERE tenant_id = ? AND store_id = ?
	`, tenantID, storeID).Error; err != nil {
		slog.Warn("revalidation: stamp attempted_at", "store_id", storeID, "err", err)
		return
	}

	submitErr := c.Svc.Submit(ctx, tax.SubmitInput{
		TenantID:     tenantID,
		StoreID:      storeID,
		Country:      r.TaxIDCountry,
		TaxID:        r.ReverseChargeTaxID,
		BusinessName: r.BusinessName,
		Source:       "revalidation",
	})
	if submitErr == nil {
		return // still valid
	}

	// Only definitive invalidity flips the row. Outage / manual review just
	// retry tomorrow.
	if !isDefinitiveFailure(submitErr) {
		return
	}

	if err := tx.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		   SET tax_id_validated            = false,
		       tax_revalidation_started_at = COALESCE(tax_revalidation_started_at, now()),
		       updated_at                  = now()
		 WHERE tenant_id = ? AND store_id = ?
	`, tenantID, storeID).Error; err != nil {
		slog.Warn("revalidation: flip invalid", "store_id", storeID, "err", err)
		return
	}

	if c.Notifier != nil {
		body := fmt.Sprintf("Your %s tax ID could not be revalidated. Please update it within 14 days to keep your storefront published.", r.TaxIDCountry)
		_ = c.Notifier.Create(ctx, &notification.Notification{
			TenantID: tenantID,
			StoreID:  storeID,
			Type:     notification.TypeSystemAlert,
			Title:    "Tax ID needs reverification",
			Message:  &body,
		})
	}

	if c.Audit != nil {
		c.Audit.Emit(nil, audit.Event{
			Action:       "subscription.tax_revalidation_failed",
			ResourceType: "subscription",
			ResourceID:   storeID.String(),
			TenantID:     tenantID,
			StoreID:      storeID,
			Severity:     audit.SeverityWarning,
			Metadata: map[string]any{
				"country":    r.TaxIDCountry,
				"grace_days": 14,
				"actor":      "system:cron:tax_revalidation",
			},
		})
	}
}

// unpublishAfter14Days flips storefront_published=false on subscriptions that
// failed revalidation more than 14 days ago. Status column intentionally
// untouched — billing continues per §19.5.
func (c *Cron) unpublishAfter14Days(ctx context.Context, tx *gorm.DB) error {
	r := tx.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		   SET storefront_published         = false,
		       storefront_unpublished_at    = now(),
		       storefront_unpublish_reason  = 'tax_revalidation_failed',
		       updated_at                   = now()
		 WHERE tax_id_validated = false
		   AND tax_revalidation_started_at IS NOT NULL
		   AND tax_revalidation_started_at < now() - INTERVAL '14 days'
		   AND storefront_published = true
	`)
	if r.Error != nil {
		return fmt.Errorf("revalidation: unpublish 14d: %w", r.Error)
	}
	if r.RowsAffected > 0 && c.Audit != nil {
		// One coarse audit row per pass — per-store audit would require a
		// separate SELECT, which the cron tries to avoid; ops can join
		// store_subscriptions to find the affected rows.
		c.Audit.Emit(nil, audit.Event{
			Action:       "subscription.storefront_unpublished_batch",
			ResourceType: "subscription",
			ResourceID:   "",
			Severity:     audit.SeverityWarning,
			Metadata: map[string]any{
				"reason":      "tax_revalidation_failed",
				"row_count":   r.RowsAffected,
				"actor":       "system:cron:tax_revalidation",
			},
		})
	}
	return nil
}

func isDefinitiveFailure(err error) bool {
	return errors.Is(err, tax.ErrNotFound) || errors.Is(err, tax.ErrInvalidFormat)
}
