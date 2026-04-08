package invitation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/notification"
	"github.com/mark8ly/platform-api/internal/tenant"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Service is the business-logic layer for invitations.
//
// It orchestrates: the FGA permission check, the token generation,
// the DB row, the email send, and — on accept — the FGA role write.
// Keeping all of it in one file is deliberate; the invitation lifecycle
// is small enough that the handler/service/repo split is the only
// layering that matters.
type Service struct {
	repo       Repository
	tenantRepo tenant.Repository
	fga        authz.Client
	sender     notification.Sender
	emailFrom  string
	acceptURL  func(slug, token string) string
	// expiry controls how long an invitation token is valid for.
	// 72 hours matches the legacy backup's `StaffInvitation`
	// default and is plenty of time for an inbox round-trip.
	expiry time.Duration
	// recorder captures plaintext tokens in dev/test so the e2e
	// suite can bypass the inbox. nil in prod.
	recorder TokenRecorder
}

// TokenRecorder is the narrow interface the service calls to publish
// plaintext tokens to the dev test helper. Kept generic so the
// existing verification.TokenRecorder can't be reused directly (its
// key is email-only and we need tenant+email to disambiguate).
type TokenRecorder interface {
	Record(tenantID, email, token string)
}

// Config holds Service dependencies.
type Config struct {
	Repo       Repository
	TenantRepo tenant.Repository
	FGA        authz.Client
	Sender     notification.Sender
	EmailFrom  string
	// AcceptURL builds the full invitation accept URL from a tenant
	// slug and plaintext token. In dev this is e.g.
	//   http://localhost:4202/accept-invite?token=...
	// In prod it becomes
	//   https://<slug>-admin.mark8ly.com/accept-invite?token=...
	AcceptURL func(slug, token string) string
	// Expiry defaults to 72h when zero.
	Expiry   time.Duration
	Recorder TokenRecorder
}

// NewService constructs a Service.
func NewService(cfg Config) *Service {
	if cfg.Expiry == 0 {
		cfg.Expiry = 72 * time.Hour
	}
	return &Service{
		repo:       cfg.Repo,
		tenantRepo: cfg.TenantRepo,
		fga:        cfg.FGA,
		sender:     cfg.Sender,
		emailFrom:  cfg.EmailFrom,
		acceptURL:  cfg.AcceptURL,
		expiry:     cfg.Expiry,
		recorder:   cfg.Recorder,
	}
}

// CreateInput is the payload for a new invitation.
type CreateInput struct {
	TenantID        string
	Email           string
	Role            string
	InvitedByUserID string
}

// Create writes a new pending invitation, sends the email, and
// returns the row. Runs the FGA `can_invite_members` check against
// the inviting user.
func (s *Service) Create(ctx context.Context, in CreateInput) (*Invitation, error) {
	tenantID := strings.TrimSpace(in.TenantID)
	email := strings.ToLower(strings.TrimSpace(in.Email))
	role := strings.TrimSpace(in.Role)
	uid := strings.TrimSpace(in.InvitedByUserID)

	if tenantID == "" {
		return nil, apperrors.BadRequest("invalid_tenant_id", "tenant id is required")
	}
	if email == "" || !strings.Contains(email, "@") {
		return nil, apperrors.BadRequest("invalid_email", "a valid email is required")
	}
	if !isAllowedRole(role) {
		return nil, apperrors.BadRequest("invalid_role", "role must be admin, staff, or viewer")
	}
	if uid == "" {
		return nil, apperrors.BadRequest("invalid_inviter", "invited_by_user_id is required")
	}

	// Authorize the invite.
	if s.fga != nil {
		allowed, err := s.fga.Check(ctx, uid, "can_invite_members", tenantID)
		if err != nil {
			return nil, apperrors.Internal("authz_check_failed", "authorization check failed")
		}
		if !allowed {
			return nil, apperrors.New(403, "forbidden", "you do not have permission to invite members")
		}
	}

	// Look up the tenant so we can put its name in the email.
	t, err := s.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	token, hash, err := newToken()
	if err != nil {
		return nil, apperrors.Wrap(err, 500, "token_generation_failed", "failed to generate invitation token")
	}

	inv := &Invitation{
		TenantID:        tenantID,
		Email:           email,
		Role:            role,
		TokenHash:       hash,
		ExpiresAt:       time.Now().Add(s.expiry),
		Status:          StatusPending,
		InvitedByUserID: uid,
	}
	if err := s.repo.Create(ctx, inv); err != nil {
		return nil, err
	}

	// Record the plaintext token for the dev e2e helper BEFORE
	// sending — if the sender blows up we still want the e2e to
	// be able to fetch the token.
	if s.recorder != nil {
		s.recorder.Record(tenantID, email, token)
	}

	// Send the email. Non-fatal: a failed send leaves the row
	// pending and the user can retry via "resend" (future slice).
	if s.sender != nil {
		url := s.acceptURL(t.Slug, token)
		subject := "You've been invited to join " + t.Name + " on Mark8ly"
		body := inviteEmailBody(t.Name, role, url)
		_ = s.sender.Send(ctx, notification.Email{
			From:     s.emailFrom,
			To:       email,
			Subject:  subject,
			HTMLBody: body,
			TextBody: inviteEmailText(t.Name, role, url),
		})
	}

	return inv, nil
}

