// Package usermfa implements TOTP-based two-factor auth enrolment
// and verification. Secrets are stored encrypted at rest via AES-GCM
// using the same SESSION_ENCRYPT_KEY the session cookie manager
// consumes. The MFA check itself is not yet enforced at login —
// this package only handles the enrol → verify → disable lifecycle
// so the admin Account Settings UI can turn it on and off.
package usermfa

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"io"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// Issuer is the string shown in the authenticator app next to the
// account. Hardcoded — every TOTP entry should read "Mark8ly".
const Issuer = "Mark8ly"

// ─── Errors ──────────────────────────────────────────────────────────

var (
	ErrNotEnrolled      = errors.New("usermfa: no enrollment row for user")
	ErrInvalidCode      = errors.New("usermfa: invalid verification code")
	ErrAlreadyEnabled   = errors.New("usermfa: already enabled")
	ErrEncryptKeyWrong  = errors.New("usermfa: encrypt key must be 16, 24, or 32 bytes")
	ErrCipherCorruption = errors.New("usermfa: stored secret is corrupted")
)

// ─── Crypto ──────────────────────────────────────────────────────────

// Crypter envelope-encrypts TOTP secrets with AES-GCM. The same key
// that encrypts the session cookie is reused here — reusing the key
// is safe because GCM nonce-uniqueness is per-(key, ciphertext) and
// we generate a fresh random nonce per encrypt call.
type Crypter struct {
	gcm cipher.AEAD
}

// NewCrypter builds a Crypter from the raw AES key bytes.
func NewCrypter(key string) (*Crypter, error) {
	raw := []byte(key)
	switch len(raw) {
	case 16, 24, 32:
	default:
		return nil, ErrEncryptKeyWrong
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("usermfa: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("usermfa: gcm: %w", err)
	}
	return &Crypter{gcm: gcm}, nil
}

// Encrypt seals plaintext as nonce || ciphertext. The nonce is prefixed
// so Decrypt can split without extra metadata.
func (c *Crypter) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("usermfa: rand: %w", err)
	}
	sealed := c.gcm.Seal(nil, nonce, plaintext, nil)
	return append(nonce, sealed...), nil
}

// Decrypt undoes Encrypt. Returns ErrCipherCorruption on any tamper.
func (c *Crypter) Decrypt(blob []byte) ([]byte, error) {
	ns := c.gcm.NonceSize()
	if len(blob) < ns {
		return nil, ErrCipherCorruption
	}
	nonce, ct := blob[:ns], blob[ns:]
	plain, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrCipherCorruption
	}
	return plain, nil
}

// ─── Repository ──────────────────────────────────────────────────────

