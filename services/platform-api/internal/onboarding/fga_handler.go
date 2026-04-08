package onboarding

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/outbox"
)

// NewFGAOutboxHandler returns the outbox handler that ships the
// onboarding-completion FGA tuples to OpenFGA. Two tuples:
//
//  1. user:<uid> owner tenant:<tid>       — Phase D/O
//  2. tenant:<tid> parent store:<sid>     — Phase Q
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
		if p.UserID == "" || p.TenantID == "" {
			return fmt.Errorf("fga outbox: missing user_id or tenant_id")
		}
		if err := fga.WriteOwnership(ctx, p.UserID, p.TenantID); err != nil {
			return fmt.Errorf("fga outbox: write ownership: %w", err)
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
