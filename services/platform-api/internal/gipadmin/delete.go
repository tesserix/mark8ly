package gipadmin

import (
	"context"
	"errors"
	"fmt"
)

// DeleteAccount removes the GIP user identified by uid from the configured
// tenant pool. It is idempotent: a missing account (USER_NOT_FOUND) is treated
// as success, since account deletion is retried and the user may already be
// gone. Deleting the GIP user invalidates all of that user's tokens.
func (c *AdminClient) DeleteAccount(ctx context.Context, uid string) error {
	if uid == "" {
		return fmt.Errorf("gipadmin: uid is required")
	}
	err := c.postAdmin(ctx, "accounts:delete", map[string]any{"localId": uid})
	if errors.Is(err, ErrUserNotFound) {
		return nil
	}
	return err
}
