package verification

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/mark8ly/platform-api/internal/notification"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Service is the business logic for the email-magic-link verification flow.
//
// Two operations:
//
//   - SendMagicLink: invalidate any outstanding tokens for the session,
//     generate a fresh URL-safe token, persist its SHA-256 hash, send the
//     verification email containing a clickable link, return.
//
//   - VerifyToken: look up the token by its SHA-256 hash, check expiry,
//     mark consumed, return the (sessionID, email) tuple so the caller
//     can complete onboarding.
//
// Why SHA-256 instead of bcrypt:
//
//   The token is 32 bytes from crypto/rand → 256 bits of entropy. There is
//   no brute-force vector because the input space is too large for any
//   adversary to enumerate. Bcrypt's slow-by-design property exists to
//   protect LOW-entropy inputs (passwords); for HIGH-entropy random tokens
//   it's the wrong tool. SHA-256 also lets us index the column directly
//   so we can look the token up by hash without scanning every row.
//
//   Same approach used by: Stripe webhook secrets, GitHub PATs, AWS API
//   keys, Slack webhook URLs.
type Service struct {
	repo         Repository
	sender       notification.Sender
	emailFrom    string
	supportEmail string
	verifyURLBase string
}

// Config holds Service dependencies.
type Config struct {
	Sender       notification.Sender
	EmailFrom    string
	SupportEmail string
	// VerifyURLBase is prefixed onto the magic link token to form the
	// clickable URL in the email body. e.g. "https://onboarding.mark8ly.com"
	// → magic link becomes "https://onboarding.mark8ly.com/onboarding/verify?token=<tok>".
	VerifyURLBase string
}

// NewService constructs a Service.
func NewService(repo Repository, cfg Config) *Service {
	if cfg.SupportEmail == "" {
		cfg.SupportEmail = cfg.EmailFrom
	}
	if cfg.VerifyURLBase == "" {
		cfg.VerifyURLBase = "http://localhost:4201"
	}
	return &Service{
		repo:          repo,
		sender:        cfg.Sender,
		emailFrom:     cfg.EmailFrom,
		supportEmail:  cfg.SupportEmail,
		verifyURLBase: cfg.VerifyURLBase,
	}
}

// SendMagicLink generates a fresh verification token for the session and
// emails the user a clickable link.
//
// Calling SendMagicLink invalidates any previously-issued, non-consumed
// tokens for the same session — only the latest link is valid.
func (s *Service) SendMagicLink(ctx context.Context, sessionID, email, businessName string) error {
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

	token, hash, err := generateToken()
	if err != nil {
		return apperrors.Wrap(err, 500, "token_generation_failed", "failed to generate verification token")
	}

	tok := &Token{
		SessionID: sessionID,
		Email:     email,
		// Reusing the existing CodeHash column. The data shape is identical
		// (a fixed-length hash of secret material); only the hashing scheme
		// changed. A future cleanup migration could rename it to TokenHash.
		CodeHash:    hash,
		Purpose:     PurposeEmailVerification,
		MaxAttempts: 0, // not used for magic link
		ExpiresAt:   time.Now().Add(TokenLifetime),
	}
	if err := s.repo.Create(ctx, tok); err != nil {
		return err
	}

	verifyURL := s.verifyURLBase + "/onboarding/verify?token=" + token

	msg, err := notification.RenderEmailVerification(email, s.emailFrom, notification.EmailVerificationVars{
		BusinessName: businessName,
		VerifyURL:    verifyURL,
		ExpiresIn:    "24 hours",
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

// VerifyResult is what VerifyToken returns to the caller on success.
// SessionID lets the onboarding handler look up the draft and complete it.
type VerifyResult struct {
	SessionID string
	Email     string
}

// VerifyToken validates a magic link token. On success the token is marked
// consumed and the (sessionID, email) tuple is returned so the caller can
// complete onboarding.
//
// Failure modes (all returned as AppErrors):
//   - token unknown (NotFound)
//   - token expired (BadRequest "token_expired")
//   - token already consumed (BadRequest "token_consumed")
func (s *Service) VerifyToken(ctx context.Context, token string) (*VerifyResult, error) {
	if token == "" {
		return nil, apperrors.BadRequest("invalid_token", "token is required")
	}

	hash := hashToken(token)
	tok, err := s.repo.GetByHash(ctx, hash)
	if err != nil {
		return nil, err
	}

	if tok.ConsumedAt != nil {
		return nil, apperrors.BadRequest("token_consumed", "this verification link has already been used")
	}
	if time.Now().After(tok.ExpiresAt) {
		return nil, apperrors.BadRequest("token_expired", "this verification link has expired; please request a new one")
	}

	if err := s.repo.MarkConsumed(ctx, tok.ID); err != nil {
		return nil, err
	}

	return &VerifyResult{
		SessionID: tok.SessionID,
		Email:     tok.Email,
	}, nil
}

// generateToken returns a (token, hash) pair where:
//   - token is 32 random bytes base64url-encoded (URL-safe, ~43 chars)
//   - hash is the SHA-256 of the token, hex-encoded for DB storage
func generateToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	return token, hashToken(token), nil
}

// hashToken returns the lowercase hex-encoded SHA-256 of the token.
// Deterministic — same input always produces the same output, which is
// what lets us look up by hash without scanning every row.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
