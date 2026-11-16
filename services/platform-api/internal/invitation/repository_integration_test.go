//go:build integration

package invitation

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/platform-api/pkg/testdb"
)

// Every role the API accepts must also fit the table's CHECK constraint.
// cashier (#1104) was allowed in Go but not in the database, so every
// cashier invitation failed with a 500 on insert.
func TestIntegration_Create_EveryAllowedTenantRole(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	var tenantID string
	if err := tx.Raw(`INSERT INTO tenants (name, owner_user_id, owner_email, status)
		VALUES ('Invite Co', 'uid-owner', 'owner@test.local', 'active') RETURNING id`).Row().Scan(&tenantID); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	for _, role := range []string{"admin", "staff", "viewer", "cashier"} {
		if !isAllowedTenantRole(role) {
			t.Fatalf("%s is no longer an allowed role; update this test", role)
		}
		inv := &Invitation{
			TenantID: tenantID, Email: role + "@test.local", Role: role,
			TokenHash: uuid.NewString()[:32] + uuid.NewString()[:32], ExpiresAt: time.Now().Add(72 * time.Hour),
			Status: "pending", InvitedByUserID: "uid-owner",
		}
		if err := repo.Create(context.Background(), inv); err != nil {
			t.Errorf("create %s invitation: %v", role, err)
		}
	}
}
