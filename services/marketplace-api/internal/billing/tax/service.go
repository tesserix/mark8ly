package tax

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/seaqueue"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// ServiceConfig wires the orchestrator's collaborators. Audit and Clock are
// optional (nil-safe); Registry, SEAQueue, and DB are required.
type ServiceConfig struct {
	DB       *gorm.DB
	Registry *Registry
	Audit    *audit.Emitter
	SEAQueue *seaqueue.Repository
	Clock    *ClockPauseTracker
	NowFunc  func() time.Time
}

// Service is the single place that mutates tax_id_validated /
// tax_id_validated_at / tax_id_name_match on store_subscriptions and the
// only place that enqueues SEA manual review or records registry outages.
type Service struct {
	cfg ServiceConfig
}

// NewService returns a configured orchestrator. Caller owns lifetimes of all
// collaborators.
func NewService(cfg ServiceConfig) *Service {
	if cfg.NowFunc == nil {
		cfg.NowFunc = func() time.Time { return time.Now().UTC() }
	}
	return &Service{cfg: cfg}
}

// SubmitInput is the orchestrator's contract. Source distinguishes admin
// submits ("signup", "remediation") from cron-driven retries ("revalidation");
// audit metadata carries it through.
type SubmitInput struct {
	TenantID       uuid.UUID
	StoreID        uuid.UUID
	Country        string
	TaxID          string
	BusinessName   string
	BillingAddress string
	Source         string
}

// Submit runs the per-country validator, persists the verdict, and emits the
// matching audit event. Return value semantics:
//
//   - nil — validated, row flipped, audit emitted.
//   - ErrManualReviewRequired — SEA queue entry created, clock paused; merchant
//     should see HTTP 202 Accepted with an SLA hint.
//   - ErrRegistryUnavailable — outage logged; merchant should retry. Clock will
//     pause once cumulative outage exceeds the §5.2 72h threshold.
//   - ErrInvalidFormat / ErrNotFound / ErrValidatorDisabled — return as-is, no
//     DB write.
func (s *Service) Submit(ctx context.Context, in SubmitInput) error {
	v, ok := s.cfg.Registry.For(in.Country)
	if !ok {
		return fmt.Errorf("tax: unsupported country %q", in.Country)
	}

	req := ValidationRequest{
		TenantID:       in.TenantID.String(),
		StoreID:        in.StoreID.String(),
		Country:        in.Country,
		TaxID:          in.TaxID,
		BusinessName:   in.BusinessName,
		BillingAddress: in.BillingAddress,
	}

	res, err := v.Validate(ctx, req)
	switch {
	case errors.Is(err, ErrValidatorDisabled):
		return err

	case errors.Is(err, ErrRegistryUnavailable):
		_ = s.cfg.Clock.BeginOutage(ctx, OutageKey{
			StoreID:    in.StoreID,
			TenantID:   in.TenantID,
			Country:    in.Country,
			Registry:   RegistryFor(in.Country),
			ErrorClass: "outage",
		}, s.cfg.NowFunc())
		return err

	case errors.Is(err, ErrInvalidFormat), errors.Is(err, ErrNotFound):
		return err

	case err != nil:
		return fmt.Errorf("tax: validator error: %w", err)
	}

	if res.ManualReviewRequired {
		_, qerr := s.cfg.SEAQueue.Enqueue(ctx, seaqueue.Entry{
			TenantID:     in.TenantID,
			StoreID:      in.StoreID,
			Country:      in.Country,
			TaxID:        in.TaxID,
			BusinessName: in.BusinessName,
			QueueReason:  res.QueueReason,
		})
		if qerr != nil {
			return fmt.Errorf("tax: enqueue manual review: %w", qerr)
		}
		// Council finding #10: clock pauses at queue ENTRY, not resolution.
		_ = s.cfg.Clock.BeginOutage(ctx, OutageKey{
			StoreID:    in.StoreID,
			TenantID:   in.TenantID,
			Country:    in.Country,
			Registry:   RegistryFor(in.Country),
			ErrorClass: "sea_queue",
		}, s.cfg.NowFunc())
		s.emitAudit(in, "subscription.tax_id_manual_review_queued", string(NameNotChecked))
		return ErrManualReviewRequired
	}

	if !res.Valid {
		return ErrNotFound
	}

	match := CompareNames(in.BusinessName, res.RegistryName)

	err = subscription.WithAdvisoryLock(ctx, s.cfg.DB, in.StoreID, func(tx *gorm.DB) error {
		now := s.cfg.NowFunc()
		r := tx.Exec(`
			UPDATE store_subscriptions
			   SET tax_id_validated    = true,
			       tax_id_validated_at = ?,
			       tax_id_name_match   = ?,
			       updated_at          = now()
			 WHERE tenant_id = ? AND store_id = ?
		`, now, string(match), in.TenantID, in.StoreID)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return fmt.Errorf("tax: subscription not found for store %s", in.StoreID)
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.emitAudit(in, "subscription.tax_id_validated", string(match))
	return nil
}

// OnFastPathApproved flips tax_id_window_shortened_at on the subscription so
// the windowguard middleware uses the 48h fast-path window. Called by P5
// when CSM approves a migration fast-path review.
func (s *Service) OnFastPathApproved(ctx context.Context, tenantID, storeID uuid.UUID) error {
	now := s.cfg.NowFunc()
	r := s.cfg.DB.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		   SET tax_id_window_shortened_at = ?,
		       updated_at                 = now()
		 WHERE tenant_id = ? AND store_id = ?
		   AND tax_id_window_shortened_at IS NULL
	`, now, tenantID, storeID)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected > 0 && s.cfg.Audit != nil {
		s.cfg.Audit.Emit(nil, audit.Event{
			Action:       "subscription.tax_window_shortened",
			ResourceType: "subscription",
			ResourceID:   storeID.String(),
			TenantID:     tenantID,
			StoreID:      storeID,
			Metadata: map[string]any{
				"window":   "fast_path",
				"duration": "48h",
				"actor":    "system:cron:migration_fast_path",
			},
		})
	}
	return nil
}

func (s *Service) emitAudit(in SubmitInput, action, nameMatch string) {
	if s.cfg.Audit == nil {
		return
	}
	s.cfg.Audit.Emit(nil, audit.Event{
		Action:       action,
		ResourceType: "subscription",
		ResourceID:   in.StoreID.String(),
		TenantID:     in.TenantID,
		StoreID:      in.StoreID,
		Metadata: map[string]any{
			"country":    in.Country,
			"registry":   RegistryFor(in.Country),
			"name_match": nameMatch,
			"source":     in.Source,
		},
	})
}

// RegistryFor returns the human-readable upstream registry name for a country.
// Used in audit metadata + outage logs.
func RegistryFor(country string) string {
	switch country {
	case "GB":
		return "HMRC"
	case "IE", "DE", "FR", "IT", "ES", "NL":
		return "VIES"
	case "AU":
		return "ABR"
	case "IN":
		return "GSTN"
	case "SG":
		return "ACRA"
	case "NZ":
		return "IRD"
	case "MY":
		return "MOF_SST"
	case "TH":
		return "RD"
	case "PH":
		return "BIR"
	case "ID":
		return "DJP"
	case "VN":
		return "GDT"
	case "US", "CA":
		return "ATTESTATION"
	}
	return "UNKNOWN"
}
