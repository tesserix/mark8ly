package onboarding

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/outbox"
)

// NewFGAOutboxHandler returns the outbox handler that ships the
// owner tuple to OpenFGA on onboarding completion.
//
// Phase O: only the owner tuple is written. Under the new DSL
// (infra/openfga/model.fga), `member` is a derived union over
// owner/admin/staff/viewer, so writing owner is sufficient to pass
// auth-bff's CheckMembership during autologin — the old explicit
// `member` tuple would be rejected by the model now since member
// is no longer directly-assignable.
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
		return nil
	}
}
