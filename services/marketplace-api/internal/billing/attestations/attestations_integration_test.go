//go:build integration

package attestations_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/attestations"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func input(invoiceID string) attestations.Input {
	return attestations.Input{
		TenantID:         uuid.New(),
		StoreID:          uuid.New(),
		SubscriptionID:   uuid.New(),
		AttestationType:  attestations.TypeApple426,
		AttestedByUserID: uuid.New(),
		AttestationText:  "I acknowledge Apple Guideline 4.2.6 may cause first-review rejection.",
		IPAddress:        "10.0.0.1",
		UserAgent:        "curl/8",
		StripeInvoiceID:  invoiceID,
	}
}

func TestAttestations_RecordAndFindByStripeInvoice(t *testing.T) {
	db := testdb.NewDB(t, "app_contract_attestations")

	id, err := attestations.Record(context.Background(), db, input("in_record_1"))
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, id)

	got, err := attestations.FindByStripeInvoice(context.Background(), db, "in_record_1")
	require.NoError(t, err)
	require.Equal(t, id, got.ID)
	require.Equal(t, attestations.TypeApple426, got.AttestationType)
	require.Equal(t, "in_record_1", got.StripeInvoiceID)
}

func TestAttestations_DuplicateStripeInvoice_ErrDuplicateInvoice(t *testing.T) {
	db := testdb.NewDB(t, "app_contract_attestations")

	_, err := attestations.Record(context.Background(), db, input("in_dup"))
	require.NoError(t, err)

	_, err = attestations.Record(context.Background(), db, input("in_dup"))
	require.Error(t, err)
	require.ErrorIs(t, err, attestations.ErrDuplicateInvoice,
		"second Record with same invoice must wrap ErrDuplicateInvoice; got %v", err)
}

func TestAttestations_UpdateRejectedByTrigger(t *testing.T) {
	db := testdb.NewDB(t, "app_contract_attestations")

	id, err := attestations.Record(context.Background(), db, input("in_trigger"))
	require.NoError(t, err)

	err = db.Exec(
		"UPDATE app_contract_attestations SET attestation_text = 'tampered' WHERE id = ?",
		id,
	).Error
	require.Error(t, err, "UPDATE must be blocked by BEFORE UPDATE trigger")
	require.Contains(t, err.Error(), "append-only")
}

// TestAttestations_DeleteRejectedByRoleRevoke requires a second DB connection
// opened as the marketplace_user app role (NOT the superuser testdb uses for
// DDL). Skipped if TEST_DATABASE_APP_URL is not set — mirrors the pattern in
// internal/attestation/immutability_test.go.
func TestAttestations_DeleteRejectedByRoleRevoke(t *testing.T) {
	appDSN := os.Getenv("TEST_DATABASE_APP_URL")
	if appDSN == "" {
		t.Skip("TEST_DATABASE_APP_URL not set — skipping role-level revoke test")
	}

	admin := testdb.NewDB(t, "app_contract_attestations")
	id, err := attestations.Record(context.Background(), admin, input("in_role_revoke"))
	require.NoError(t, err)

	appDB, err := gorm.Open(postgres.Open(appDSN), &gorm.Config{})
	require.NoError(t, err)

	err = appDB.Exec("DELETE FROM app_contract_attestations WHERE id = ?", id).Error
	require.Error(t, err, "DELETE must be blocked by role-level REVOKE, not trigger")
	require.Contains(t, err.Error(), "permission denied",
		"error must be permission denied (role revoke), not append-only (trigger)")
}

func TestAttestations_Record_MissingRequiredFields(t *testing.T) {
	db := testdb.NewDB(t, "app_contract_attestations")
	cases := []struct {
		name string
		mut  func(attestations.Input) attestations.Input
	}{
		{"missing type", func(i attestations.Input) attestations.Input { i.AttestationType = ""; return i }},
		{"missing invoice", func(i attestations.Input) attestations.Input { i.StripeInvoiceID = ""; return i }},
		{"missing text", func(i attestations.Input) attestations.Input { i.AttestationText = ""; return i }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := attestations.Record(context.Background(), db, tc.mut(input("in_missing")))
			require.Error(t, err)
		})
	}
}

func TestAttestations_FindByStripeInvoice_NotFound(t *testing.T) {
	db := testdb.NewDB(t, "app_contract_attestations")
	_, err := attestations.FindByStripeInvoice(context.Background(), db, "in_missing_xyz")
	require.Error(t, err)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound),
		"want errors.Is(err, gorm.ErrRecordNotFound); got %v", err)
}
