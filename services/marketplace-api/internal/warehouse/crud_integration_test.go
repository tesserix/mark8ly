//go:build integration

package warehouse_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/warehouse"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #177 PR 5b — the repository half of warehouse CRUD.
//
// Upsert cannot serve the admin API. It is keyed on (store_id, name), which
// is precisely the trap #177 exists to remove: an edit that renames a
// warehouse would land on a DIFFERENT row and leave the stock behind on the
// old one. Create and UpdateByID key on the id instead, and a name
// collision is a reported conflict rather than a silent overwrite.

func TestCreate_RejectsADuplicateLiveName(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	_, err := repo.Create(ctx, db, sample(tenantID, storeID, "Main Warehouse"))
	require.NoError(t, err)

	_, err = repo.Create(ctx, db, sample(tenantID, storeID, "Main Warehouse"))
	require.ErrorIs(t, err, warehouse.ErrNameTaken,
		"a second warehouse under a live name must be refused, NOT silently merged into the first")

	all, err := repo.List(ctx, db, storeID, true)
	require.NoError(t, err)
	require.Len(t, all, 1)
}

// The partial index (000122) exists so an archived name can be reused.
func TestCreate_AllowsAnArchivedName(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	first, err := repo.Create(ctx, db, sample(tenantID, storeID, "Main Warehouse"))
	require.NoError(t, err)
	require.NoError(t, repo.Archive(ctx, db, first.ID))

	second, err := repo.Create(ctx, db, sample(tenantID, storeID, "Main Warehouse"))
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
}

// THE test this slice exists for. Renaming must move the row, not fork it.
func TestUpdateByID_RenameKeepsTheSameRow(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	created, err := repo.Create(ctx, db, sample(tenantID, storeID, "Main Warehouse"))
	require.NoError(t, err)

	updated := created
	updated.Name = "Bondi Depot"
	updated.City = "Bondi Beach"
	got, err := repo.UpdateByID(ctx, db, updated)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID, "a rename must edit the row, not create a second one")
	require.Equal(t, "Bondi Depot", got.Name)
	require.Equal(t, "Bondi Beach", got.City)

	all, err := repo.List(ctx, db, storeID, true)
	require.NoError(t, err)
	require.Len(t, all, 1, "renaming must not leave the old row behind holding the stock")
}

func TestUpdateByID_RejectsANameAnotherLiveWarehouseHolds(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	_, err := repo.Create(ctx, db, sample(tenantID, storeID, "Main Warehouse"))
	require.NoError(t, err)
	other, err := repo.Create(ctx, db, sample(tenantID, storeID, "Overflow"))
	require.NoError(t, err)

	other.Name = "Main Warehouse"
	_, err = repo.UpdateByID(ctx, db, other)
	require.ErrorIs(t, err, warehouse.ErrNameTaken)
}

func TestUpdateByID_UnknownIDIsNotFound(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	ghost := sample(tenantID, storeID, "Ghost")
	ghost.ID = "00000000-0000-0000-0000-0000000000ff"
	_, err := repo.UpdateByID(ctx, db, ghost)
	require.ErrorIs(t, err, warehouse.ErrNotFound)
}

// An archived warehouse is removed as far as the merchant is concerned.
// Editing one would be editing something the UI does not show.
func TestUpdateByID_RefusesAnArchivedWarehouse(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	created, err := repo.Create(ctx, db, sample(tenantID, storeID, "Gone"))
	require.NoError(t, err)
	require.NoError(t, repo.Archive(ctx, db, created.ID))

	created.Name = "Back From The Dead"
	_, err = repo.UpdateByID(ctx, db, created)
	require.ErrorIs(t, err, warehouse.ErrNotFound)

	// Asserting the error alone is not enough, and a mutation proved it: an
	// UPDATE that omits the archived filter still reports ErrNotFound,
	// because the read-back applies it — while having already rewritten the
	// archived row. The row itself is the assertion that matters.
	var name string
	require.NoError(t, db.Raw(`SELECT name FROM warehouses WHERE id = ?`, created.ID).Scan(&name).Error)
	require.Equal(t, "Gone", name, "an archived warehouse must not be written to at all")
}

func TestSetDefault_MovesTheFlagAndLeavesExactlyOne(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	first, err := repo.Create(ctx, db, sample(tenantID, storeID, "First"))
	require.NoError(t, err)
	second, err := repo.Create(ctx, db, sample(tenantID, storeID, "Second"))
	require.NoError(t, err)
	require.NoError(t, repo.SetDefault(ctx, db, storeID, first.ID))

	require.NoError(t, repo.SetDefault(ctx, db, storeID, second.ID))

	var defaults []string
	require.NoError(t, db.Raw(
		`SELECT id FROM warehouses WHERE store_id = ? AND is_default`, storeID).Scan(&defaults).Error)
	require.Equal(t, []string{second.ID}, defaults,
		"exactly one default per store — the old flag must be cleared in the same transaction")
}

func TestSetDefault_RefusesAnArchivedWarehouse(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	live, err := repo.Create(ctx, db, sample(tenantID, storeID, "Live"))
	require.NoError(t, err)
	require.NoError(t, repo.SetDefault(ctx, db, storeID, live.ID))
	archived, err := repo.Create(ctx, db, sample(tenantID, storeID, "Archived"))
	require.NoError(t, err)
	require.NoError(t, repo.Archive(ctx, db, archived.ID))

	err = repo.SetDefault(ctx, db, storeID, archived.ID)
	require.ErrorIs(t, err, warehouse.ErrNotFound)

	var stillDefault string
	require.NoError(t, db.Raw(
		`SELECT id FROM warehouses WHERE store_id = ? AND is_default`, storeID).Scan(&stillDefault).Error)
	require.Equal(t, live.ID, stillDefault,
		"a refused SetDefault must not have cleared the existing default first")
}

// Another store's warehouse must not be reachable by id alone.
func TestSetDefault_IgnoresAnotherStoresWarehouse(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantA, storeA := seedStore(t, db)
	_, storeB := seedStore(t, db)

	mine, err := repo.Create(ctx, db, sample(tenantA, storeA, "Mine"))
	require.NoError(t, err)

	err = repo.SetDefault(ctx, db, storeB, mine.ID)
	require.ErrorIs(t, err, warehouse.ErrNotFound)
}

// An id alone is a guessable handle to another tenant's row; the handler
// takes the store from the URL, so the repository must scope by it too.
func TestUpdateByID_IgnoresAnotherStoresWarehouse(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantA, storeA := seedStore(t, db)
	_, storeB := seedStore(t, db)

	mine, err := repo.Create(ctx, db, sample(tenantA, storeA, "Mine"))
	require.NoError(t, err)

	attempt := mine
	attempt.StoreID = storeB
	attempt.Name = "Stolen"
	_, err = repo.UpdateByID(ctx, db, attempt)
	require.ErrorIs(t, err, warehouse.ErrNotFound)

	var name string
	require.NoError(t, db.Raw(`SELECT name FROM warehouses WHERE id = ?`, mine.ID).Scan(&name).Error)
	require.Equal(t, "Mine", name, "another store's warehouse must be untouched")
}
