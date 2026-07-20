// Command backfill-gip-claims stamps the tenant_id GIP custom claim onto
// every existing tenant owner who is missing it.
//
// Why: marketplace-api's mobile bearer auth resolves a caller's tenant
// solely from the tenant_id custom claim on the GIP ID token
// (services/marketplace-api/internal/auth/gip_verifier.go). Nothing ever
// wrote that claim, so owners created through the onboarding wizard could
// authenticate but had every mobile API call refused — surfacing as an
// endless bounce back to the login screen. The web admin was unaffected
// because it resolves the tenant from the database.
//
// Onboarding now enqueues this write via the outbox for NEW tenants; this
// command repairs the ones created before that fix.
//
// Idempotent: EnsureTenantClaim merges into existing claims and skips
// accounts that already carry the tenant. Safe to re-run.
//
// Usage:
//
//	DATABASE_URL=... GIP_PROJECT_ID=... GIP_TENANT_ID=... GIP_WEB_API_KEY=... \
//	  go run ./cmd/backfill-gip-claims [-dry-run]
//
// Exits 0 on complete success; 1 if any owner failed (each failure is
// logged and the run continues).
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/gipadmin"
	"github.com/mark8ly/platform-api/internal/tenant"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "list what would change without writing")
	flag.Parse()

	dbURL := mustEnv("DATABASE_URL")
	cfg := gipadmin.Config{
		ProjectID: mustEnv("GIP_PROJECT_ID"),
		TenantID:  mustEnv("GIP_TENANT_ID"),
		WebAPIKey: mustEnv("GIP_WEB_API_KEY"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	admin, err := gipadmin.New(ctx, cfg)
	if err != nil {
		log.Fatalf("gipadmin init: %v", err)
	}

	var tenants []tenant.Tenant
	if err := db.WithContext(ctx).Find(&tenants).Error; err != nil {
		log.Fatalf("list tenants: %v", err)
	}
	log.Printf("found %d tenant(s)", len(tenants))

	var failed int
	for _, t := range tenants {
		if t.OwnerUserID == "" {
			log.Printf("SKIP  %-24s (%s) — no owner_user_id", t.Name, t.ID)
			continue
		}
		if *dryRun {
			log.Printf("DRY   %-24s owner=%s tenant=%s", t.Name, t.OwnerUserID, t.ID)
			continue
		}
		if err := admin.EnsureTenantClaim(ctx, t.OwnerUserID, t.ID); err != nil {
			// Keep going: one broken account must not block the rest.
			log.Printf("FAIL  %-24s owner=%s: %v", t.Name, t.OwnerUserID, err)
			failed++
			continue
		}
		log.Printf("OK    %-24s owner=%s tenant=%s", t.Name, t.OwnerUserID, t.ID)
	}

	if failed > 0 {
		log.Printf("completed with %d failure(s)", failed)
		os.Exit(1)
	}
	log.Print("all tenant owners have their tenant_id claim")
	log.Print("NOTE: owners must sign out and back in — claims only refresh on a new ID token")
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s is required", k)
	}
	return v
}
