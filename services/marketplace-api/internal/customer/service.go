package customer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
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
	logger *slog.Logger
}

// NewService constructs a Service.
func NewService(db *gorm.DB, repo Repository, logger *slog.Logger) *Service {
	return &Service{db: db, repo: repo, logger: logger}
}

// EnsureProfile upserts a customer profile on first authenticated request.
// Uses INSERT ON CONFLICT (store_id, email) DO UPDATE SET gip_uid = EXCLUDED.gip_uid
// so concurrent first-logins are safe.
//
// Returns the existing or newly created profile.
func (s *Service) EnsureProfile(ctx context.Context, input EnsureProfileInput) (*CustomerProfile, error) {
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
	return result, nil
}

func nilIfEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
