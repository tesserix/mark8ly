package onboarding

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark8ly/platform-api/internal/outbox"
)

// GIPClaimOutboxKind is the outbox kind for writing the owner's tenant_id
// custom claim into Google Identity Platform.
const GIPClaimOutboxKind = "gip.set_tenant_claim"

// gipClaimPayload is the outbox payload for GIPClaimOutboxKind.
type gipClaimPayload struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
}

// TenantClaimSetter writes the tenant_id custom claim onto a GIP user.
// gipadmin.AdminClient satisfies this. Kept as an interface so onboarding
// does not depend on the concrete client and can be tested with a fake.
type TenantClaimSetter interface {
	EnsureTenantClaim(ctx context.Context, uid, tenantID string) error
}

// NewGIPClaimOutboxHandler returns the outbox handler that stamps the
// owner's tenant_id custom claim onto their GIP account.
//
// Why this must exist, and why it goes through the outbox:
//
// marketplace-api's mobile bearer auth resolves a caller's tenant purely
// from the tenant_id custom claim on the GIP ID token
// (services/marketplace-api/internal/auth/gip_verifier.go). Nothing in the
// codebase ever wrote that claim, so every tenant created through the
// onboarding wizard had an owner who could authenticate but was refused by
// the mobile API — which the app surfaced as an endless bounce back to the
// login screen. The web admin was unaffected because it resolves the tenant
// from the database instead.
//
// It rides the outbox rather than a best-effort post-commit call (as
// EnsureSelfVendor does) precisely because a silent failure here locks the
// owner out of the mobile app entirely. The drainer retries with backoff, so
// a transient Identity Toolkit outage self-heals instead of stranding them.
//
// EnsureTenantClaim is idempotent, so redelivery is safe.
func NewGIPClaimOutboxHandler(claims TenantClaimSetter) outbox.Handler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p gipClaimPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("gip claim outbox: parse payload: %w", err)
		}
		if p.UserID == "" || p.TenantID == "" {
			return fmt.Errorf("gip claim outbox: missing user_id or tenant_id")
		}
		if err := claims.EnsureTenantClaim(ctx, p.UserID, p.TenantID); err != nil {
			return fmt.Errorf("gip claim outbox: set tenant claim for %s: %w", p.UserID, err)
		}
		return nil
	}
}
