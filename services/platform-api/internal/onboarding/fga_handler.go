package onboarding

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/outbox"
)

// NewFGAOutboxHandler returns the outbox handler that ships FGA membership
// writes to OpenFGA. It writes BOTH the owner and member tuples for the
// (user, tenant) pair from the payload.
//
// Returning an error from the handler causes the drainer to retry with
// exponential backoff. Eventual success is guaranteed as long as OpenFGA
// is reachable.
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
		if err := fga.WriteMembership(ctx, p.UserID, p.TenantID); err != nil {
			return fmt.Errorf("fga outbox: write membership: %w", err)
		}
		return nil
	}
}
