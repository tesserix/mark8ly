// Command break-glass-provision provisions the break-glass account for
// exactly one tenant. Zero break-glass accounts exist anywhere in this
// estate today (mark8ly#642) — Bootstrapper.Provision has existed since
// the OpenBao migration, but nothing has ever called it, and even once
// called, a break-glass principal with no OpenFGA tuple cannot log in
// (see docs/superpowers/plans/2026-09-07-break-glass-activation-v2.md,
// "Decisions taken before planning", D3's amendment). This command closes
// both gaps in one operator-run step:
//
//  1. Write the break-glass secret (password + TOTP) to OpenBao.
//  2. Insert the break_glass_accounts row.
//  3. Grant the synthetic break-glass principal the `admin` role on the
//     tenant in OpenFGA, so apps/admin/middleware.ts's getMe check,
//     marketplace-api's RequireTenantRelation, and auth-bff's
//     CheckMembership gates all admit it.
//
// Steps 1–2 are Bootstrapper.Provision, unchanged. Step 3 is new: it is
// the one FGA write marketplace-api's authz package now exposes
// (internal/authz/client.go's WriteRole), used here and nowhere else.
//
// # Role choice: admin, not owner
//
// The OpenFGA model (infra/openfga/model.fga) is a strict hierarchy —
// owner ⊇ admin ⊇ staff ⊇ viewer, with `member: viewer` — so `admin`
// already satisfies every gate above, plus can_edit_settings /
// can_invite_members / can_manage_stores: everything break-glass exists
// to fix (a broken SSO config) requires. It deliberately withholds the
// one thing `owner` adds beyond that: `tenant_owner: owner from parent`
// on stores, i.e. owner-only reach into store-level resources. Break-glass
// is for regaining admin access when SSO is down, not a way to bypass
// owner-only controls — so it gets the minimum role that unblocks the
// documented gates, not the maximum available.
//
// # Ordering, idempotency, and what happens on partial failure
//
// The three steps are strictly ordered and each is independently
// idempotent, so re-running this command for the same tenant converges
// rather than corrupts state:
//
//   - OpenBao (step 1): Bootstrapper.Provision only calls
//     SecretManager.Upsert on the *first* run — see step 2 below. A
//     second run never reaches it.
//   - DB row (step 2): Provision first checks Repository.GetByTenant; if a
//     row already exists it returns ErrAlreadyProvisioned instead of
//     writing a second secret version or attempting a duplicate INSERT.
//     This command treats ErrAlreadyProvisioned as "steps 1–2 already
//     done" and proceeds to step 3 rather than failing — that's what
//     makes a re-run after a step-3 failure (below) actually recover.
//   - FGA tuple (step 3): authz.Client.WriteRole tolerates OpenFGA's
//     "already exists" validation error and returns nil, so writing the
//     same (user, admin, tenant) tuple twice is a no-op.
//
// If step 3 fails after steps 1–2 succeeded, the result is an account
// that exists but cannot log in — worse than doing nothing only in that
// it is silent unless something says so. This command does NOT roll back
// steps 1–2: the secret and DB row are harmless on their own (nothing
// reads them without the FGA tuple also being present, per the three
// gates above), and unwinding a successful OpenBao write plus DB insert
// to "retry cleanly" would need its own failure handling without
// actually reducing operator effort — the fix either way is "run this
// command again for the same tenant." So instead it exits non-zero with
// an explicit message naming exactly that recovery step, rather than
// leaving a half-provisioned account that looks the same as a fully
// working one from the DB alone.
//
// # Usage
//
//	break-glass-provision -tenant <tenant-uuid>
//
// Reads DATABASE_URL, MARKETPLACE_FGA_API_URL, and the OpenBao
// OPENBAO_ADDR / OPENBAO_ROLE / OPENBAO_KV_MOUNT / SHIPPING_SECRET_STORE /
// ENCRYPTION_MODE / ENCRYPTION_KEY vars — the same OpenBao settings
// cmd/break-glass-rotation already reads, plus the OpenFGA API URL that
// job deliberately does not need. Exits 0 on success (including "already
// fully provisioned"), non-zero otherwise with a message on stderr.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/bao"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	adminhandlers "github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

