package onboarding

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/outbox"
)

// NewFGAOutboxHandler returns the outbox handler that ships the
// onboarding-completion FGA tuples to OpenFGA:
//
//  1. user:<subject> owner tenant:<tid>   — Phase D/O, once per subject
//  2. tenant:<tid> parent store:<sid>     — Phase Q
//
// There is ONE subject on the GIP path (the GIP uid) and TWO on the
// Zitadel path (the owner's lowercased email and their Zitadel user id).
// See fgaWritePayload.UserIDs for why both keys are required rather than
// merely convenient.
//
// The parent tuple unlocks the `from parent` inheritance in the
// store type's DSL: tenant owners/admins automatically get
// store-level permissions on every store in their tenant, so the
// common single-founder case needs zero per-store tuples.
//
// Returning an error from the handler causes the drainer to retry
// with exponential backoff. Eventual success is guaranteed as long
// as OpenFGA is reachable.
func NewFGAOutboxHandler(fga authz.Client) outbox.Handler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p fgaWritePayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("fga outbox: parse payload: %w", err)
		}
		// user_ids is the current shape; user_id is what every row
		// enqueued before #685 carries, and rows pending across the
		// rollout must still drain. Reading both is what makes this deploy
		// safe in either order.
		subjects := p.UserIDs
		if len(subjects) == 0 && p.UserID != "" {
			subjects = []string{p.UserID}
		}
		if len(subjects) == 0 || p.TenantID == "" {
			return fmt.Errorf("fga outbox: missing user_id or tenant_id")
		}
		for _, subject := range subjects {
			if subject == "" {
				return fmt.Errorf("fga outbox: empty subject in user_ids")
			}
			if err := fga.WriteOwnership(ctx, subject, p.TenantID); err != nil {
				return fmt.Errorf("fga outbox: write ownership for %q: %w", subject, err)
			}
		}
		// StoreID is optional so pre-Phase-Q outbox rows that were
		// written without one (and any future tuple-write payloads
		// that don't involve a store) still drain cleanly.
		if p.StoreID != "" {
			if err := fga.WriteStoreParent(ctx, p.StoreID, p.TenantID); err != nil {
				return fmt.Errorf("fga outbox: write store parent: %w", err)
			}
		}
		return nil
	}
}