type enrollment struct {
	UserID     string
	SecretEnc  []byte
	Enabled    bool
	VerifiedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Repository is the DB layer for user_mfa.
type Repository struct {
	db *sql.DB
}

// NewRepository wraps a sql.DB handle. Safe to call with nil — every
// public method short-circuits when the underlying db is nil.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) get(ctx context.Context, userID string) (*enrollment, error) {
	if r == nil || r.db == nil {
		return nil, sql.ErrNoRows
	}
	const q = `
		SELECT user_id, secret_enc, enabled, verified_at, created_at, updated_at
		FROM user_mfa WHERE user_id = $1
	`
	var e enrollment
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(
		&e.UserID, &e.SecretEnc, &e.Enabled, &e.VerifiedAt, &e.CreatedAt, &e.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &e, nil
}

// upsertSecret stores a fresh (unverified) secret. If a row already
// exists for the user it's replaced — this path is used when a user
// restarts enrollment before verifying the prior attempt.
func (r *Repository) upsertSecret(ctx context.Context, userID string, enc []byte) error {
	if r == nil || r.db == nil {
		return nil
	}
	const q = `
		INSERT INTO user_mfa (user_id, secret_enc, enabled, verified_at, updated_at)
		VALUES ($1, $2, FALSE, NULL, NOW())
		ON CONFLICT (user_id) DO UPDATE
		SET secret_enc = EXCLUDED.secret_enc,
		    enabled = FALSE,
		    verified_at = NULL,
		    updated_at = NOW()
	`
	_, err := r.db.ExecContext(ctx, q, userID, enc)
	return err
}

func (r *Repository) markEnabled(ctx context.Context, userID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	const q = `
		UPDATE user_mfa
		SET enabled = TRUE, verified_at = NOW(), updated_at = NOW()
		WHERE user_id = $1
	`
	res, err := r.db.ExecContext(ctx, q, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotEnrolled
	}
	return nil
}

func (r *Repository) delete(ctx context.Context, userID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_mfa WHERE user_id = $1`, userID)
	return err
}

// ─── Service ─────────────────────────────────────────────────────────

// Service is the business layer for MFA enrolment.
type Service struct {
	repo    *Repository
	crypter *Crypter
}

// NewService constructs a Service. Returns an error when the key is
// the wrong length — main.go should treat that as fatal since MFA
// can't safely degrade to plaintext storage.
func NewService(db *sql.DB, encryptKey string) (*Service, error) {
	cr, err := NewCrypter(encryptKey)
	if err != nil {
		return nil, err
	}
	return &Service{repo: NewRepository(db), crypter: cr}, nil
}

// Status represents the current MFA state for a user.
type Status struct {
	Enabled  bool
	Verified bool
}

// GetStatus returns whether the user has enabled MFA. A missing row
// is treated as "not enabled" — same UX whether the user has never
// enrolled or previously disabled.
func (s *Service) GetStatus(ctx context.Context, userID string) (Status, error) {
	if s == nil {
		return Status{}, nil
	}
	e, err := s.repo.get(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Status{}, nil
		}
		return Status{}, err
	}
	return Status{Enabled: e.Enabled, Verified: e.VerifiedAt != nil}, nil
}

// IsEnabled is a convenience over GetStatus for callers that only
// need the boolean — the autologin MFA gate, specifically. Any error
// collapses into (false, err) so the gate can decide whether to
// treat the user as not-enrolled under a transient DB blip.
func (s *Service) IsEnabled(ctx context.Context, userID string) (bool, error) {
	st, err := s.GetStatus(ctx, userID)
	if err != nil {
		return false, err
	}
	return st.Enabled, nil
}

// EnrollResult is what Enroll returns to the caller. QRDataURL is a
// ready-to-embed `data:image/png;base64,...` string the admin UI can
// drop straight into an <img src>. OtpauthURL is the raw URI in case
// a client wants to render its own QR.
type EnrollResult struct {
	QRDataURL   string
	OtpauthURL  string
	Secret      string // base32 — shown as a fallback for manual entry
}

// Enroll generates a fresh TOTP secret for the user, stores it
// encrypted (enabled=false), and returns the QR + otpauth URI. Safe
// to call repeatedly — each call overwrites the prior unverified
// secret, which also invalidates any in-progress QR the user may
// have mid-scan.
func (s *Service) Enroll(ctx context.Context, userID, accountName string) (*EnrollResult, error) {
	if s == nil {
		return nil, errors.New("usermfa: service not configured")
	}
	// If already enabled, refuse — re-enrollment requires Disable first.
	if st, err := s.GetStatus(ctx, userID); err == nil && st.Enabled {
		return nil, ErrAlreadyEnabled
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      Issuer,
		AccountName: accountName,
		SecretSize:  20,
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
	})
	if err != nil {
		return nil, fmt.Errorf("usermfa: generate: %w", err)
	}

	enc, err := s.crypter.Encrypt([]byte(key.Secret()))
	if err != nil {
		return nil, fmt.Errorf("usermfa: encrypt: %w", err)
	}
	if err := s.repo.upsertSecret(ctx, userID, enc); err != nil {
		return nil, fmt.Errorf("usermfa: upsert: %w", err)
	}

	// Render QR as PNG → base64 → data URL. 256×256 is large enough
	// for every authenticator scanner without being heavy on the wire.
	img, err := key.Image(256, 256)
	if err != nil {
		return nil, fmt.Errorf("usermfa: qr image: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("usermfa: qr encode: %w", err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	return &EnrollResult{
		QRDataURL:  dataURL,
		OtpauthURL: key.URL(),
		Secret:     key.Secret(),
	}, nil
}

// Verify checks a 6-digit code against the stored secret and, on match,
// flips enabled=true. Safe to call repeatedly — only the secret in the
// last Enroll call counts.
func (s *Service) Verify(ctx context.Context, userID, code string) error {
	if s == nil {
		return errors.New("usermfa: service not configured")
	}
	e, err := s.repo.get(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotEnrolled
		}
		return fmt.Errorf("usermfa: get: %w", err)
	}
	secret, err := s.crypter.Decrypt(e.SecretEnc)
	if err != nil {
		return err
	}
	if !totp.Validate(code, string(secret)) {
		return ErrInvalidCode
	}
	return s.repo.markEnabled(ctx, userID)
}

// Disable removes the enrollment. Keeps the user in a "not enrolled"
// state — a subsequent Enroll starts fresh with a new secret.
func (s *Service) Disable(ctx context.Context, userID string) error {
	if s == nil {
		return nil
	}
	return s.repo.delete(ctx, userID)
}
