// Command backfill-vendors ensures every existing tenant has a self-vendor
// row in marketplace-api, replacing the placeholder name/slug written by
// migration 000027 with the real tenant name + its default-store slug.
//
// Phase 1 of the tenant/vendor/store refactor. See
// docs/superpowers/specs/2026-04-14-tenant-vendor-store-architecture-design.md
//
// Run once against prod after the Helm image rolls out. Safe to re-run.
//
// Usage:
//
//	DATABASE_URL=...   MARKETPLACE_API_URL=... go run ./cmd/backfill-vendors
//
// Exits 0 on complete success; 1 if any tenant failed (error per-tenant is
// logged and the run continues).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/marketplaceapi"
	"github.com/mark8ly/platform-api/internal/store"
	"github.com/mark8ly/platform-api/internal/tenant"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	apiURL := os.Getenv("MARKETPLACE_API_URL")
	if apiURL == "" {
		log.Fatal("MARKETPLACE_API_URL is required")
	}

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("db open: %v", err)
	}

	vc := marketplaceapi.NewVendorClient(apiURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var tenants []tenant.Tenant
	if err := db.WithContext(ctx).Find(&tenants).Error; err != nil {
		log.Fatalf("list tenants: %v", err)
	}

	ok, skipped, failed := 0, 0, 0
	for _, t := range tenants {
		// Each tenant's slug comes from its first (oldest) store.
		// Tenants with zero stores haven't completed onboarding; skip.
		var s store.Store
		err := db.WithContext(ctx).
			Where("tenant_id = ?", t.ID).
			Order("created_at ASC").
			First(&s).Error
		if err != nil {
			log.Printf("tenant %s (%s): no store; skipping", t.ID, t.Name)
			skipped++
			continue
		}
		if _, err := vc.EnsureSelfVendor(ctx, t.ID, t.Name, s.Slug); err != nil {
			log.Printf("tenant %s (%s): ensure failed: %v", t.ID, t.Name, err)
			failed++
			continue
		}
		if _, err := vc.UpdateSelfVendor(ctx, t.ID, t.Name, s.Slug); err != nil {
			log.Printf("tenant %s (%s): update failed: %v", t.ID, t.Name, err)
			failed++
			continue
		}
		ok++
	}
	fmt.Printf("backfill complete: %d ok, %d skipped (no store), %d failed\n", ok, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}
