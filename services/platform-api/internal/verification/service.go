package verification

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mark8ly/platform-api/internal/notification"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Service is the business logic for the email-OTP verification flow.
//
// Two operations:
//   - SendCode: invalidate any outstanding tokens for the session, generate
//     a new 6-digit code, persist its bcrypt hash, send the code via the
//     notification.Sender, return.
//   - Verify: look up the active token for the session, check expiry +
//     attempts + bcrypt match, on success mark consumed and return ok.
type Service struct {
	repo         Repository
	sender       notification.Sender
	emailFrom    string
	supportEmail string
}

// Config holds Service dependencies.
type Config struct {
	Sender       notification.Sender
	EmailFrom    string
	SupportEmail string
}

// NewService constructs a Service.
func NewService(repo Repository, cfg Config) *Service {
	if cfg.SupportEmail == "" {
		cfg.SupportEmail = cfg.EmailFrom
	}
	return &Service{
		repo:         repo,
		sender:       cfg.Sender,
		emailFrom:    cfg.EmailFrom,
		supportEmail: cfg.SupportEmail,
	}
}

// SendCode generates a fresh OTP for the session and emails it to the user.
//
// Calling SendCode invalidates any previously-issued, non-consumed tokens
// for the same session — only the latest code is valid. This prevents
// "stale token accumulation" where multiple resends leave several tokens
// active, each independently brute-forceable.
func (s *Service) SendCode(ctx context.Context, sessionID, email, businessName string) error {
	if sessionID == "" {
		return apperrors.BadRequest("invalid_session", "session id is required")
	}
	if email == "" {
		return apperrors.BadRequest("invalid_email", "email is required")
	}

	// Invalidate any outstanding tokens before issuing a fresh one.
	if err := s.repo.InvalidateForSession(ctx, sessionID); err != nil {
		return err
	}

	code, err := generateCode()
	if err != nil {
		return apperrors.Wrap(err, 500, "code_generation_failed", "failed to generate verification code")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return apperrors.Wrap(err, 500, "code_hash_failed", "failed to hash verification code")
	}

	tok := &Token{
		SessionID:   sessionID,
		Email:       email,
		CodeHash:    string(hash),
		Purpose:     PurposeEmailVerification,
		MaxAttempts: MaxAttemptsDefault,
		ExpiresAt:   time.Now().Add(TokenLifetime),
	}
	if err := s.repo.Create(ctx, tok); err != nil {
		return err
	}

	msg, err := notification.RenderEmailVerification(email, s.emailFrom, notification.EmailVerificationVars{
		BusinessName: businessName,
		OTPCode:      code,
		ExpiresIn:    "10 minutes",
		SupportEmail: s.supportEmail,
	})
	if err != nil {
		return apperrors.Wrap(err, 500, "email_render_failed", "failed to render verification email")
	}
	if err := s.sender.Send(ctx, msg); err != nil {
		return apperrors.Wrap(err, 500, "email_send_failed", "failed to send verification email")
	}
	return nil
}

// Verify checks the submitted code against the active token for the session.
// On success the token is marked consumed and ok is true.
//
// Failure modes (all returned as AppErrors):
//   - no active token (NotFound)
//   - token expired (BadRequest "expired")
//   - max attempts exceeded (BadRequest "too_many_attempts")
//   - code mismatch (BadRequest "invalid_code") — attempts is incremented
func (s *Service) Verify(ctx context.Context, sessionID, code string) error {
	if sessionID == "" {
		return apperrors.BadRequest("invalid_session", "session id is required")
	}
	if code == "" {
		return apperrors.BadRequest("invalid_code", "code is required")
	}

	tok, err := s.repo.LatestForSession(ctx, sessionID)
	if err != nil {
		return err
	}

	if time.Now().After(tok.ExpiresAt) {
		return apperrors.BadRequest("token_expired", "verification code has expired; request a new one")
	}
	if tok.Attempts >= tok.MaxAttempts {
		return apperrors.BadRequest("too_many_attempts", "too many attempts; request a new code")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(tok.CodeHash), []byte(code)); err != nil {
		// Mismatch: bump attempts so future requests eventually lock out.
		_ = s.repo.IncrementAttempts(ctx, tok.ID)
		return apperrors.BadRequest("invalid_code", "verification code is incorrect")
	}

	if err := s.repo.MarkConsumed(ctx, tok.ID); err != nil {
		return err
	}
	return nil
}

// generateCode returns a uniformly random 6-digit numeric code as a string.
// crypto/rand is required — math/rand would be predictable to an attacker.
func generateCode() (string, error) {
	const max = 1_000_000 // 6-digit space: 000000..999999
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
