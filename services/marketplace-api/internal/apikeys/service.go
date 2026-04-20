package apikeys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// Sentinel errors surfaced to the admin handlers.
var (
	ErrPlanDoesNotAllowAPI         = errors.New("apikeys: plan does not allow API keys")
	ErrWriteScopeRequiresPro       = errors.New("apikeys: write scopes require Pro plan")
	ErrRateLimitExceedsPlanCeiling = errors.New("apikeys: rate_limit_per_min exceeds plan ceiling")
	ErrCannotRotateRevoked         = errors.New("apikeys: cannot rotate a revoked key")
)

// RotationOverlap — §18.4 24h grace where the rotated key remains valid so
// the integrator can swap at leisure.
const RotationOverlap = 24 * time.Hour

// PlanCeiling returns the maximum rate_limit_per_min the given plan can set,
// or 0 when the plan does not permit API keys at all.
//   - Trial / Starter: 0 (no API keys)
//   - Studio: 200 (read-only)
//   - Pro:    1000 (read + write)
func PlanCeiling(p subscription.SubscriptionPlan) int {
	switch p {
	case subscription.PlanStudio:
		return 200
	case subscription.PlanPro, subscription.PlanMarketplace:
		return 1000
	default:
		return 0
	}
}

// Service holds the business logic for key creation, rotation, revocation.
// Holds a gorm.DB reference *only* for transaction scoping in Rotate; all
// other DB work goes through the repo.
type Service struct {
	repo    *Repo
	cache   *Cache
	db      *gorm.DB
	env     Env
	emitter *audit.Emitter
	now     func() time.Time
}

