package main

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
)

// TestPaymentRows_ScopeMapping pins Domain "payment" with the row's own
// provider column, and Field literals api_key/secret_key/webhook_secret —
// see internal/handlers/admin/settings.go:551-558.
func TestPaymentRows_ScopeMapping(t *testing.T) {
	id := uuid.New()
	tenant := uuid.New()
	row := paymentGatewayConfigRow{
		ID:                     id,
		TenantID:               tenant,
		Provider:               "razorpay",
		APIKeyEncrypted:        "gsm://projects/p/secrets/x",
		SecretKeyEncrypted:     "gsm://projects/p/secrets/y",
		WebhookSecretEncrypted: "gsm://projects/p/secrets/z",
	}
	got := paymentRows(row)
	if len(got) != 3 {
		t.Fatalf("len(paymentRows) = %d, want 3", len(got))
	}
	wantFields := []string{"api_key", "secret_key", "webhook_secret"}
	for i, field := range wantFields {
		wantScope := carriersecrets.Scope{TenantID: tenant.String(), Domain: "payment", Provider: "razorpay", Field: field}
		if got[i].Scope != wantScope {
			t.Fatalf("row %d scope = %+v, want %+v", i, got[i].Scope, wantScope)
		}
		// encodeSegment escapes a literal '_' to "__" to stay injective (#606).
		wantPath := "kv/mark8ly/marketplace-api/tenants/" + tenant.String() + "/payment/razorpay/" + strings.ReplaceAll(field, "_", "__")
		if gotPath := carriersecrets.BaoPath(got[i].Scope); gotPath != wantPath {
			t.Fatalf("BaoPath(%+v) = %q, want %q", got[i].Scope, gotPath, wantPath)
		}
		if got[i].Table != "payment_gateway_configs" {
			t.Fatalf("row %d table = %q, want payment_gateway_configs", i, got[i].Table)
		}
	}
}

// TestShippingRows_ScopeMapping pins Domain "shipping" with the row's own
// `provider` column (confirmed NOT `carrier` against
// internal/shipping/repository.go's CarrierConfig struct).
func TestShippingRows_ScopeMapping(t *testing.T) {
	id := uuid.New()
	tenant := uuid.New()
	row := shippingCarrierConfigRow{
		ID:                 id,
		TenantID:           tenant,
		Provider:           "delhivery",
		APIKeyEncrypted:    "gsm://projects/p/secrets/x",
		SecretKeyEncrypted: "gsm://projects/p/secrets/y",
	}
	got := shippingRows(row)
	if len(got) != 2 {
		t.Fatalf("len(shippingRows) = %d, want 2", len(got))
	}
	wantFields := []string{"api_key", "secret_key"}
	for i, field := range wantFields {
		wantScope := carriersecrets.Scope{TenantID: tenant.String(), Domain: "shipping", Provider: "delhivery", Field: field}
		if got[i].Scope != wantScope {
			t.Fatalf("row %d scope = %+v, want %+v", i, got[i].Scope, wantScope)
		}
		// encodeSegment escapes a literal '_' to "__" to stay injective (#606).
		wantPath := "kv/mark8ly/marketplace-api/tenants/" + tenant.String() + "/shipping/delhivery/" + strings.ReplaceAll(field, "_", "__")
		if gotPath := carriersecrets.BaoPath(got[i].Scope); gotPath != wantPath {
			t.Fatalf("BaoPath(%+v) = %q, want %q", got[i].Scope, gotPath, wantPath)
		}
		if got[i].Table != "shipping_carrier_configs" {
			t.Fatalf("row %d table = %q, want shipping_carrier_configs", i, got[i].Table)
		}
	}
}

// TestTaxRows_ScopeMapping pins Domain "tax", Field "api_key", provider
// from the row's own `provider` column.
func TestTaxRows_ScopeMapping(t *testing.T) {
	id := uuid.New()
	tenant := uuid.New()
	row := taxProviderConfigRow{
		ID:              id,
		TenantID:        tenant,
		Provider:        "taxjar",
		APIKeyEncrypted: "gsm://projects/p/secrets/x",
	}
	got := taxRows(row)
	if len(got) != 1 {
		t.Fatalf("len(taxRows) = %d, want 1", len(got))
	}
	wantScope := carriersecrets.Scope{TenantID: tenant.String(), Domain: "tax", Provider: "taxjar", Field: "api_key"}
	if got[0].Scope != wantScope {
		t.Fatalf("scope = %+v, want %+v", got[0].Scope, wantScope)
	}
	// encodeSegment escapes a literal '_' to "__" to stay injective (#606).
	wantPath := "kv/mark8ly/marketplace-api/tenants/" + tenant.String() + "/tax/taxjar/api__key"
	if gotPath := carriersecrets.BaoPath(got[0].Scope); gotPath != wantPath {
		t.Fatalf("BaoPath = %q, want %q", gotPath, wantPath)
	}
	if got[0].Table != "tax_provider_configs" {
		t.Fatalf("table = %q, want tax_provider_configs", got[0].Table)
	}
}

// TestDomainRows_ScopeMapping is the trap case: Domain "platform", Provider
// fixed "cloudflare", and Field is the row's FQDN — NOT a fixed field name
// — per internal/domain/service.go's scopeForCFToken (150-155). Also pins
// the exact sanitized BaoPath: BaoPath lower-cases and replaces every
// non-alnum/_/- character (the FQDN's dots) with '_'.
func TestDomainRows_ScopeMapping(t *testing.T) {
	id := uuid.New()
	tenant := uuid.New()
	fqdn := "shop.example.com"
	row := customDomainRow{
		ID:                  id,
		TenantID:            tenant,
		Domain:              fqdn,
		CFAPITokenEncrypted: "gsm://projects/p/secrets/x",
	}
	got := domainRows(row)
	if len(got) != 1 {
		t.Fatalf("len(domainRows) = %d, want 1", len(got))
	}
	wantScope := carriersecrets.Scope{TenantID: tenant.String(), Domain: "platform", Provider: "cloudflare", Field: fqdn}
	if got[0].Scope != wantScope {
		t.Fatalf("scope = %+v, want %+v", got[0].Scope, wantScope)
	}
	// FQDN dots are hex-escaped by encodeSegment ('.' -> "_2E") to stay
	// injective (#606) — a literal '_' would otherwise be indistinguishable
	// from an escape sequence, and two distinct FQDNs could collide.
	wantPath := "kv/mark8ly/marketplace-api/tenants/" + tenant.String() + "/platform/cloudflare/shop_2Eexample_2Ecom"
	if gotPath := carriersecrets.BaoPath(got[0].Scope); gotPath != wantPath {
		t.Fatalf("BaoPath = %q, want %q", gotPath, wantPath)
	}
	if got[0].Table != "custom_domains" || got[0].Column != "cf_api_token_encrypted" {
		t.Fatalf("table/column = %q/%q, want custom_domains/cf_api_token_encrypted", got[0].Table, got[0].Column)
	}
}
