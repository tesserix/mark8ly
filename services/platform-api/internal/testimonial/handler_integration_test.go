//go:build integration

package testimonial

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	platformapi "github.com/mark8ly/platform-api"
	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/pkg/testdb"
	"github.com/stretchr/testify/require"
)

func TestIntegrationMerchantPublicationAndWithdrawal(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(`INSERT INTO tenants (id, name, owner_user_id, owner_email) VALUES (?, 'Review company', 'review-owner', ?)`, tenantID, tenantID+"@test.local").Error)
	require.NoError(t, db.Exec(`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone) VALUES (?, ?, ?, 'Review shop', 'AU', 'AUD', 'Australia/Sydney')`, storeID, tenantID, "review-"+storeID).Error)
	fga := authz.NewFake()
	require.NoError(t, fga.WriteStoreParent(context.Background(), storeID, tenantID))
	require.NoError(t, fga.WriteOwnership(context.Background(), "review-owner", tenantID))
	h := NewHandler(db, fga)
	r := gin.New()
	h.RegisterInternal(r.Group("/internal"))
	h.RegisterPublic(r.Group("/api/v1"))
	call := func(method, path, uid, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", uid)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	path := "/internal/stores/" + storeID + "/testimonial"
	readPublic := func() []PublicReview {
		w := call(http.MethodGet, "/api/v1/testimonials", "", "")
		require.Equal(t, 200, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "author_name", "personal names must not be in the public contract")
		require.NotContains(t, w.Body.String(), "submitted_by")
		require.NotContains(t, w.Body.String(), "consented_at")
		var body struct {
			Data []PublicReview `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body.Data
	}
	require.Equal(t, `{"data":null}`, call(http.MethodGet, path, "review-owner", "").Body.String())
	for _, publish := range []bool{false, true, true, false} {
		body, err := json.Marshal(map[string]any{"author_name": " A Seller ", "quote": " My honest review. ", "rating": 3, "publish": publish, "consent": publish})
		require.NoError(t, err)
		w := call(http.MethodPut, path, "review-owner", string(body))
		require.Equal(t, 200, w.Code, w.Body.String())
		rows := readPublic()
		if publish {
			require.Contains(t, rows, PublicReview{StoreID: storeID, StoreName: "Review shop", Position: "Store owner", Quote: "My honest review.", Rating: 3})
		} else {
			for _, row := range rows {
				require.NotEqual(t, storeID, row.StoreID)
			}
		}
	}
	w := call(http.MethodPut, path, "review-owner", `{"author_name":"Seller","quote":"My review.","rating":5,"publish":true,"consent":true}`)
	require.Equal(t, 200, w.Code)
	for _, table := range []string{"stores", "tenants"} {
		id := storeID
		if table == "tenants" {
			id = tenantID
		}
		require.NoError(t, db.Table(table).Where("id = ?", id).Update("status", "suspended").Error)
		for _, row := range readPublic() {
			require.NotEqual(t, storeID, row.StoreID, "suspended businesses must not be promoted")
		}
		require.NoError(t, db.Table(table).Where("id = ?", id).Update("status", "active").Error)
	}
	var count int64
	require.NoError(t, db.Table("merchant_testimonials").Where("store_id = ?", storeID).Count(&count).Error)
	require.EqualValues(t, 1, count, "retries update the same review")
	require.Equal(t, 404, call(http.MethodPut, path, "other-tenant", `{}`).Code)
	require.NoError(t, fga.WriteRole(context.Background(), "review-admin", authz.RoleAdmin, tenantID))
	w = call(http.MethodPut, path, "review-admin", `{"position":"Store owner","author_name":"Private Person","quote":"Admin review.","rating":4,"publish":true,"consent":true}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, readPublic(), PublicReview{StoreID: storeID, StoreName: "Review shop", Position: "Store admin", Quote: "Admin review.", Rating: 4})
}

func TestIntegrationDraftSeedsStayPrivate(t *testing.T) {
	db := testdb.NewTx(t)
	down, err := platformapi.MigrationsFS.ReadFile("migrations/000022_merchant_testimonials.down.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(down)).Error)
	for _, name := range []string{"Mugshot Studio", "Buttercup Bakes", "Daily Basket"} {
		tenantID, storeID := uuid.NewString(), uuid.NewString()
		require.NoError(t, db.Exec(`INSERT INTO tenants (id, name, owner_user_id, owner_email) VALUES (?, ?, ?, ?)`, tenantID, name, tenantID, tenantID+"@test.local").Error)
		require.NoError(t, db.Exec(`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone) VALUES (?, ?, ?, ?, 'AU', 'AUD', 'Australia/Sydney')`, storeID, tenantID, "draft-"+storeID, name).Error)
	}
	up, err := platformapi.MigrationsFS.ReadFile("migrations/000022_merchant_testimonials.up.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(up)).Error)
	var drafts []struct {
		StoreName   string
		Position    string
		Published   bool
		Rating      int
		ConsentedAt *string
	}
	require.NoError(t, db.Table("merchant_testimonials r").Joins("JOIN stores s ON s.id = r.store_id").Select("s.name AS store_name, r.position, r.published, r.rating, r.consented_at").Find(&drafts).Error)
	require.Len(t, drafts, 3)
	var names []string
	for _, draft := range drafts {
		names = append(names, draft.StoreName)
		require.Equal(t, "Store owner", draft.Position)
		require.False(t, draft.Published)
		require.Equal(t, 5, draft.Rating)
		require.Nil(t, draft.ConsentedAt)
	}
	require.ElementsMatch(t, []string{"Mugshot Studio", "Buttercup Bakes", "Daily Basket"}, names)
	r := gin.New()
	NewHandler(db, nil).RegisterPublic(r.Group("/api/v1"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/testimonials", nil))
	require.Equal(t, 200, w.Code)
	require.JSONEq(t, `{"data":[]}`, w.Body.String())
}
