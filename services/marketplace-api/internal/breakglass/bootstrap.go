package breakglass

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is pinned at 12 per §12.4. Higher would slow login past
// the 3s budget on low-tier db; lower would be a policy regression.
const BcryptCost = 12

// Bootstrapper provisions exactly one break-glass account per tenant.
// Secret Manager is written FIRST so a DB failure never leaves the
// password_hash referencing a non-existent blob.
type Bootstrapper struct {
	repo      *Repository
	secrets   *SecretManager
	projectID string
}

// NewBootstrapper returns a Bootstrapper.
func NewBootstrapper(repo *Repository, secrets *SecretManager, projectID string) *Bootstrapper {
	return &Bootstrapper{repo: repo, secrets: secrets, projectID: projectID}
}

// Provision creates the break-glass account for tenantID. Idempotent:
// a second call returns ErrAlreadyProvisioned rather than stacking
// two active secrets.
//
// The order is deliberate:
//  1. Generate password + TOTP + bcrypt(password).
//  2. Write Blob to Secret Manager at SecretPathFor(projectID, tenantID).
//  3. INSERT row. On PK conflict → ErrAlreadyProvisioned (safe: the
//     existing row was provisioned by someone else; Secret Manager
//     gets a new version that the existing hash won't validate, but
//     that only affects a duplicate re-provision, not the live row).
//
// If step 2 fails the DB is untouched; if step 3 fails we've left an
// orphan Secret Manager version — benign, and a follow-up rotate fixes
// it.
func (b *Bootstrapper) Provision(ctx context.Context, tenantID uuid.UUID) error {
	if tenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id required", ErrInvalidCredentials)
	}

	// Short-circuit if we already have a row — avoids burning a new
	// Secret Manager version on every retry.
	if _, err := b.repo.GetByTenant(ctx, tenantID); err == nil {
		return ErrAlreadyProvisioned
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}

	pw, err := GeneratePassword()
	if err != nil {
		return fmt.Errorf("generate password: %w", err)
	}
	totpSecret, err := GenerateTOTPSecret()
	if err != nil {
		return fmt.Errorf("generate totp secret: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), BcryptCost)
	if err != nil {
		return fmt.Errorf("bcrypt: %w", err)
	}

	secretPath := SecretPathFor(b.projectID, tenantID.String())
	if err := b.secrets.Upsert(ctx, secretPath, Blob{
		Password:    pw,
		TOTPSecret:  totpSecret,
		GeneratedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}

	return b.repo.Create(ctx, &Account{
		TenantID:      tenantID,
		SecretPath:    secretPath,
		PasswordHash:  string(hash),
		TOTPSecretRef: TOTPJSONPointer,
		TOTPEnrolled:  true,
		LastRotatedAt: time.Now().UTC(),
	})
}
