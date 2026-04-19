package appcreds

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	wlmetrics "github.com/mark8ly/marketplace-api/internal/whitelabel/metrics"
)

// ErrNotFound is returned by Service.Load when the requested credential
// does not exist in Secret Manager. Callers render this to 404 at the
// HTTP boundary (never 403: 403 leaks "credential exists").
var ErrNotFound = errors.New("appcreds: credential not found")

// Config groups Service construction params.
type Config struct {
	ProjectID string         // GCP project hosting the merchant_* secrets
	SM        SM             // production: NewGCPSM; tests: NewFakeSM
	Emitter   *audit.Emitter // audit stream for every access
}

// Service is the single choke-point for white-label app credential I/O.
// Every Store / Load / Delete / PurgeAll call emits an audit event and
// increments the Prometheus credential-access counter.
//
// Use NewService once at boot; pass it around as a dependency. Do not
// construct one per request.
type Service struct {
	projectID string
	sm        SM
	emitter   *audit.Emitter
}

// NewService constructs a Service from the supplied Config. The returned
// value is nil-safe for calls (checks happen in each method) but callers
// should set both SM and Emitter in production.
func NewService(cfg Config) *Service {
	return &Service{
		projectID: cfg.ProjectID,
		sm:        cfg.SM,
		emitter:   cfg.Emitter,
	}
}

// StoreInput is the payload for Service.Store.
type StoreInput struct {
	TenantID uuid.UUID
	StoreID  uuid.UUID
	CredType CredType
	Payload  []byte
	Actor    string // "user:<uuid>" | "system:cron:lifecycle" | etc.
}

// LoadInput is the payload for Service.Load.
type LoadInput struct {
	TenantID uuid.UUID
	StoreID  uuid.UUID
	CredType CredType
	Actor    string
}

// DeleteInput is the payload for Service.Delete.
type DeleteInput struct {
	TenantID uuid.UUID
	StoreID  uuid.UUID
	CredType CredType
	Actor    string
}

// Store persists a credential into Secret Manager and emits the write
// audit event. Returns wrapped error on SM failure.
func (s *Service) Store(ctx context.Context, in StoreInput) error {
	if err := validateCommonInput(in.TenantID, in.CredType); err != nil {
		return err
	}
	name := Path(s.projectID, in.TenantID.String(), in.CredType)
	if err := s.sm.CreateOrAddVersion(ctx, name, in.Payload); err != nil {
		return fmt.Errorf("appcreds: store %s: %w", in.CredType, err)
	}
	s.emit(in.TenantID, in.StoreID, in.CredType, "write", in.Actor)
	return nil
}

// Load reads the latest version of a credential. Every call emits a read
// audit event AND increments the prom counter — this is the signal P17
// dashboards alert on for unusual access patterns.
//
// Returns ErrNotFound when the secret doesn't exist. Never returns a
// permission-denied error with metadata that would confirm existence —
// that's for the IAM layer to enforce out-of-band (§18.9).
func (s *Service) Load(ctx context.Context, in LoadInput) ([]byte, error) {
	if err := validateCommonInput(in.TenantID, in.CredType); err != nil {
		return nil, err
	}
	name := Path(s.projectID, in.TenantID.String(), in.CredType)
	payload, err := s.sm.AccessLatest(ctx, name)
	if err != nil {
		if errors.Is(err, ErrSMNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("appcreds: load %s: %w", in.CredType, err)
	}

	// Audit + metrics AFTER a successful read — a failed SM call that
	// errored shouldn't pollute the access counter.
	s.emit(in.TenantID, in.StoreID, in.CredType, "read", in.Actor)
	wlmetrics.CredentialAccessed.WithLabelValues(string(in.CredType)).Inc()

	return payload, nil
}

// Delete removes a single credential. Idempotent: a second Delete on the
// same cred emits the audit event (so the teardown timeline is complete)
// and returns nil.
func (s *Service) Delete(ctx context.Context, in DeleteInput) error {
	if err := validateCommonInput(in.TenantID, in.CredType); err != nil {
		return err
	}
	name := Path(s.projectID, in.TenantID.String(), in.CredType)
	if err := s.sm.Delete(ctx, name); err != nil {
		return fmt.Errorf("appcreds: delete %s: %w", in.CredType, err)
	}
	s.emit(in.TenantID, in.StoreID, in.CredType, "delete", in.Actor)
	return nil
}

// PurgeAll removes every known CredType for a tenant. Used at §13.5 day
// 90 when the app lifecycle ends. Emits one audit event per CredType so
// the purge is auditable as a sequence.
//
// If any single delete fails, the error is returned and the remaining
// creds are NOT touched — callers (the lifecycle advancer) should retry
// the same action; Delete is idempotent, so a previously-deleted cred
// stays deleted.
func (s *Service) PurgeAll(ctx context.Context, tenantID, storeID uuid.UUID, actor string) error {
	for _, ct := range AllCredTypes() {
		if err := s.Delete(ctx, DeleteInput{
			TenantID: tenantID, StoreID: storeID, CredType: ct, Actor: actor,
		}); err != nil {
			return fmt.Errorf("appcreds: purge all (%s): %w", ct, err)
		}
	}
	return nil
}

// emit builds and fires the audit event. Centralised so the Operation
// label stays in sync with the Action naming scheme on the audit side.
func (s *Service) emit(tenantID, storeID uuid.UUID, ct CredType, op, actor string) {
	if s.emitter == nil {
		return // permitted in tests that don't care about audit
	}
	s.emitter.EmitCredentialAccess(nil, audit.CredentialAccess{
		TenantID:       tenantID,
		StoreID:        storeID,
		CredentialType: string(ct),
		Operation:      op,
		Actor:          actor,
	})
}

// validateCommonInput catches the fatal mis-wirings up front so callers
// get a clear error rather than a confusing SM 4xx/panic.
func validateCommonInput(tenantID uuid.UUID, ct CredType) error {
	if tenantID == uuid.Nil {
		return fmt.Errorf("appcreds: tenantID is required")
	}
	if ct == "" {
		return fmt.Errorf("appcreds: credType is required")
	}
	// Closed-enum guard — reject unknown types so a future caller
	// can't accidentally store a secret under a name PurgeAll won't
	// iterate.
	for _, valid := range AllCredTypes() {
		if ct == valid {
			return nil
		}
	}
	return fmt.Errorf("appcreds: unknown credType %q (must be one of AllCredTypes)", ct)
}
