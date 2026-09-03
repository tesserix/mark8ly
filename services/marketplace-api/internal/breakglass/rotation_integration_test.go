//go:build integration

package breakglass_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #404: rotating a disabled account's credentials must NOT clear
// disabled_at. Re-enabling is only ever an explicit, audited act (the
// enable endpoint) — otherwise an operator rotating for an unrelated
// reason (e.g. the 90-day cron, or a routine credential refresh) would
// silently undo someone else's deliberate security decision.
func TestIntegration_RotateOne_DoesNotReEnableADisabledAccount(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)
	secrets := breakglass.NewSecretManager(breakglass.NewFakeSecretClient())
	rotator := breakglass.NewRotator(repo, secrets, nil, nil)

	tenantID := uuid.New()
	boot := breakglass.NewBootstrapper(repo, secrets, "test-project")
	require.NoError(t, boot.Provision(context.Background(), tenantID))

	require.NoError(t, repo.Disable(context.Background(), tenantID, "suspected compromise"))

	before, err := repo.GetByTenant(context.Background(), tenantID)
	require.NoError(t, err)
	beforeHash := before.PasswordHash

	require.NoError(t, rotator.RotateOne(context.Background(), tenantID))

	after, err := repo.GetByTenant(context.Background(), tenantID)
	require.NoError(t, err)

	require.NotNil(t, after.DisabledAt,
		"RotateOne must NOT clear disabled_at — rotating credentials is not the same act as re-enabling")
	require.NotNil(t, after.DisabledReason)
	require.Equal(t, "suspected compromise", *after.DisabledReason,
		"the disable reason must survive a rotation untouched")
	require.NotEqual(t, beforeHash, after.PasswordHash,
		"rotation must still have actually happened — a disabled account is still rotated, just not re-enabled")
}
