//go:build integration

package attestation_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/attestation"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestBusinessEntityAttestation_UpdateRejectedByTrigger(t *testing.T) {
	db := testdb.NewDB(t, "business_entity_attestations")
	row := attestation.BusinessEntityAttestation{
		StoreID: uuid.New(), TenantID: uuid.New(),
		Country: "US", CheckboxText: "attest", CheckboxVersion: "v1",
	}
	require.NoError(t, db.Create(&row).Error)

	err := db.Exec("UPDATE business_entity_attestations SET country='CA' WHERE id=?", row.ID).Error
	require.Error(t, err, "UPDATE must be rejected by trigger")
	require.Contains(t, err.Error(), "append-only")
}

func TestBusinessEntityAttestation_DeleteRejectedByRoleRevoke(t *testing.T) {
	appDSN := os.Getenv("TEST_DATABASE_APP_URL")
	if appDSN == "" {
		t.Skip("TEST_DATABASE_APP_URL not set — skipping role-level revoke test")
	}

	// Seed a row via superuser connection.
	admin := testdb.NewDB(t, "business_entity_attestations")
	row := attestation.BusinessEntityAttestation{
		StoreID: uuid.New(), TenantID: uuid.New(),
		Country: "US", CheckboxText: "attest", CheckboxVersion: "v1",
	}
	require.NoError(t, admin.Create(&row).Error)

	// Open a connection as the app role.
	appDB, err := gorm.Open(postgres.Open(appDSN), &gorm.Config{})
	require.NoError(t, err)

	// DELETE must be denied at the role level, NOT the trigger.
	err = appDB.Exec("DELETE FROM business_entity_attestations WHERE id=?", row.ID).Error
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
}
