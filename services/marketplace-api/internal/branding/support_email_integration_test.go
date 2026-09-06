//go:build integration

package branding

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/storeidentity"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestSupportEmail_SurvivesAnUnrelatedBrandingSave is the #749 hazard
// against real SQL. Upsert's UPDATE path is Select("*"), which writes
// every mapped column — including, now, support_email. The fake-repo unit
// test proves Update merges correctly; only this one proves the row that
// lands in Postgres still holds the address.
func TestSupportEmail_SurvivesAnUnrelatedBrandingSave(t *testing.T) {
	tx := testdb.NewTx(t)
	ctx := context.Background()

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, tx, tenantID, storeID)

	svc := NewService(ServiceConfig{DB: tx, Repo: NewRepository()})

	addr := "hello@nadiasceramics.com"
	_, err := svc.Update(ctx, UpdateInput{
		TenantID: tenantID, StoreID: storeID, SupportEmail: &addr,
	})
	require.NoError(t, err)

	// A second save that never mentions support_email — the shape of
	// almost every real branding edit.
	accent := "#2D4A2B"
	_, err = svc.Update(ctx, UpdateInput{
		TenantID: tenantID, StoreID: storeID, ColorAccent: &accent,
	})
	require.NoError(t, err)

	var stored string
	require.NoError(t, tx.Raw(
		`SELECT support_email FROM store_branding WHERE store_id = ?`, storeID,
	).Row().Scan(&stored))
	require.Equal(t, addr, stored,
		"Select(\"*\") must write back the address it read, not blank it")
}

// TestSupportEmail_ReachesReplyTo walks the whole path #749 exists to
// close: the merchant saves an address through the branding service, and
// the mail sender picks it up as Reply-To. Before this change nothing
// wrote the column, so storeidentity always read "" and every store's
// customer mail replied to the platform.
func TestSupportEmail_ReachesReplyTo(t *testing.T) {
	tx := testdb.NewTx(t)
	ctx := context.Background()

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, tx, tenantID, storeID)

	svc := NewService(ServiceConfig{DB: tx, Repo: NewRepository()})
	addr := "hello@nadiasceramics.com"
	_, err := svc.Update(ctx, UpdateInput{
		TenantID: tenantID, StoreID: storeID, SupportEmail: &addr,
	})
	require.NoError(t, err)

	store, err := storeidentity.NewDBLoader(tx).Load(ctx, storeID)
	require.NoError(t, err)
	require.Equal(t, addr, store.ContactEmail)

	id := email.StoreIdentity("noreply@mark8ly.com", email.StoreSender{
		Name: store.Name, Slug: store.Slug, ContactEmail: store.ContactEmail,
	})
	require.Equal(t, addr, id.ReplyTo)
}

// TestSupportEmail_UnsetFallsBackToPlatform records the decision behind
// the empty case: blank is not an error, it means "replies reach the
// platform". The admin UI says so in as many words.
func TestSupportEmail_UnsetFallsBackToPlatform(t *testing.T) {
	tx := testdb.NewTx(t)
	ctx := context.Background()

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, tx, tenantID, storeID)

	svc := NewService(ServiceConfig{DB: tx, Repo: NewRepository()})
	tagline := "Wheel-thrown in Margate"
	_, err := svc.Update(ctx, UpdateInput{
		TenantID: tenantID, StoreID: storeID, Tagline: &tagline,
	})
	require.NoError(t, err)

	store, err := storeidentity.NewDBLoader(tx).Load(ctx, storeID)
	require.NoError(t, err)
	require.Empty(t, store.ContactEmail)

	id := email.StoreIdentity("noreply@mark8ly.com", email.StoreSender{
		Name: store.Name, Slug: store.Slug, ContactEmail: store.ContactEmail,
	})
	require.Equal(t, "noreply@mark8ly.com", id.ReplyTo)
}