// Verify looks up an invitation by its plaintext token, validates
// that it's still pending and unexpired, and returns a sanitised
// summary for the public accept page. Never returns the token.
type VerifyResult struct {
	InvitationID string    `json:"invitation_id"`
	TenantID     string    `json:"tenant_id"`
	TenantName   string    `json:"tenant_name"`
	TenantSlug   string    `json:"tenant_slug"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Verify returns the public view of an invitation or a specific
// error code the accept page surfaces to the user.
func (s *Service) Verify(ctx context.Context, plainToken string) (*VerifyResult, error) {
	plainToken = strings.TrimSpace(plainToken)
	if plainToken == "" {
		return nil, apperrors.BadRequest("invalid_token", "token is required")
	}
	hash := hashToken(plainToken)
	inv, err := s.repo.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if inv.Status != StatusPending {
		return nil, apperrors.Conflict("invitation_"+inv.Status, "this invitation is "+inv.Status)
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, apperrors.Conflict("invitation_expired", "this invitation has expired")
	}
	t, err := s.tenantRepo.GetByID(ctx, inv.TenantID)
	if err != nil {
		return nil, err
	}
	return &VerifyResult{
		InvitationID: inv.ID,
		TenantID:     inv.TenantID,
		TenantName:   t.Name,
		TenantSlug:   t.Slug,
		Email:        inv.Email,
		Role:         inv.Role,
		ExpiresAt:    inv.ExpiresAt,
	}, nil
}

// AcceptInput is the payload for accepting an invitation.
type AcceptInput struct {
	Token string
	// UID is the GIP UID of the accepting user. The caller
	// (admin BFF) has verified the GIP id_token and extracted
	// the uid + verified email.
	UID string
	// VerifiedEmail is the GIP token's verified email claim.
	// Must match the invitation's email (case-insensitive).
	VerifiedEmail string
}

// AcceptResult is returned on successful accept so the admin BFF
// can call /auth/switch-tenant with the new tenant id.
type AcceptResult struct {
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
}

// Accept validates the token, writes the FGA role tuple, and marks
// the invitation accepted. Strictly enforces email match to prevent
// bearer-token abuse.
func (s *Service) Accept(ctx context.Context, in AcceptInput) (*AcceptResult, error) {
	token := strings.TrimSpace(in.Token)
	uid := strings.TrimSpace(in.UID)
	verifiedEmail := strings.ToLower(strings.TrimSpace(in.VerifiedEmail))
	if token == "" || uid == "" || verifiedEmail == "" {
		return nil, apperrors.BadRequest("invalid_input", "token, uid, and verified_email are required")
	}

	inv, err := s.repo.GetByTokenHash(ctx, hashToken(token))
	if err != nil {
		return nil, err
	}
	if inv.Status != StatusPending {
		return nil, apperrors.Conflict("invitation_"+inv.Status, "this invitation is "+inv.Status)
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, apperrors.Conflict("invitation_expired", "this invitation has expired")
	}

	// Strict email match. Prevents a stolen link from being used
	// by anyone other than the original invitee.
	if !strings.EqualFold(inv.Email, verifiedEmail) {
		return nil, apperrors.New(
			403,
			"email_mismatch",
			"this invitation was sent to a different email address",
		)
	}

	// Write the FGA role tuple. Degraded mode (fga nil) skips
	// the write — dev-only, never prod.
	if s.fga != nil {
		role := authz.Role(inv.Role)
		if err := s.fga.WriteRole(ctx, uid, role, inv.TenantID); err != nil {
			return nil, apperrors.Wrap(err, 500, "authz_write_failed", "failed to grant role")
		}
	}

	if err := s.repo.MarkAccepted(ctx, inv.ID); err != nil {
		return nil, err
	}
	return &AcceptResult{
		TenantID: inv.TenantID,
		Role:     inv.Role,
	}, nil
}

// ListPending returns the tenant's pending invitations for the
// /settings/team table. Gated on can_view_settings by the handler.
func (s *Service) ListPending(ctx context.Context, tenantID string) ([]Invitation, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, apperrors.BadRequest("invalid_tenant_id", "tenant id is required")
	}
	return s.repo.ListPendingByTenant(ctx, tenantID)
}

// Revoke marks a pending invitation as revoked. Gated on
// can_invite_members by the handler (can't invite = can't revoke).
func (s *Service) Revoke(ctx context.Context, tenantID, invitationID, uid string) error {
	tenantID = strings.TrimSpace(tenantID)
	invitationID = strings.TrimSpace(invitationID)
	uid = strings.TrimSpace(uid)
	if tenantID == "" || invitationID == "" || uid == "" {
		return apperrors.BadRequest("invalid_input", "tenant_id, invitation_id and uid are required")
	}

	if s.fga != nil {
		allowed, err := s.fga.Check(ctx, uid, "can_invite_members", tenantID)
		if err != nil {
			return apperrors.Internal("authz_check_failed", "authorization check failed")
		}
		if !allowed {
			return apperrors.New(403, "forbidden", "you do not have permission to revoke invitations")
		}
	}

	inv, err := s.repo.GetByID(ctx, invitationID)
	if err != nil {
		return err
	}
	if inv.TenantID != tenantID {
		return apperrors.NotFound("invitation_not_found", "no invitation with that id")
	}
	return s.repo.MarkRevoked(ctx, invitationID)
}

// newToken generates a fresh invitation token and returns
// (plaintext, sha256_hex).
func newToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	plain := hex.EncodeToString(buf)
	return plain, hashToken(plain), nil
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func inviteEmailBody(tenantName, role, url string) string {
	return `<p>You've been invited to join <strong>` + tenantName +
		`</strong> on Mark8ly as a <strong>` + role + `</strong>.</p>` +
		`<p><a href="` + url + `">Accept the invitation</a></p>` +
		`<p>This link expires in 72 hours.</p>`
}

func inviteEmailText(tenantName, role, url string) string {
	return "You've been invited to join " + tenantName +
		" on Mark8ly as a " + role + ".\n\n" +
		"Accept the invitation: " + url + "\n\n" +
		"This link expires in 72 hours."
}
