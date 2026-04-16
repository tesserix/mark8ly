package customer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// EnsureProfileInput is the input for profile auto-creation.
type EnsureProfileInput struct {
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

// signupFreshness is how recently a profile must have been created for
// EnsureProfile to treat the upsert as a fresh signup. The repo reload
// happens within milliseconds of the insert, so 2s comfortably covers
// jitter without false-positive-ing on rapid re-logins.
const signupFreshness = 2 * time.Second

// EnsureProfile upserts a customer profile on first authenticated request.
// Uses INSERT ON CONFLICT (store_id, email) DO UPDATE SET gip_uid = EXCLUDED.gip_uid
// so concurrent first-logins are safe.
//
// Returns the existing or newly created profile. When called from a
// gin handler (c may be nil for non-HTTP callers) and the upsert
// inserted a new row, emits a customer.signed_up audit event.
func (s *Service) EnsureProfile(ctx context.Context, input EnsureProfileInput, c *gin.Context) (*CustomerProfile, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" {
		return nil, fmt.Errorf("customer: email is required for profile auto-creation")
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
		return nil, fmt.Errorf("customer: ensure profile: %w", err)
	}

	s.logger.Info("customer profile ensured",
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
