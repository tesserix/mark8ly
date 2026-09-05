package customer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// JoinStoreInput is the input for creating a store membership.
type JoinStoreInput struct {
	StoreID   uuid.UUID
	TenantID  uuid.UUID
	GipUID    string
	Email     string
	FirstName string
	LastName  string
}

// Service provides customer business logic.
type Service struct {
	repo   Repository
	db     *gorm.DB
	audit  *audit.Emitter // optional — nil-safe; emits customer.signed_up
	logger *slog.Logger
}

// NewService constructs a Service.
func NewService(db *gorm.DB, repo Repository, logger *slog.Logger) *Service {
	return &Service{db: db, repo: repo, logger: logger}
}

// WithAudit attaches an audit emitter so first-time storefront signups
// produce a customer.signed_up audit event. Nil-safe.
func (s *Service) WithAudit(e *audit.Emitter) *Service {
	s.audit = e
	return s
}

// ErrBlocked is returned by JoinStore when a membership row already
// exists for (store_id, email) but the merchant has blocked it. Joining
// must never resurrect or reset a blocked customer, so the join is
// refused outright rather than upserted over.
var ErrBlocked = errors.New("customer: membership is blocked")

// signupFreshness is how recently a profile must have been created for
// JoinStore to treat the upsert as a fresh signup. The repo reload
// happens within milliseconds of the insert, so 2s comfortably covers
// jitter without false-positive-ing on rapid re-logins.
const signupFreshness = 2 * time.Second

// LookupProfile returns the membership row for (store_id, email), or
// ErrNotFound when this identity has not joined this store.
//
// This is the ONLY customer-profile call any session path may make. A
// Mark8ly login is platform-wide, but access to a store is a distinct
// membership the customer has to join deliberately; before this existed,
// the session path called the creating upsert below and every
// authenticated request at any store minted a membership as a side
// effect (see docs/superpowers/specs/2026-09-05-customer-store-membership-design.md).
func (s *Service) LookupProfile(ctx context.Context, storeID uuid.UUID, email string) (*CustomerProfile, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, ErrNotFound
	}
	return s.repo.GetProfileByEmail(ctx, storeID, email)
}

// JoinStore creates the customer's membership of a store: the
// customer_profiles row keyed on (store_id, email), and nothing else —
// no new identity, no new credential. It is reached ONLY from an
// explicit join (the storefront /account/join endpoint and the mobile
// /account/register endpoint), never from a session or browsing path.
//
// Uses INSERT ON CONFLICT (store_id, email) DO UPDATE SET gip_uid = EXCLUDED.gip_uid
// so a double-submitted join is safe. The conflict branch deliberately
// touches only gip_uid/updated_at, so re-joining can never reset status,
// block_reason, tags, or notes; a blocked membership is additionally
// refused up front with ErrBlocked rather than relying on that.
//
// Returns the existing or newly created profile. When called from a
// gin handler (c may be nil for non-HTTP callers) and the upsert
// inserted a new row, emits a customer.signed_up audit event.
func (s *Service) JoinStore(ctx context.Context, input JoinStoreInput, c *gin.Context) (*CustomerProfile, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" {
		return nil, fmt.Errorf("customer: email is required to join a store")
	}

	// Refuse before writing anything: a merchant who blocked this
	// customer must not have that undone by the customer re-joining.
	existing, err := s.repo.GetProfileByEmail(ctx, input.StoreID, email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("customer: join store: %w", err)
	}
	if existing != nil && existing.Status == StatusBlocked {
		return nil, ErrBlocked
	}

	profile := &CustomerProfile{
		TenantID:  input.TenantID,
		StoreID:   input.StoreID,
		GipUID:    nilIfEmpty(input.GipUID),
		Email:     email,
		FirstName: nilIfEmpty(input.FirstName),
		LastName:  nilIfEmpty(input.LastName),
		Status:    StatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result, err := s.repo.UpsertProfile(ctx, profile)
	if err != nil {
		return nil, fmt.Errorf("customer: join store: %w", err)
	}

	s.logger.Info("customer joined store",
		"store_id", input.StoreID,
		"email", email,
		"profile_id", result.ID,
	)

	// Detect a fresh signup by checking how recently the persisted row
	// was created. The repo reload happens immediately after the upsert,
	// so a freshly inserted row's CreatedAt is still very close to now;
	// an existing row's CreatedAt is older. Edge case: rapid re-login
	// inside signupFreshness double-fires — acceptable noise floor.
	if s.audit != nil && time.Since(result.CreatedAt) < signupFreshness {
		s.audit.Emit(c, audit.Event{
			TenantID:       input.TenantID,
			StoreID:        input.StoreID,
			ForceActorType: audit.ActorSystem,
			Action:         "customer.signed_up",
			ResourceType:   "customer",
			ResourceID:     result.ID.String(),
			Metadata: map[string]any{
				"email":  email,
				"source": "storefront",
			},
		})
	}

	return result, nil
}

func nilIfEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
