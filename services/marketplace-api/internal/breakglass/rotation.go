package breakglass

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Rotator generates fresh credentials for existing break-glass accounts.
// Two triggers: the 24h post-use hook (rotation_scheduled_at) and the
// 90-day cadence cron. Both funnel through RotateOne, which preserves
// the "Secret Manager first, DB second" ordering that the login
// handler depends on.
type Rotator struct {
	repo    *Repository
	secrets *SecretManager
	audit   *AuditEmitter
	slack   *SlackClient
}

// NewRotator returns a Rotator. `slack` and `audit` may be nil — the
// Rotator treats both as best-effort sinks so a broken Slack hook
// doesn't abort credential rotation.
func NewRotator(repo *Repository, secrets *SecretManager, auditE *AuditEmitter, slack *SlackClient) *Rotator {
	return &Rotator{repo: repo, secrets: secrets, audit: auditE, slack: slack}
}

// RotateOne rotates credentials for a single tenant. Steps:
//  1. Generate password + TOTP + bcrypt hash.
//  2. Write Blob to Secret Manager (MUST come before DB write).
//  3. Swap the password_hash on disk and clear rotation_scheduled_at.
//  4. Best-effort audit + Slack notify.
//
// A failure between steps 2 and 3 leaves the Secret Manager value
// ahead of the DB hash; a retry (the rotator cron runs daily) picks
// that up because rotation_scheduled_at is still pending.
func (r *Rotator) RotateOne(ctx context.Context, tenantID uuid.UUID) error {
	acc, err := r.repo.GetByTenant(ctx, tenantID)
	if err != nil {
		r.recordFailure(tenantID, "fetch_account: "+err.Error())
		return err
	}

	pw, err := GeneratePassword()
	if err != nil {
		r.recordFailure(tenantID, "generate_password: "+err.Error())
		return err
	}
	totpSecret, err := GenerateTOTPSecret()
	if err != nil {
		r.recordFailure(tenantID, "generate_totp: "+err.Error())
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), BcryptCost)
	if err != nil {
		r.recordFailure(tenantID, "bcrypt: "+err.Error())
		return err
	}

	if err := r.secrets.Upsert(ctx, acc.SecretPath, Blob{
		Password:    pw,
		TOTPSecret:  totpSecret,
		GeneratedAt: time.Now().UTC(),
	}); err != nil {
		r.recordFailure(tenantID, "secret_manager: "+err.Error())
		return err
	}

	if err := r.repo.ReplaceAfterRotation(ctx, tenantID, string(hash)); err != nil {
		r.recordFailure(tenantID, "db_replace: "+err.Error())
		return err
	}

	r.recordSuccess(ctx, tenantID)
	return nil
}

// RotateDue runs a single pass over every account that's due for
// rotation (post-use or 90-day). Returns the count of successful
// rotations. Failures are logged + emitted but never abort the
// remaining tenants — one dead IdP must not stop the cron.
func (r *Rotator) RotateDue(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	accs, err := r.repo.FindDueForRotation(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("find due: %w", err)
	}
	rotated := 0
	for _, a := range accs {
		if err := r.RotateOne(ctx, a.TenantID); err != nil {
			continue
		}
		rotated++
	}
	return rotated, nil
}

func (r *Rotator) recordSuccess(ctx context.Context, tenantID uuid.UUID) {
	if r.audit != nil {
		r.audit.EmitRotation(tenantID, true, "")
	}
	if r.slack != nil {
		// Fire-and-forget Slack notify; failures here do not revert
		// the rotation.
		_ = r.slack.PostRotationAlert(ctx, tenantID, true, "")
	}
}

func (r *Rotator) recordFailure(tenantID uuid.UUID, reason string) {
	if r.audit != nil {
		r.audit.EmitRotation(tenantID, false, reason)
	}
	// Slack best-effort; rotation cron logs will carry the detail.
	if r.slack != nil {
		_ = r.slack.PostRotationAlert(context.Background(), tenantID, false, reason)
	}
}
