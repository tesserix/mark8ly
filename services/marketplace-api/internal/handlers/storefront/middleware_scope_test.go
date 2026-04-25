package storefront

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

func TestSessionClaimsMatchesStore(t *testing.T) {
	store := &stores.Store{
		ID:       "store-1",
		TenantID: "tenant-1",
		Slug:     "store-a",
	}

	claims := sessionClaims{
		StoreSlug: "store-a",
		StoreID:   "store-1",
		TenantID:  "tenant-1",
	}
	if !claims.MatchesStore(store) {
		t.Fatal("expected claims to match store")
	}
}

func TestSessionClaimsMatchesStoreRejectsCrossStoreCookie(t *testing.T) {
	store := &stores.Store{
		ID:       "store-2",
		TenantID: "tenant-2",
		Slug:     "store-b",
	}

	claims := sessionClaims{
		StoreSlug: "store-a",
		StoreID:   "store-1",
		TenantID:  "tenant-1",
	}
	if claims.MatchesStore(store) {
		t.Fatal("expected claims for store-a to be rejected on store-b")
	}
}

func TestSessionClaimsMatchesStoreRejectsLegacyUnscopedCookie(t *testing.T) {
	store := &stores.Store{
		ID:       "store-1",
		TenantID: "tenant-1",
		Slug:     "store-a",
	}

	claims := sessionClaims{}
	if claims.MatchesStore(store) {
		t.Fatal("expected unscoped legacy claims to be rejected")
	}
}