// NewService constructs a Service. db is required for the Rotate transaction;
// pass the same handle that backs the repo. emitter may be nil; lifecycle
// audit events are then no-ops.
func NewService(db *gorm.DB, repo *Repo, cache *Cache, env Env, emitter *audit.Emitter) *Service {
	return &Service{
		repo:    repo,
		cache:   cache,
		db:      db,
		env:     env,
		emitter: emitter,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// CreateInput is the orchestrator's contract from the admin handler.
type CreateInput struct {
	TenantID        uuid.UUID
	StoreID         uuid.UUID
	CreatedBy       uuid.UUID
	Scopes          []string
	RateLimitPerMin int
	Label           string
	Plan            subscription.SubscriptionPlan
}

// CreateResult carries the freshly-minted plaintext (returned exactly once)
// alongside the persisted row's ID and a UI-display masked form.
type CreateResult struct {
	ID        uuid.UUID
	Plaintext string
	Display   string
}

// Create issues a new key. Plan-ceiling, scope-tier, and rate-limit checks
// run before any DB write; any failure short-circuits with a typed sentinel.
func (s *Service) Create(ctx context.Context, in CreateInput) (CreateResult, error) {
	if err := ValidateScopes(in.Scopes); err != nil {
		return CreateResult{}, err
	}
	ceiling := PlanCeiling(in.Plan)
	if ceiling == 0 {
		return CreateResult{}, ErrPlanDoesNotAllowAPI
	}
	if !AllReadOnly(in.Scopes) && !plangate.IsAllowed(in.Plan, plangate.FeatureFullAPI) {
		return CreateResult{}, ErrWriteScopeRequiresPro
	}
	if in.RateLimitPerMin <= 0 {
		in.RateLimitPerMin = 100
	}
	if in.RateLimitPerMin > ceiling {
		return CreateResult{}, fmt.Errorf("%w (plan ceiling = %d)", ErrRateLimitExceedsPlanCeiling, ceiling)
	}
	if in.Label == "" {
		return CreateResult{}, errors.New("apikeys: label required")
	}

	gen, err := Generate(s.env)
	if err != nil {
		return CreateResult{}, fmt.Errorf("apikeys: generate: %w", err)
	}
	hash, err := Hash(gen.Plaintext)
	if err != nil {
		return CreateResult{}, fmt.Errorf("apikeys: hash: %w", err)
	}

	row := APIKey{
		ID:              uuid.New(),
		TenantID:        in.TenantID,
		StoreID:         in.StoreID,
		KeyPrefix:       gen.Prefix,
		KeyHash:         hash,
		Scopes:          ScopeSet(in.Scopes),
		RateLimitPerMin: in.RateLimitPerMin,
		Label:           in.Label,
		CreatedBy:       in.CreatedBy,
	}
	if err := s.repo.Create(ctx, &row); err != nil {
		return CreateResult{}, fmt.Errorf("apikeys: persist: %w", err)
	}

	s.emit(audit.APIKeyEvent{
		TenantID: row.TenantID, StoreID: row.StoreID,
		KeyID: row.ID, KeyPrefix: row.KeyPrefix,
		Action: "created", ActorUser: in.CreatedBy,
	})

	return CreateResult{
		ID:        row.ID,
		Plaintext: gen.Plaintext,
		Display:   Display(gen.Plaintext),
	}, nil
}

// RotateResult carries the new key's plaintext + when the old key's overlap
// window closes. Both belong in the HTTP response so the integrator can swap.
type RotateResult struct {
	NewID        uuid.UUID
	NewPlaintext string
	OldExpiresAt time.Time
}

// Rotate issues a fresh key inheriting scopes/label/rate-limit from the old
// row and back-dates the old row's revoked_at to now()+24h. Both rows live
// in the same transaction so a partial failure leaves no orphan.
func (s *Service) Rotate(ctx context.Context, tenantID, oldID uuid.UUID, reason string) (RotateResult, error) {
	old, err := s.repo.FindByID(ctx, oldID)
	if err != nil {
		return RotateResult{}, err
	}
	if old.TenantID != tenantID {
		return RotateResult{}, ErrNotFound
	}
	if !old.IsUsable(s.now()) {
		return RotateResult{}, ErrCannotRotateRevoked
	}

	gen, err := Generate(s.env)
	if err != nil {
		return RotateResult{}, fmt.Errorf("apikeys: generate: %w", err)
	}
	hash, err := Hash(gen.Plaintext)
	if err != nil {
		return RotateResult{}, fmt.Errorf("apikeys: hash: %w", err)
	}

	overlapEnd := s.now().Add(RotationOverlap)
	newRow := APIKey{
		ID:               uuid.New(),
		TenantID:         old.TenantID,
		StoreID:          old.StoreID,
		KeyPrefix:        gen.Prefix,
		KeyHash:          hash,
		Scopes:           old.Scopes,
		RateLimitPerMin:  old.RateLimitPerMin,
		Label:            old.Label,
		CreatedBy:        old.CreatedBy,
		RotationReplaces: &old.ID,
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&newRow).Error; err != nil {
			return err
		}
		return tx.Model(&APIKey{}).Where("id = ?", old.ID).
			Updates(map[string]any{
				"revoked_at":     overlapEnd,
				"revoked_reason": reason,
			}).Error
	})
	if err != nil {
		return RotateResult{}, fmt.Errorf("apikeys: rotate transaction: %w", err)
	}

	if s.cache != nil {
		s.cache.InvalidateByKeyID(old.ID)
	}
	s.emit(audit.APIKeyEvent{
		TenantID: old.TenantID, StoreID: old.StoreID,
		KeyID: newRow.ID, KeyPrefix: newRow.KeyPrefix,
		Action: "rotated", Reason: reason, ActorUser: old.CreatedBy,
	})
	return RotateResult{
		NewID:        newRow.ID,
		NewPlaintext: gen.Plaintext,
		OldExpiresAt: overlapEnd,
	}, nil
}

// Revoke immediately marks the key revoked. Idempotent on already-revoked
// rows (returns nil without changing revoked_at).
func (s *Service) Revoke(ctx context.Context, tenantID, id uuid.UUID, reason string) error {
	old, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if old.TenantID != tenantID {
		return ErrNotFound
	}
	if old.RevokedAt != nil && !old.RevokedAt.After(s.now()) {
		return nil // already revoked outright
	}
	if err := s.repo.Revoke(ctx, id, s.now(), reason); err != nil {
		return fmt.Errorf("apikeys: revoke: %w", err)
	}
	if s.cache != nil {
		s.cache.InvalidateByKeyID(id)
	}
	s.emit(audit.APIKeyEvent{
		TenantID: old.TenantID, StoreID: old.StoreID,
		KeyID: id, KeyPrefix: old.KeyPrefix,
		Action: "revoked", Reason: reason, ActorUser: old.CreatedBy,
	})
	return nil
}

// List returns metadata for all keys belonging to a tenant+store, most
// recent first. Caller is responsible for redacting any sensitive field
// before serialising to HTTP.
func (s *Service) List(ctx context.Context, tenantID, storeID uuid.UUID) ([]APIKey, error) {
	return s.repo.ListForStore(ctx, tenantID, storeID)
}

// emit is a nil-safe wrapper around audit.Emitter.EmitAPIKeyEvent.
func (s *Service) emit(ev audit.APIKeyEvent) {
	if s.emitter == nil {
		return
	}
	s.emitter.EmitAPIKeyEvent(nil, ev)
}
