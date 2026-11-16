//go:build integration

package testimonial

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	platformapi "github.com/mark8ly/platform-api"
	"github.com/mark8ly/platform-api/pkg/testdb"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestIntegrationUnregisteredDraftNeverAppearsPublicly(t *testing.T) {
	db := testdb.NewTx(t)
	var before int64
	require.NoError(t, db.Table("stores").Count(&before).Error)
	require.NoError(t, db.Exec(`INSERT INTO merchant_testimonials (store_name, position, quote, rating) VALUES ('Unregistered test store', 'Store owner', 'Proposed draft copy.', 5)`).Error)
	var after int64
	require.NoError(t, db.Table("stores").Count(&after).Error)
	require.Equal(t, before, after, "drafts must not create merchant accounts")
	err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(`UPDATE merchant_testimonials SET published = true, consented_at = now(), submitted_by = 'test' WHERE store_name = 'Unregistered test store'`).Error
	})
	require.Error(t, err, "an unregistered draft cannot be published")
	r := gin.New()
	NewHandler(db, nil).RegisterPublic(r.Group("/api/v1"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/testimonials", nil))
	require.Equal(t, 200, w.Code)
	require.NotContains(t, w.Body.String(), "Unregistered test store")
	require.NotContains(t, w.Body.String(), "Proposed draft copy.")
}

func TestIntegrationFourRequestedDraftsLinkOnlyExistingStores(t *testing.T) {
	db := testdb.NewTx(t)
	down, err := platformapi.MigrationsFS.ReadFile("migrations/000023_internal_testimonial_drafts.down.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(down)).Error)
	tenantID, storeID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(`INSERT INTO tenants (id, name, owner_user_id, owner_email) VALUES (?, 'Draft test company', ?, ?)`, tenantID, tenantID, tenantID+"@test.local").Error)
	require.NoError(t, db.Exec(`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone) VALUES (?, ?, ?, 'The Bondi Store', 'AU', 'AUD', 'Australia/Sydney')`, storeID, tenantID, "bondi-"+storeID).Error)
	up, err := platformapi.MigrationsFS.ReadFile("migrations/000023_internal_testimonial_drafts.up.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(up)).Error)
	var drafts []struct {
		StoreID   *string
		StoreName string
		Position  string
		Quote     string
		Rating    int
		Published bool
	}
	require.NoError(t, db.Table("merchant_testimonials").Select("store_id, store_name, position, quote, rating, published").Find(&drafts).Error)
	require.Len(t, drafts, 4)
	var names []string
	for _, draft := range drafts {
		names = append(names, draft.StoreName)
		require.Equal(t, "Store owner", draft.Position)
		require.Equal(t, 5, draft.Rating)
		require.False(t, draft.Published)
		require.Contains(t, draft.Quote, draft.StoreName)
		if draft.StoreName == "The Bondi Store" {
			require.NotNil(t, draft.StoreID)
			require.Equal(t, storeID, *draft.StoreID)
		} else {
			require.Nil(t, draft.StoreID)
		}
	}
	require.ElementsMatch(t, []string{"Mugshot Studio", "Buttercup Bakes", "Daily Basket", "The Bondi Store"}, names)
}
