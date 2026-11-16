//go:build integration

package testimonial

import (
	"encoding/json"
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

func TestIntegrationUnlinkedReviewAppearsOnlyOncePublished(t *testing.T) {
	db := testdb.NewTx(t)
	var before int64
	require.NoError(t, db.Table("stores").Count(&before).Error)
	var id string
	require.NoError(t, db.Raw(`INSERT INTO merchant_testimonials (store_name, position, quote, rating) VALUES ('Offline test store', 'Store owner', 'Offline store review.', 5) RETURNING id`).Scan(&id).Error)
	var after int64
	require.NoError(t, db.Table("stores").Count(&after).Error)
	require.Equal(t, before, after, "unlinked reviews must not create merchant accounts")
	r := gin.New()
	NewHandler(db, nil).RegisterPublic(r.Group("/api/v1"))
	readPublic := func() string {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/testimonials", nil))
		require.Equal(t, 200, w.Code, w.Body.String())
		return w.Body.String()
	}
	require.NotContains(t, readPublic(), "Offline store review.", "unpublished drafts stay private")
	require.NoError(t, db.Exec(`UPDATE merchant_testimonials SET published = true, consented_at = now(), submitted_by = 'store-manager' WHERE id = ?`, id).Error)
	var body struct{ Data []PublicReview }
	require.NoError(t, json.Unmarshal([]byte(readPublic()), &body))
	require.Contains(t, body.Data, PublicReview{StoreID: id, StoreName: "Offline test store", Position: "Store owner", Quote: "Offline store review.", Rating: 5})
	require.NotContains(t, readPublic(), "store-manager")
}

func TestIntegrationUnlinkedReviewStillNeedsStoreName(t *testing.T) {
	db := testdb.NewTx(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(`INSERT INTO merchant_testimonials (position, quote, rating, published, consented_at, submitted_by) VALUES ('Store owner', 'Nameless review.', 5, true, now(), 'test')`).Error
	})
	require.Error(t, err)
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