// env is the env-var surface this command needs. Deliberately not
// pkg/config.Load(): that requires production-only auth secrets
// (MARKETPLACE_INTERNAL_AUTH_SECRET, CUSTOMER_SESSION_SECRET, ...) this
// one-shot operator tool never reads, for the same reason
// cmd/break-glass-rotation defines its own env struct instead of calling
// Load(). Unlike that job, this command DOES need MARKETPLACE_FGA_API_URL
// (step 3 writes a tuple), which is why it can't reuse
// config.LoadCarrierSecretJob() either — that loader exists specifically
// to avoid requiring it.
type env struct {
	DatabaseURL         string `envconfig:"DATABASE_URL" required:"true"`
	FGAAPIURL           string `envconfig:"MARKETPLACE_FGA_API_URL" required:"true"`
	ShippingSecretStore string `envconfig:"SHIPPING_SECRET_STORE" default:"inline"`
	OpenBaoAddr         string `envconfig:"OPENBAO_ADDR" default:"http://openbao-active.openbao.svc.cluster.local:8200"`
	OpenBaoRole         string `envconfig:"OPENBAO_ROLE" default:""`
	OpenBaoKVMount      string `envconfig:"OPENBAO_KV_MOUNT" default:"kv"`
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	tenantFlag := flag.String("tenant", "", "tenant UUID to provision a break-glass account for (required)")
	flag.Parse()

	if err := run(context.Background(), log, *tenantFlag); err != nil {
		log.Error("break-glass-provision: failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger, tenantFlag string) error {
	if strings.TrimSpace(tenantFlag) == "" {
		return errors.New("break-glass-provision: -tenant is required")
	}
	tenantID, err := uuid.Parse(tenantFlag)
	if err != nil {
		return fmt.Errorf("break-glass-provision: -tenant %q is not a valid UUID: %w", tenantFlag, err)
	}

	_ = godotenv.Load() // .env is optional, same as pkg/config.Load
	var e env
	if err := envconfig.Process("", &e); err != nil {
		return fmt.Errorf("break-glass-provision: load config: %w", err)
	}

	// validateShippingSecretStore is unexported on *config.Config, so this
	// command relies on carriersecrets.Build below to fail loudly on a bad
	// SHIPPING_SECRET_STORE / OPENBAO_* combination rather than duplicating
	// that validation here.

	conn, err := db.Open(e.DatabaseURL)
	if err != nil {
		return fmt.Errorf("break-glass-provision: db open: %w", err)
	}

	baoClient, err := bao.New(bao.Config{
		Address:        e.OpenBaoAddr,
		Mount:          e.OpenBaoKVMount,
		KubernetesRole: e.OpenBaoRole,
	})
	if err != nil {
		return fmt.Errorf("break-glass-provision: openbao client init: %w", err)
	}

	repo := breakglass.NewRepository(conn)
	secrets := breakglass.NewSecretManager(breakglass.NewBaoSecretClient(carriersecrets.NewBaoClient(baoClient)))
	bootstrapper := breakglass.NewBootstrapper(repo, secrets)

	// Step 3's client needs the store discovered first — same as
	// cmd/marketplace-api/main.go does at boot.
	discoverCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	storeID, err := authz.DiscoverStoreID(discoverCtx, e.FGAAPIURL, authz.FGAStoreName)
	cancel()
	if err != nil {
		return fmt.Errorf("break-glass-provision: discover FGA store: %w", err)
	}
	if storeID == "" {
		return fmt.Errorf("break-glass-provision: FGA store %q not found at %s",
			authz.FGAStoreName, e.FGAAPIURL)
	}
	fgaClient, err := authz.New(authz.Config{APIURL: e.FGAAPIURL, StoreID: storeID})
	if err != nil {
		return fmt.Errorf("break-glass-provision: new FGA client: %w", err)
	}

	return provisionTenant(ctx, log, bootstrapper, fgaClient, tenantID)
}

// provisionTenant runs the three-step sequence documented at the top of
// this file — secret, then DB row (both via Bootstrapper.Provision), then
// the FGA admin tuple — and is the piece under test: bootstrapper and fga
// are both interfaces/structs satisfied by breakglass_test.go and
// authz.FakeClient in tests, so the ordering, idempotency, and
// failure-message behaviour below is exercised with no live DB, OpenBao,
// or OpenFGA.
func provisionTenant(ctx context.Context, log *slog.Logger, bootstrapper *breakglass.Bootstrapper, fga authz.Client, tenantID uuid.UUID) error {
	// Step 1+2: OpenBao secret, then the DB row. Bootstrapper.Provision
	// enforces that internal ordering and that internal idempotency —
	// see the package doc above and internal/breakglass/bootstrap.go.
	if err := bootstrapper.Provision(ctx, tenantID); err != nil {
		if errors.Is(err, breakglass.ErrAlreadyProvisioned) {
			log.Info("break-glass-provision: secret + DB row already provisioned, proceeding to FGA grant",
				"tenant_id", tenantID)
		} else {
			return fmt.Errorf("break-glass-provision: provision secret+DB row: %w", err)
		}
	} else {
		log.Info("break-glass-provision: wrote secret and DB row", "tenant_id", tenantID)
	}

	// Step 3: OpenFGA grant.
	principal := adminhandlers.BreakGlassUserID(tenantID).String()
	if err := fga.WriteRole(ctx, principal, authz.RoleAdmin, tenantID.String()); err != nil {
		return fmt.Errorf("break-glass-provision: grant FGA admin tuple: %w — "+
			"tenant %s HAS a secret and DB row, but the account CANNOT LOG IN "+
			"until the FGA tuple exists (D3's amendment: three gates refuse a "+
			"UID with no tenant relation). Re-run: "+
			"break-glass-provision -tenant %s — steps 1 and 2 are idempotent "+
			"and will be skipped, and only the FGA grant will be retried",
			err, tenantID, tenantID)
	}

	log.Info("break-glass-provision: done",
		"tenant_id", tenantID,
		"principal", principal,
		"role", authz.RoleAdmin,
	)
	return nil
}
