//go:build integration

// Proves gormRowStore's SQL actually matches the live schema — the unit
// tests in rowstore_test.go exercise the pure Scope-mapping functions
// (paymentRows/shippingRows/taxRows/domainRows) against hand-built structs,
// but never touch a real payment_gateway_configs/shipping_carrier_configs/
// tax_provider_configs/custom_domains table. This is exactly the class of
// gap that produced the earlier `carrier` vs `provider` column mistake:
// a struct tag can be wrong and every unit test still passes, because the
// unit tests build the struct value directly instead of scanning it out of
// Postgres.
//
// Gated on TEST_DATABASE_URL, matching this service's established
// integration-test convention (see pkg/testdb and internal/arbitrage's
// openIntegrationDB). Missing it skips loudly rather than passing silently.
package main

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func openIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})
	return db
}

// TestGormRowStore_FetchAllAndUpdateReference_RealSchema seeds one row in
// each of the four tracked tables with a gsm:// reference, fetches through
// gormRowStore, confirms the Scope each row produces resolves to the
// expected BaoPath, then updates the reference and confirms it round-trips
// back out of the real table.
func TestGormRowStore_FetchAllAndUpdateReference_RealSchema(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	paymentID := uuid.New()
	if err := db.Exec(
		`INSERT INTO payment_gateway_configs (id, tenant_id, store_id, provider, api_key_encrypted)
		 VALUES (?, ?, ?, 'razorpay', 'gsm://projects/p/secrets/payment-key')`,
		paymentID, tenantID, storeID,
	).Error; err != nil {
		t.Fatalf("seed payment_gateway_configs: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM payment_gateway_configs WHERE id = ?", paymentID) })

	shippingID := uuid.New()
	if err := db.Exec(
		`INSERT INTO shipping_carrier_configs (id, tenant_id, store_id, provider, api_key_encrypted)
		 VALUES (?, ?, ?, 'delhivery', 'gsm://projects/p/secrets/shipping-key')`,
		shippingID, tenantID, storeID,
	).Error; err != nil {
		t.Fatalf("seed shipping_carrier_configs: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM shipping_carrier_configs WHERE id = ?", shippingID) })

	taxID := uuid.New()
	if err := db.Exec(
		`INSERT INTO tax_provider_configs (id, tenant_id, store_id, provider, api_key_encrypted)
		 VALUES (?, ?, ?, 'taxjar', 'gsm://projects/p/secrets/tax-key')`,
		taxID, tenantID, storeID,
	).Error; err != nil {
		t.Fatalf("seed tax_provider_configs: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM tax_provider_configs WHERE id = ?", taxID) })

	domainID := uuid.New()
	fqdn := "backfill-test-" + storeID.String()[:8] + ".example.com"
	if err := db.Exec(
		`INSERT INTO custom_domains (id, tenant_id, store_id, domain, cf_api_token_encrypted)
		 VALUES (?, ?, ?, ?, 'gsm://projects/p/secrets/cf-token')`,
		domainID, tenantID, storeID, fqdn,
	).Error; err != nil {
		t.Fatalf("seed custom_domains: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM custom_domains WHERE id = ?", domainID) })

	store := newGormRowStore(db)
	rows, err := store.FetchAll(ctx)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	find := func(table, id string) *Row {
		for i := range rows {
			if rows[i].Table == table && rows[i].ID == id {
				return &rows[i]
			}
		}
		return nil
	}

	paymentRow := find("payment_gateway_configs", paymentID.String())
	if paymentRow == nil {
		t.Fatal("seeded payment_gateway_configs row not found by FetchAll")
	}
	wantPaymentScope := carriersecrets.Scope{TenantID: tenantID.String(), Domain: "payment", Provider: "razorpay", Field: "api_key"}
	if paymentRow.Scope != wantPaymentScope {
		t.Fatalf("payment scope = %+v, want %+v", paymentRow.Scope, wantPaymentScope)
	}

	shippingRow := find("shipping_carrier_configs", shippingID.String())
	if shippingRow == nil {
		t.Fatal("seeded shipping_carrier_configs row not found by FetchAll")
	}
	wantShippingScope := carriersecrets.Scope{TenantID: tenantID.String(), Domain: "shipping", Provider: "delhivery", Field: "api_key"}
	if shippingRow.Scope != wantShippingScope {
		t.Fatalf("shipping scope = %+v, want %+v — if Provider is empty, the real column is not named `provider`", shippingRow.Scope, wantShippingScope)
	}

	taxRow := find("tax_provider_configs", taxID.String())
	if taxRow == nil {
		t.Fatal("seeded tax_provider_configs row not found by FetchAll")
	}
	wantTaxScope := carriersecrets.Scope{TenantID: tenantID.String(), Domain: "tax", Provider: "taxjar", Field: "api_key"}
	if taxRow.Scope != wantTaxScope {
		t.Fatalf("tax scope = %+v, want %+v", taxRow.Scope, wantTaxScope)
	}

	domainRow := find("custom_domains", domainID.String())
	if domainRow == nil {
		t.Fatal("seeded custom_domains row not found by FetchAll")
	}
	wantDomainScope := carriersecrets.Scope{TenantID: tenantID.String(), Domain: "platform", Provider: "cloudflare", Field: fqdn}
	if domainRow.Scope != wantDomainScope {
		t.Fatalf("domain scope = %+v, want %+v", domainRow.Scope, wantDomainScope)
	}

	// UpdateReference round-trip: write a bao:// reference into the real
	// row and confirm it reads back.
	newRef := carriersecrets.FormatBaoReference(domainRow.Scope)
	if err := store.UpdateReference(ctx, *domainRow, newRef); err != nil {
		t.Fatalf("UpdateReference: %v", err)
	}
	var gotRef string
	if err := db.Raw("SELECT cf_api_token_encrypted FROM custom_domains WHERE id = ?", domainID).Scan(&gotRef).Error; err != nil {
		t.Fatalf("read back cf_api_token_encrypted: %v", err)
	}
	if gotRef != newRef {
		t.Fatalf("cf_api_token_encrypted after UpdateReference = %q, want %q", gotRef, newRef)
	}
}
