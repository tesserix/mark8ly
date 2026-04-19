//go:build integration

package internalsvc_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/internalsvc"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type seedStore struct {
	tenantID, storeID uuid.UUID
	slug              string
}

func seedStoreFixture(t *testing.T, db *gorm.DB, slug string) seedStore {
	t.Helper()
	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO stores
			(id, tenant_id, slug, name, country_code, currency_code, timezone, status, synced_at)
		VALUES (?, ?, ?, 'Acme Roasters', 'US', 'USD', 'UTC', 'active', now())
	`, storeID, tenantID, slug).Error)
	return seedStore{tenantID: tenantID, storeID: storeID, slug: slug}
}

func seedSubscription(t *testing.T, db *gorm.DB, s seedStore, status subscription.SubscriptionStatus) {
	t.Helper()
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions
			(tenant_id, store_id, stripe_customer_id, plan, status,
			 tax_id_validated, tax_id_name_match, current_period_end,
			 created_at, updated_at)
		VALUES (?, ?, 'cus_x', 'starter', ?, false, 'not_checked', ?, now(), now())
	`, s.tenantID, s.storeID, string(status), time.Now().Add(30*24*time.Hour)).Error)
}

func seedBranding(t *testing.T, db *gorm.DB, s seedStore, logoURL, supportEmail string) {
	t.Helper()
	require.NoError(t, db.Exec(`
		INSERT INTO store_branding
			(tenant_id, store_id, logo_url, support_email)
		VALUES (?, ?, ?, ?)
	`, s.tenantID, s.storeID, logoURL, supportEmail).Error)
}

func runRequest(t *testing.T, db *gorm.DB, host string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	internalsvc.NewStorefrontStatusHandler(db).Register(r.Group("/internal"), "")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/storefront-status/"+host, nil))
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) internalsvc.StorefrontStatusResponse {
	t.Helper()
	var out internalsvc.StorefrontStatusResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

func TestStorefrontStatus_PlatformSubdomain_ClosedReturnsBranding(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "store_branding", "stores")
	s := seedStoreFixture(t, db, "acme-test-1")
	seedSubscription(t, db, s, subscription.StatusStoreClosed)
	seedBranding(t, db, s, "https://cdn.mark8ly.com/acme/logo.png", "hi@acmeroasters.com")

	rec := runRequest(t, db, "acme-test-1.mark8ly.com")
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decode(t, rec)
	require.Equal(t, "store_closed", resp.Status)
	require.Equal(t, "starter", resp.Plan)
	require.Equal(t, "Acme Roasters", resp.Branding.Name)
	require.Equal(t, "https://cdn.mark8ly.com/acme/logo.png", resp.Branding.LogoURL)
	require.Equal(t, "hi@acmeroasters.com", resp.Branding.SupportEmail)
}

func TestStorefrontStatus_LiveStore_ReturnsActive(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "store_branding", "stores")
	s := seedStoreFixture(t, db, "live-test-1")
	seedSubscription(t, db, s, subscription.StatusActive)

	rec := runRequest(t, db, "live-test-1.mark8ly.com")
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decode(t, rec)
	require.Equal(t, "active", resp.Status)
	require.Empty(t, resp.Branding.LogoURL, "logo defaults empty when no store_branding row")
}

func TestStorefrontStatus_UnknownHost_404(t *testing.T) {
	db := testdb.NewDB(t)
	rec := runRequest(t, db, "ghost-host.mark8ly.com")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestStorefrontStatus_NestedSubdomain_404(t *testing.T) {
	db := testdb.NewDB(t)
	rec := runRequest(t, db, "preview.acme.mark8ly.com")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestStorefrontStatus_MissingHostParam_400(t *testing.T) {
	db := testdb.NewDB(t)
	r := gin.New()
	internalsvc.NewStorefrontStatusHandler(db).Register(r.Group("/internal"), "")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/storefront-status/", nil))
	require.Equal(t, http.StatusNotFound, rec.Code, "missing param produces a 404 from gin route matching")
}

func TestStorefrontStatus_CustomDomain_Resolves(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "store_branding", "custom_domains", "stores")
	s := seedStoreFixture(t, db, "custom-test-1")
	seedSubscription(t, db, s, subscription.StatusStoreClosed)
	require.NoError(t, db.Exec(`
		INSERT INTO custom_domains
			(tenant_id, store_id, domain, status, cf_api_token_encrypted)
		VALUES (?, ?, 'shop.acme.example', 'active', '')
	`, s.tenantID, s.storeID).Error)

	rec := runRequest(t, db, "shop.acme.example")
	require.Equal(t, http.StatusOK, rec.Code)
	resp := decode(t, rec)
	require.Equal(t, "store_closed", resp.Status)
}

func TestStorefrontStatus_AuthGate_Rejects(t *testing.T) {
	db := testdb.NewDB(t)
	r := gin.New()
	internalsvc.NewStorefrontStatusHandler(db).Register(r.Group("/internal"), "topsecret")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/storefront-status/x.mark8ly.com", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// Sanity: context.Background path doesn't panic when host is uppercase.
func TestStorefrontStatus_HostCaseInsensitive(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "store_branding", "stores")
	s := seedStoreFixture(t, db, "case-test-1")
	seedSubscription(t, db, s, subscription.StatusStoreClosed)

	r := gin.New()
	internalsvc.NewStorefrontStatusHandler(db).Register(r.Group("/internal"), "")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/storefront-status/CASE-TEST-1.MARK8LY.COM", nil).WithContext(context.Background()))
	require.Equal(t, http.StatusOK, rec.Code)
}
