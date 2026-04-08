package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/notification"
	"github.com/mark8ly/platform-api/internal/outbox"
	"github.com/mark8ly/platform-api/internal/store"
	"github.com/mark8ly/platform-api/internal/tenant"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Service is the business logic for the onboarding flow.
//
// The Complete() method is the bug-fix landing point for auth-bug #2 (FGA
// tuple race) and #3 (no retry on FGA failure) from
// docs/planning/auth-bugs.md. The fix has three parts:
//
//  1. The DB transaction creates BOTH the tenant row AND the outbox row
//     describing the FGA tuple writes. Either both commit or neither does.
//  2. The outbox drainer (separate goroutine) ships the tuple to OpenFGA
//     with retries. If FGA is down, the row stays pending and is retried.
//  3. (Out of this package's scope) auth-bff's auto-login does an FGA
//     Check with retry-on-not-found, so the user can't reach admin until
//     the tuple is actually visible.
//
// Together these close the race window from both ends.
type Service struct {
	db                    *gorm.DB
	repo                  Repository
	tenantRepo            tenant.Repository
	storeRepo             store.Repository
	sender                notification.Sender
	emailFrom             string
	supportSite           string
	adminURLTemplate      string
	storefrontURLTemplate string
}

// Config holds Service dependencies.
type Config struct {
	DB         *gorm.DB
	Repo       Repository
	TenantRepo tenant.Repository
	StoreRepo  store.Repository
	Sender     notification.Sender
	EmailFrom  string
	// AdminURLTemplate is a template like "https://%s-admin.mark8ly.com"
	// for building the welcome email's admin link. Supports both the
	// per-slug shape (contains %s) and flat-host shape (no %s → used
	// as-is, for dev on localhost).
	AdminURLTemplate      string
	StorefrontURLTemplate string
	SupportEmail          string
}

// NewService constructs a Service.
func NewService(cfg Config) *Service {
	return &Service{
		db:                    cfg.DB,
		repo:                  cfg.Repo,
		tenantRepo:            cfg.TenantRepo,
		storeRepo:             cfg.StoreRepo,
		sender:                cfg.Sender,
		emailFrom:             cfg.EmailFrom,
		supportSite:           cfg.SupportEmail,
		adminURLTemplate:      cfg.AdminURLTemplate,
		storefrontURLTemplate: cfg.StorefrontURLTemplate,
	}
}

// CreateRequest is the input to Create.
type CreateRequest struct {
	Email string `json:"email"`
}

// Create starts a new onboarding session for the given email. Idempotent
// in spirit (same email can start multiple sessions if previous ones were
// abandoned), but each call always returns a fresh session ID.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Session, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, apperrors.BadRequest("invalid_email", "valid email is required")
	}

	sess := &Session{
		Email:  email,
		Draft:  json.RawMessage(`{}`),
		Status: StatusInProgress,
	}
	if err := s.repo.Create(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// Get returns a session by ID.
func (s *Service) Get(ctx context.Context, id string) (*Session, error) {
	return s.repo.GetByID(ctx, id)
}

// SaveDraft replaces the session's draft JSON. The draft is opaque to the
// backend — the wizard's frontend owns the schema. We only validate that
// it parses as JSON.
func (s *Service) SaveDraft(ctx context.Context, id string, draft json.RawMessage) error {
	if !json.Valid(draft) {
		return apperrors.BadRequest("invalid_draft", "draft must be valid JSON")
	}
	return s.repo.UpdateDraft(ctx, id, draft)
}

// MarkVerified is called by the verification service after a successful
// OTP verification. Idempotent.
func (s *Service) MarkVerified(ctx context.Context, sessionID string) error {
	return s.repo.MarkEmailVerified(ctx, sessionID)
}

// CompleteRequest is the input to Complete. It contains the merchant's
// final answers — everything needed to mint a tenant.
type CompleteRequest struct {
	SessionID    string `json:"session_id"`
	BusinessName string `json:"business_name"`
	Slug         string `json:"slug"`
	OwnerUserID  string `json:"owner_user_id"` // GIP UID
	OwnerEmail   string `json:"owner_email"`
	CountryCode  string `json:"country_code"`
	CurrencyCode string `json:"currency_code"`
	Timezone     string `json:"timezone"`
}

// CompleteResult is what Complete returns on success.
type CompleteResult struct {
	TenantID string `json:"tenant_id"`
	Slug     string `json:"slug"`
}

// fgaWritePayload is the outbox event payload for an FGA tuple write.
// Stored in outbox_events.payload as JSONB; consumed by the registered
// handler in cmd/server.
//
// Phase Q added StoreID: the drainer also writes the
// `store:<id> parent tenant:<id>` tuple so FGA's store-type
// `from parent` inheritance works for the default store created
// during onboarding.
type fgaWritePayload struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	StoreID  string `json:"store_id,omitempty"`
}

// FGAOutboxKind is the dispatch key for FGA membership writes.
const FGAOutboxKind = "fga.write_membership"

