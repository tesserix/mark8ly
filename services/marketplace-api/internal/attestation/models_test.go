//go:build integration

package attestation_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/mark8ly/marketplace-api/internal/attestation"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestBusinessEntityAttestation_RoundTrip(t *testing.T) {
	db := testdb.NewDB(t, "business_entity_attestations")

	row := attestation.BusinessEntityAttestation{
		StoreID: uuid.New(), TenantID: uuid.New(),
		Country: "US", CheckboxText: "I attest", CheckboxVersion: "v1",
	}
	require.NoError(t, db.Create(&row).Error)

	var got attestation.BusinessEntityAttestation
	require.NoError(t, db.First(&got, "id=?", row.ID).Error)
	require.Equal(t, "US", got.Country)
}
