//go:build integration

package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/product"
)

func fieldsURL(storeID, productID string) string {
	return "/api/v1/admin/stores/" + storeID + "/products/" + productID + "/personalisation-fields"
}

// decodeField pulls the single-field response shape.
func decodeField(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out), "body: %s", string(body))
	return out
}

func TestAPI_Personalisation_CreateImageField_AppliesDefaults(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	w := request(t, env.router, http.MethodPost, fieldsURL(storeID, pid), map[string]any{
		"key":      "your_photo",
		"label":    "Your photo",
		"kind":     "image",
		"required": true,
	}, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	got := decodeField(t, w.Body.Bytes())
	require.Equal(t, "your_photo", got["key"])
	require.Equal(t, "image", got["kind"])
	require.Equal(t, true, got["required"])
	require.EqualValues(t, 1, got["max_images"],
		"an image field must carry a max_images even when the merchant did not type one")
	require.NotContains(t, got, "max_length",
		"kind-irrelevant columns are omitted, not sent as null — the form drives off presence")
	require.NotContains(t, got, "price_delta")
}

func TestAPI_Personalisation_SelectArrivesWithItsValuesOrNotAtAll(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	// No values: refused.
	w := request(t, env.router, http.MethodPost, fieldsURL(storeID, pid), map[string]any{
		"key": "finish", "label": "Finish", "kind": "select",
	}, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// With values: created, and the deltas come back.
	w = request(t, env.router, http.MethodPost, fieldsURL(storeID, pid), map[string]any{
		"key": "finish", "label": "Finish", "kind": "select",
		"options": []map[string]any{
			{"value": "matte", "label": "Matte", "price_delta": "0"},
			{"value": "gloss", "label": "Gloss", "price_delta": "2.50"},
		},
	}, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	got := decodeField(t, w.Body.Bytes())
	opts, ok := got["options"].([]any)
	require.True(t, ok, "options missing: %s", w.Body.String())
	require.Len(t, opts, 2)

	// Both rows landed in the same transaction as the field.
	var n int64
	require.NoError(t, env.db.Raw(
		`SELECT count(*) FROM product_personalisation_options o
		  JOIN product_personalisation_fields f ON f.id = o.field_id
		 WHERE f.product_id = ?`, pid).Scan(&n).Error)
	require.EqualValues(t, 2, n)
}

func TestAPI_Personalisation_FieldFromAnotherStoreIsNotFound(t *testing.T) {
	env := setupTestRouter(t)
	storeA, tenantID := seedStoreRow(t, env.db, "")
	storeB, _ := seedStoreRow(t, env.db, tenantID) // same tenant, different store
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pidA, _, _ := seedProductViaService(t, env, storeA, tenantID)
	pidB, _, _ := seedProductViaService(t, env, storeB, tenantID)

	w := request(t, env.router, http.MethodPost, fieldsURL(storeA, pidA), map[string]any{
		"key": "name", "label": "Name", "kind": "text",
	}, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	fieldID := decodeField(t, w.Body.Bytes())["id"].(string)

	// Store B's path, store A's field id. StoreMiddleware proves :storeId
	// belongs to the tenant and nothing more, so this is the check that
	// matters — see store_scope_arch_test.go.
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPatch, fieldsURL(storeB, pidB) + "/" + fieldID},
		{http.MethodDelete, fieldsURL(storeB, pidB) + "/" + fieldID},
		{http.MethodPost, fieldsURL(storeB, pidB) + "/" + fieldID + "/options"},
	} {
		w := request(t, env.router, tc.method, tc.path,
			map[string]any{"label": "Hijacked", "value": "x"}, authHeaders(userID, tenantID))
		require.Equalf(t, http.StatusNotFound, w.Code,
			"%s %s leaked another store's field: %s", tc.method, tc.path, w.Body.String())
	}
}

func TestAPI_Personalisation_FieldFromAnotherTenantIsNotFound(t *testing.T) {
	env := setupTestRouter(t)
	storeA, tenantA := seedStoreRow(t, env.db, "")
	storeB, tenantB := seedStoreRow(t, env.db, "")

	userA := uuid.NewString()
	env.fga.Grant(userA, authz.RoleAdmin, tenantA)
	userB := uuid.NewString()
	env.fga.Grant(userB, authz.RoleAdmin, tenantB)

	pidA, _, _ := seedProductViaService(t, env, storeA, tenantA)
	pidB, _, _ := seedProductViaService(t, env, storeB, tenantB)

	w := request(t, env.router, http.MethodPost, fieldsURL(storeA, pidA), map[string]any{
		"key": "name", "label": "Name", "kind": "text",
	}, authHeaders(userA, tenantA))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	fieldID := decodeField(t, w.Body.Bytes())["id"].(string)

	w = request(t, env.router, http.MethodDelete,
		fieldsURL(storeB, pidB)+"/"+fieldID, nil, authHeaders(userB, tenantB))
	require.Equal(t, http.StatusNotFound, w.Code,
		"tenant B deleted tenant A's field: %s", w.Body.String())

	// And it is still there.
	var n int64
	require.NoError(t, env.db.Raw(
		`SELECT count(*) FROM product_personalisation_fields WHERE id = ?`, fieldID).Scan(&n).Error)
	require.EqualValues(t, 1, n)
}

func TestAPI_Personalisation_SelectKeepsAtLeastOneValue(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	w := request(t, env.router, http.MethodPost, fieldsURL(storeID, pid), map[string]any{
		"key": "finish", "label": "Finish", "kind": "select",
		"options": []map[string]any{
			{"value": "matte", "label": "Matte"},
			{"value": "gloss", "label": "Gloss"},
		},
	}, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	got := decodeField(t, w.Body.Bytes())
	fieldID := got["id"].(string)
	opts := got["options"].([]any)
	firstID := opts[0].(map[string]any)["id"].(string)
	secondID := opts[1].(map[string]any)["id"].(string)

	// One of two: fine.
	w = request(t, env.router, http.MethodDelete,
		fieldsURL(storeID, pid)+"/"+fieldID+"/options/"+firstID, nil, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The last one: refused, because an active product with an empty
	// select would start rejecting every checkout of itself.
	w = request(t, env.router, http.MethodDelete,
		fieldsURL(storeID, pid)+"/"+fieldID+"/options/"+secondID, nil, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}

func TestAPI_Personalisation_DeletingFieldCascadesItsOptions(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	w := request(t, env.router, http.MethodPost, fieldsURL(storeID, pid), map[string]any{
		"key": "finish", "label": "Finish", "kind": "select",
		"options": []map[string]any{{"value": "matte", "label": "Matte"}},
	}, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	fieldID := decodeField(t, w.Body.Bytes())["id"].(string)

	w = request(t, env.router, http.MethodDelete,
		fieldsURL(storeID, pid)+"/"+fieldID, nil, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	var n int64
	require.NoError(t, env.db.Raw(
		`SELECT count(*) FROM product_personalisation_options WHERE field_id = ?`, fieldID).Scan(&n).Error)
	require.Zero(t, n, "options must cascade from the field")
}

func TestAPI_Personalisation_SoftDeletedProductHidesItsFields(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	w := request(t, env.router, http.MethodPost, fieldsURL(storeID, pid), map[string]any{
		"key": "name", "label": "Name", "kind": "text",
	}, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	fieldID := decodeField(t, w.Body.Bytes())["id"].(string)

	require.NoError(t, env.db.Exec(
		`UPDATE products SET deleted_at = now() WHERE id = ?`, pid).Error)

	// The row is still in the table — the scope clause is what hides it,
	// so an id someone kept cannot be used to edit a deleted product.
	w = request(t, env.router, http.MethodPatch,
		fieldsURL(storeID, pid)+"/"+fieldID,
		map[string]any{"label": "Edited"}, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestAPI_Personalisation_CheckboxDeltaOfZeroIsWritten(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	w := request(t, env.router, http.MethodPost, fieldsURL(storeID, pid), map[string]any{
		"key": "gift_wrap", "label": "Gift wrap", "kind": "checkbox", "price_delta": "4.00",
	}, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	fieldID := decodeField(t, w.Body.Bytes())["id"].(string)

	// Making a paid add-on free sends 0. A zero-check in the handler would
	// discard it and the merchant would keep charging for gift wrap.
	w = request(t, env.router, http.MethodPatch,
		fieldsURL(storeID, pid)+"/"+fieldID,
		map[string]any{"price_delta": "0"}, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var delta string
	require.NoError(t, env.db.Raw(
		`SELECT price_delta::text FROM product_personalisation_fields WHERE id = ?`, fieldID).
		Scan(&delta).Error)
	require.Equal(t, "0.00", delta)
}

func TestAPI_Personalisation_ListIsInPositionOrder(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	for _, f := range []struct {
		key string
		pos int
	}{{"third", 30}, {"first", 10}, {"second", 20}} {
		w := request(t, env.router, http.MethodPost, fieldsURL(storeID, pid), map[string]any{
			"key": f.key, "label": f.key, "kind": "text", "position": f.pos,
		}, authHeaders(userID, tenantID))
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	}

	w := request(t, env.router, http.MethodGet, fieldsURL(storeID, pid), nil, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Fields []struct{ Key string } `json:"fields"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Fields, 3)
	require.Equal(t, "first", resp.Fields[0].Key)
	require.Equal(t, "second", resp.Fields[1].Key)
	require.Equal(t, "third", resp.Fields[2].Key)
}

func TestAPI_Personalisation_AbsoluteCeilingApplies(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	// The plan gate is not wired in this harness, so this exercises the
	// service's ceiling — the one that holds even on a plan with
	// Unlimited.
	for i := 0; i < product.MaxPersonalisationFieldsPerProduct; i++ {
		w := request(t, env.router, http.MethodPost, fieldsURL(storeID, pid), map[string]any{
			"key": "f_" + uuid.NewString()[:8], "label": "F", "kind": "text",
		}, authHeaders(userID, tenantID))
		require.Equalf(t, http.StatusCreated, w.Code, "field %d: %s", i, w.Body.String())
	}

	w := request(t, env.router, http.MethodPost, fieldsURL(storeID, pid), map[string]any{
		"key": "one_too_many", "label": "F", "kind": "text",
	}, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}

// TestPersonalisation_DatabaseBacksTheGoValidation proves the CHECK
// constraints exist, by going around the service and writing the rows it
// would have refused.
//
// Without this, the Go validation and migration 000139 could drift and
// nothing would notice until some other writer — a CSV import, a copy-to-
// store, a backfill — reached the table.
func TestPersonalisation_DatabaseBacksTheGoValidation(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	insert := func(cols, vals string, args ...any) error {
		q := `INSERT INTO product_personalisation_fields
		        (tenant_id, store_id, product_id, key, label, kind` + cols + `)
		      VALUES (?, ?, ?, ?, ?, ?` + vals + `)`
		all := append([]any{tenantID, storeID, pid}, args...)
		return env.db.Exec(q, all...).Error
	}

	require.Error(t, insert(", max_length", ", ?", "k1", "L", "image", 50),
		"max_length on an image kind must be refused by the database")
	require.Error(t, insert(", max_images", ", ?", "k2", "L", "text", 3),
		"max_images on a text kind must be refused by the database")
	require.Error(t, insert(", price_delta", ", ?", "k3", "L", "select", "1.00"),
		"price_delta on a select must be refused — a select's money is on its options")
	require.Error(t, insert("", "", "Bad-Key", "L", "text"),
		"the key format CHECK must reject what personalisationKeyRe rejects")
	require.Error(t, insert("", "", "k4", "   ", "text"),
		"a blank label must be refused by the database")
	require.Error(t, insert("", "", "k5", "L", "hologram"),
		"an unknown kind must be refused by the database")

	// And the shape the service would actually write is accepted.
	require.NoError(t, insert(", max_images", ", ?", "good_key", "Your photo", "image", 1))
}