// Complete is the bug-fix transaction.
//
// In one DB transaction:
//
//  1. Verify the session is in a completable state (verified email)
//  2. Create the tenant row
//  3. Mark the session completed and link tenant_id
//  4. Enqueue an outbox event for the FGA membership write
//
// All four happen-or-nothing. The outbox drainer picks up the event and
// writes the OpenFGA tuple. If the drainer is delayed or FGA is down,
// the membership tuple is eventually written; the user just sees auth-bff
// retry on auto-login until the tuple appears.
//
// After commit (NOT in the tx) we send the welcome email best-effort.
// Failure to send is logged but does not fail completion.
func (s *Service) Complete(ctx context.Context, req CompleteRequest) (*CompleteResult, error) {
	if err := validateCompleteRequest(req); err != nil {
		return nil, err
	}

	// Snapshot the session before the tx — so we can email the user even
	// if we don't need the row data inside the tx.
	sess, err := s.repo.GetByID(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if sess.Status == StatusCompleted {
		return nil, apperrors.Conflict("already_completed", "session is already completed")
	}
	if sess.EmailVerifiedAt == nil {
		return nil, apperrors.BadRequest("email_not_verified", "email must be verified before completion")
	}

	// Phase Q: onboarding creates BOTH a tenant (the company) and a
	// default store (the first storefront) in the same transaction.
	// The merchant's "business name" becomes both the tenant.name
	// (company) and the store.name ("Main Store" label is reserved
	// for backfilled legacy tenants only). The store row carries the
	// slug + currency + timezone + country — the user-facing
	// storefront identity.
	t := &tenant.Tenant{
		Name:        req.BusinessName,
		OwnerUserID: req.OwnerUserID,
		OwnerEmail:  req.OwnerEmail,
		Status:      tenant.StatusActive,
	}
	st := &store.Store{
		Slug:            req.Slug,
		Name:            req.BusinessName,
		CountryCode:     req.CountryCode,
		CurrencyCode:    req.CurrencyCode,
		Timezone:        req.Timezone,
		StorefrontTheme: json.RawMessage(`{}`),
		Status:          store.StatusActive,
	}

	// THE BUG-FIX TRANSACTION (Phase D) extended with the Phase Q
	// store creation. Either every row commits together or nothing
	// does — if the store insert fails on a slug collision, the
	// tenant row is rolled back along with it.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.tenantRepo.CreateInTx(ctx, tx, t); err != nil {
			return err
		}
		st.TenantID = t.ID
		if err := s.storeRepo.CreateInTx(ctx, tx, st); err != nil {
			return err
		}
		if err := s.repo.CompleteInTx(ctx, tx, req.SessionID, t.ID); err != nil {
			return err
		}
		// Phase D outbox event — owner tuple on the tenant. Store-
		// level tuples come with Phase R (store-level invites).
		if err := outbox.Enqueue(tx, FGAOutboxKind, fgaWritePayload{
			UserID:   req.OwnerUserID,
			TenantID: t.ID,
			StoreID:  st.ID,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if s.sender != nil {
		_ = s.sendWelcome(ctx, t, st, req)
	}

	return &CompleteResult{TenantID: t.ID, Slug: st.Slug}, nil
}

func (s *Service) sendWelcome(ctx context.Context, t *tenant.Tenant, st *store.Store, req CompleteRequest) error {
	msg, err := notification.RenderWelcome(t.OwnerEmail, s.emailFrom, notification.WelcomeVars{
		BusinessName:  t.Name,
		AdminURL:      formatURLTemplate(s.adminURLTemplate, st.Slug),
		StorefrontURL: formatURLTemplate(s.storefrontURLTemplate, st.Slug),
		SupportEmail:  s.supportSite,
	})
	if err != nil {
		return err
	}
	return s.sender.Send(ctx, msg)
}

// formatURLTemplate substitutes %s with slug if present, otherwise
// returns the template as-is. Lets ops pick flat-host (dev) or
// per-slug (prod) URL shapes via config without a code change.
func formatURLTemplate(template, slug string) string {
	if strings.Contains(template, "%s") {
		return fmt.Sprintf(template, slug)
	}
	return template
}

// validateCompleteRequest enforces presence + shape of every required field.
func validateCompleteRequest(req CompleteRequest) error {
	if req.SessionID == "" {
		return apperrors.BadRequest("invalid_session", "session_id is required")
	}
	if strings.TrimSpace(req.BusinessName) == "" {
		return apperrors.BadRequest("invalid_business_name", "business_name is required")
	}
	if req.Slug == "" {
		return apperrors.BadRequest("invalid_slug", "slug is required")
	}
	if req.OwnerUserID == "" {
		return apperrors.BadRequest("invalid_owner", "owner_user_id is required")
	}
	if req.OwnerEmail == "" || !strings.Contains(req.OwnerEmail, "@") {
		return apperrors.BadRequest("invalid_owner_email", "owner_email is required and must be valid")
	}
	if len(req.CountryCode) != 2 {
		return apperrors.BadRequest("invalid_country_code", "country_code must be a 2-letter ISO code")
	}
	if len(req.CurrencyCode) != 3 {
		return apperrors.BadRequest("invalid_currency_code", "currency_code must be a 3-letter ISO 4217 code")
	}
	if req.Timezone == "" {
		return apperrors.BadRequest("invalid_timezone", "timezone is required")
	}
	return nil
}
