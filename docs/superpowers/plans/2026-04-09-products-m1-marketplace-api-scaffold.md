# Products M1 — marketplace-api service scaffold

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land a new `services/marketplace-api/` Go binary that boots, answers `/health` and `/ready`, connects to a dedicated `marketplace_db` Postgres database, supports the `MODE=admin|storefront|both` engine switch from §14.8 of the spec, and is deployed to the dev cluster via ArgoCD — with nothing else in it yet.

**Architecture:** Mirror the `services/platform-api/` structure byte-for-byte for the scaffolding layers (`pkg/config`, `pkg/db`, `pkg/httpserver`, `pkg/logger`, `pkg/migrate`, `pkg/testdb`). New service owns a new Postgres DB `marketplace_db` on the shared instance, with its own `marketplace_db_schema_migrations` tracking table. Two Knative Services deploy from the same image via the `MODE` env var. No business logic, no middleware beyond logging/recovery, no migrations yet beyond the empty `marketplace_db_schema_migrations` table.

**Tech Stack:** Go 1.26, Gin, GORM, Postgres 15, golang-migrate, envconfig, slog, Alpine 3.19 multi-stage Docker, Knative Serving, Kustomize, ArgoCD, GitHub Actions.

**Spec reference:** `docs/superpowers/specs/2026-04-09-products-feature-slice-1-design.md` — authoritative layers are §14 → §13 → §§1–12 (in priority order). Relevant sections for this milestone: §3.1 (service layout), §13.1.3 (stores projection — data model only, table creation in M2), §13.8 (M1 infra prerequisites), §14.8 (two-Knative-Services deployment shape).

**Out of scope for M1** (handled by later milestones): schema migrations beyond the empty tracking table (M2), product/category/media business logic (M3), FGA bootstrap (M4), HTTP handlers (M5/M6), admin UI (M7).

---

## Decisions locked for this milestone

1. **Module path:** `github.com/mark8ly/marketplace-api` — mirrors `github.com/mark8ly/platform-api`.
2. **Database name:** `marketplace_db` — matches spec §13.8 verbatim. Added to the `POSTGRES_MULTIPLE_DATABASES` env of the dev Postgres container alongside `platform_api`, `auth_bff`, `openfga`. Local dev user is `dev` (docker-compose convention); the production user `marketplace_user` is created by infra tooling outside this plan.
3. **Per-service migrations tracking table:** `marketplace_db_schema_migrations`. Isolated from platform-api's.
4. **Local dev port:** `:8087` (platform-api is `:8086`, auth-bff is `:8088`). In `MODE=both` both Gin engines share this single port; in cluster deployments each Knative Service runs in a single mode on its own port.
5. **Scaffolding duplication:** `pkg/config`, `pkg/db`, `pkg/logger`, `pkg/migrate`, `pkg/testdb` are **copied from platform-api** verbatim (with the module path adjusted). `pkg/httpserver` is **adapted**, not copied, because marketplace-api's two-engine (`Engines`) design diverges from platform-api's single-engine `New` signature — see Task 3 for the new code and the instruction to **not** copy platform-api's `server_test.go`. Inter-service compile-time coupling is explicitly forbidden by the architecture decision. A future `pkg/go-shared` extraction is tracked as a slice-2+ refactor when a third service emerges.
6. **MODE env:** `admin`, `storefront`, or `both`. Default `both` for local dev; Knative manifests in the infra repo set `admin` and `storefront` explicitly per service.
7. **No migrations in M1.** `migrations/` directory is empty; `migrate up` creates only `marketplace_db_schema_migrations`. The first real migration (`0001_products_initial`) lands in M2.
8. **Authentication middleware is not wired in M1.** `/health` and `/ready` are unauthenticated. The GIP middleware factory is imported but only used when real admin routes land in M5.

---

## File structure produced by M1

```
services/marketplace-api/
├── cmd/
│   ├── marketplace-api/
│   │   └── main.go                   # Gin server (admin / storefront / both)
│   └── migrate/
│       └── main.go                   # golang-migrate CLI
├── internal/
│   ├── health/
│   │   ├── handler.go                # GET /health, GET /ready
│   │   └── handler_test.go
│   └── mode/
│       ├── mode.go                   # MODE parsing + validation
│       └── mode_test.go
├── pkg/
│   ├── config/
│   │   ├── config.go                 # envconfig-based loader
│   │   └── config_test.go
│   ├── db/
│   │   └── db.go                     # GORM open with retry
│   ├── httpserver/
│   │   └── server.go                 # Gin engine factory + middleware
│   ├── logger/
│   │   └── logger.go                 # slog JSON/text
│   ├── migrate/
│   │   └── migrate.go                # golang-migrate wrapper
│   └── testdb/
│       └── testdb.go                 # Per-test TX rollback helper
├── migrations/
│   └── .gitkeep                      # Empty in M1
├── migrations.go                     # go:embed migrations/*.sql + ExpectedSchemaVersion
├── go.mod
├── go.sum
├── Dockerfile
├── .dockerignore
└── README.md
```

Infra-side files (in `tesserix-infra` repo) produced by M1:

```
tesserix-infra/k8s/apps/marketplace/marketplace-api-admin/
├── kustomization.yaml
├── knative-service.yaml
├── service-account.yaml
└── external-secret.yaml

tesserix-infra/k8s/apps/marketplace/marketplace-api-storefront/
├── kustomization.yaml
└── knative-service.yaml             # Same image, MODE=storefront

tesserix-infra/k8s/argocd/appsets/services.yaml
                                     # Add marketplace-api-admin + marketplace-api-storefront entries
```

Monorepo-level:

```
infra/dev/docker-compose.yml         # Add marketplace_db to POSTGRES_MULTIPLE_DATABASES + new service block
infra/dev/.env.local.example         # Add marketplace-api env vars
.github/workflows/ci.yml             # Add marketplace-api to the Go matrix + its own job
```

---

## Task decomposition

15 tasks total. Tasks 1–9 are local codebase changes. Tasks 10–13 touch the `tesserix-infra` repo. Tasks 14–15 are verification gates.

Each task is one logical commit.

---

### Task 1: Go module scaffold + pkg/logger + pkg/config

**Files:**
- Create: `services/marketplace-api/go.mod`
- Create: `services/marketplace-api/pkg/logger/logger.go`
- Create: `services/marketplace-api/pkg/config/config.go`
- Create: `services/marketplace-api/pkg/config/config_test.go`
- Create: `services/marketplace-api/internal/mode/mode.go`
- Create: `services/marketplace-api/internal/mode/mode_test.go`

- [ ] **Step 1.1: Initialize the Go module**

```bash
cd services/marketplace-api
go mod init github.com/mark8ly/marketplace-api
go mod edit -go=1.26
```

The `go mod edit -go=1.26` step pins the module to Go 1.26 regardless of the toolchain version the engineer has installed locally. Without this, `go mod init` writes whatever version the host toolchain defaults to, and CI (which builds with 1.26) will diverge from local.

Then pin dependencies to match `services/platform-api/go.mod` exactly. Copy the relevant `require` lines from `platform-api/go.mod` for these packages (versions must match; the easiest way is to open `services/platform-api/go.mod` and copy the lines verbatim):

- `github.com/gin-gonic/gin`
- `github.com/joho/godotenv`
- `github.com/kelseyhightower/envconfig`
- `github.com/golang-migrate/migrate/v4`
- `gorm.io/driver/postgres`
- `gorm.io/gorm`
- `github.com/stretchr/testify`

Run:

```bash
go mod tidy
```

Expected: `go.sum` is written, no errors.

- [ ] **Step 1.2: Copy `pkg/logger/logger.go` from platform-api**

Copy `services/platform-api/pkg/logger/logger.go` to `services/marketplace-api/pkg/logger/logger.go` with **no changes** — the file uses only stdlib `log/slog` and has no package-specific imports.

- [ ] **Step 1.3: Write `internal/mode/mode.go`**

```go
// Package mode owns the MODE env var parsing and validation.
// MODE selects which Gin engine(s) the marketplace-api binary constructs
// on startup: admin, storefront, or both. Two Knative Services deploy
// the same image with MODE=admin and MODE=storefront respectively; local
// dev uses MODE=both on a single port for convenience.
package mode

import "fmt"

// Mode is the deployment mode.
type Mode string

const (
	Admin      Mode = "admin"
	Storefront Mode = "storefront"
	Both       Mode = "both"
)

// Parse returns a Mode from a raw string, defaulting to Both when empty.
// Returns an error on any other unknown value.
func Parse(raw string) (Mode, error) {
	switch raw {
	case "":
		return Both, nil
	case string(Admin), string(Storefront), string(Both):
		return Mode(raw), nil
	default:
		return "", fmt.Errorf("mode: unknown MODE value %q (want admin|storefront|both)", raw)
	}
}

// RunsAdmin reports whether this mode serves admin routes.
func (m Mode) RunsAdmin() bool { return m == Admin || m == Both }

// RunsStorefront reports whether this mode serves storefront routes.
func (m Mode) RunsStorefront() bool { return m == Storefront || m == Both }
```

- [ ] **Step 1.4: Write `internal/mode/mode_test.go`**

```go
package mode

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		raw     string
		want    Mode
		wantErr bool
	}{
		{"", Both, false},
		{"admin", Admin, false},
		{"storefront", Storefront, false},
		{"both", Both, false},
		{"Admin", "", true},      // case-sensitive
		{"nonsense", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := Parse(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse(%q) err = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestRunsAdminAndStorefront(t *testing.T) {
	tests := []struct {
		m            Mode
		runsAdmin    bool
		runsStorefront bool
	}{
		{Admin, true, false},
		{Storefront, false, true},
		{Both, true, true},
	}
	for _, tt := range tests {
		if got := tt.m.RunsAdmin(); got != tt.runsAdmin {
			t.Errorf("%v.RunsAdmin() = %v, want %v", tt.m, got, tt.runsAdmin)
		}
		if got := tt.m.RunsStorefront(); got != tt.runsStorefront {
			t.Errorf("%v.RunsStorefront() = %v, want %v", tt.m, got, tt.runsStorefront)
		}
	}
}
```

- [ ] **Step 1.5: Run mode tests**

```bash
cd services/marketplace-api
go test ./internal/mode/...
```

Expected: `PASS` (no coverage reporting needed at this point).

- [ ] **Step 1.6: Write `pkg/config/config.go`**

Start from `services/platform-api/pkg/config/config.go` and trim it to the minimal set marketplace-api needs for M1:

```go
// Package config loads runtime configuration from environment variables.
package config

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds all runtime configuration for marketplace-api.
//
// MODE selects which Gin engine(s) the binary constructs — see
// internal/mode. Default is "both" for local dev. Knative Services in
// the infra repo set this explicitly per service.
type Config struct {
	Env         string `envconfig:"ENV" default:"dev"`
	Mode        string `envconfig:"MODE" default:"both"`
	HTTPPort    int    `envconfig:"HTTP_PORT" default:"8087"`
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
}

// Load reads .env (if present) and binds environment variables into Config.
func Load() (*Config, error) {
	_ = godotenv.Load() // .env is optional

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
```

- [ ] **Step 1.7: Write `pkg/config/config_test.go`**

```go
package config

import (
	"os"
	"testing"
)

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() with no DATABASE_URL = nil, want error")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://x/y")
	defer os.Unsetenv("DATABASE_URL")
	os.Unsetenv("ENV")
	os.Unsetenv("MODE")
	os.Unsetenv("HTTP_PORT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want dev", cfg.Env)
	}
	if cfg.Mode != "both" {
		t.Errorf("Mode = %q, want both", cfg.Mode)
	}
	if cfg.HTTPPort != 8087 {
		t.Errorf("HTTPPort = %d, want 8087", cfg.HTTPPort)
	}
}
```

- [ ] **Step 1.8: Run config tests**

```bash
go test ./pkg/config/...
```

Expected: `PASS`.

- [ ] **Step 1.9: Run the full test suite to ensure nothing is broken**

```bash
go test ./...
```

Expected: `PASS` across `internal/mode/...` and `pkg/config/...`.

- [ ] **Step 1.10: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/go.mod services/marketplace-api/go.sum \
        services/marketplace-api/pkg/logger services/marketplace-api/pkg/config \
        services/marketplace-api/internal/mode
git commit -m "feat(marketplace-api): Go module scaffold with config + mode + logger"
```

---

### Task 2: pkg/db + pkg/migrate + migrations embed

**Files:**
- Create: `services/marketplace-api/pkg/db/db.go`
- Create: `services/marketplace-api/pkg/migrate/migrate.go`
- Create: `services/marketplace-api/migrations.go`
- Create: `services/marketplace-api/migrations/.gitkeep`

- [ ] **Step 2.1: Copy `pkg/db/db.go` from platform-api**

Copy `services/platform-api/pkg/db/db.go` to `services/marketplace-api/pkg/db/db.go` with **no changes**.

- [ ] **Step 2.2: Copy `pkg/migrate/migrate.go` from platform-api, changing the migrations table name**

Copy `services/platform-api/pkg/migrate/migrate.go` to `services/marketplace-api/pkg/migrate/migrate.go`, then change this line:

```go
const migrationsTable = "platform_api_schema_migrations"
```

to:

```go
const migrationsTable = "marketplace_db_schema_migrations"
```

Leave everything else identical.

- [ ] **Step 2.3: Create `migrations.go` with the embed directive and expected version**

```go
package marketplaceapi

import "embed"

// MigrationsFS embeds all SQL migration files.
// Both cmd/marketplace-api (for AssertVersion on startup) and cmd/migrate
// (for Up/Down/Version) read from here.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// ExpectedSchemaVersion is the migration version the current code was
// written against. cmd/marketplace-api refuses to start if the database's
// migration state doesn't match. M1 runs against version 0 (empty schema,
// only the marketplace_db_schema_migrations table exists). Bump this with
// every migration.
const ExpectedSchemaVersion uint = 0
```

- [ ] **Step 2.4: Create empty migrations directory**

```bash
mkdir -p services/marketplace-api/migrations
touch services/marketplace-api/migrations/.gitkeep
```

The `//go:embed migrations/*.sql` directive will match zero files in M1, which is intentional — `migrate up` will create the tracking table and report "no change". **Important:** Go's `embed` does not error on zero matches as long as the pattern is a glob and the directory exists; confirm by running `go build ./...` after creating the directory.

- [ ] **Step 2.5: Verify the build**

```bash
cd services/marketplace-api
go build ./...
```

Expected: no errors. The `embed` directive with zero matching files is legal.

- [ ] **Step 2.6: Commit**

```bash
git add services/marketplace-api/pkg/db services/marketplace-api/pkg/migrate \
        services/marketplace-api/migrations.go services/marketplace-api/migrations/.gitkeep
git commit -m "feat(marketplace-api): embed migrations + DB + migrate wrappers"
```

---

### Task 3: pkg/httpserver + internal/health

**Files:**
- Create: `services/marketplace-api/pkg/httpserver/server.go`
- Create: `services/marketplace-api/internal/health/handler.go`
- Create: `services/marketplace-api/internal/health/handler_test.go`

- [ ] **Step 3.1: Write `pkg/httpserver/server.go`**

⚠ **Do not copy `services/platform-api/pkg/httpserver/server_test.go`.** Platform-api's test calls `httpserver.New("test", log)` (two arguments), which is incompatible with marketplace-api's three-argument `New(env, m, log)`. Use it only as a reference for how to test a Gin engine factory; write a new test in Step 3.3b below.

Start from `services/platform-api/pkg/httpserver/server.go` and adapt. Unlike platform-api, marketplace-api constructs engines per `Mode`, so `New` takes a `mode.Mode` and returns an `Engines` struct with per-mode fields so the caller can listen on the appropriate port(s) and assemble route groups cleanly.

```go
// Package httpserver provides Gin setup with the conventions used across
// mark8ly services. marketplace-api runs two engines — admin and
// storefront — on the same port in MODE=both (local dev) or on one port
// per Knative Service in prod.
package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/mode"
)

// Engines holds the Gin engines constructed for the current mode.
// Admin is nil when mode does not run admin; same for Storefront.
type Engines struct {
	Admin      *gin.Engine
	Storefront *gin.Engine
}

// New constructs the Gin engines appropriate for the given mode. Shared
// /health and /ready handlers are mounted on every active engine. Request
// logging and panic recovery are applied uniformly.
func New(env string, m mode.Mode, log *slog.Logger) Engines {
	if env != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	build := func(label string) *gin.Engine {
		r := gin.New()
		r.Use(gin.Recovery())
		r.Use(requestLogger(log.With(slog.String("engine", label))))
		return r
	}

	var e Engines
	if m.RunsAdmin() {
		e.Admin = build("admin")
	}
	if m.RunsStorefront() {
		e.Storefront = build("storefront")
	}
	return e
}

// MergedForBoth returns a single engine hosting both admin and storefront
// route groups. Used only in MODE=both for local dev convenience where we
// listen on a single port. In production each mode runs its own process
// on its own Knative Service, so MergedForBoth is not called.
func MergedForBoth(env string, log *slog.Logger) *gin.Engine {
	if env != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger(log))
	return r
}

func requestLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("dur", time.Since(start)),
		)
	}
}

// OK returns a 200 JSON response with a canonical body. Use for /health.
func OK(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

- [ ] **Step 3.2: Write `internal/health/handler.go`**

```go
// Package health owns /health and /ready handlers.
//
// /health  — liveness. Returns 200 if the process is up. No DB check.
// /ready   — readiness. Returns 200 only when the DB is reachable via a
//             cheap `SELECT 1`. Used by Knative and the dev docker-compose
//             healthcheck.
package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler is the health/ready HTTP handler.
type Handler struct {
	db *gorm.DB
}

// New constructs a Handler bound to the given *gorm.DB. db may be nil in
// tests that only exercise /health.
func New(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// Register mounts /health and /ready on the given engine.
func (h *Handler) Register(r gin.IRouter) {
	r.GET("/health", h.health)
	r.GET("/ready", h.ready)
}

func (h *Handler) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) ready(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_unavailable"})
		return
	}
	sqlDB, err := h.db.DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_unavailable", "error": err.Error()})
		return
	}
	if err := sqlDB.Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_unreachable", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

- [ ] **Step 3.3: Write `internal/health/handler_test.go`**

```go
package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHealthEndpoint_ReturnsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	New(nil).Register(r)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}

func TestReadyEndpoint_WithNilDB_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	New(nil).Register(r)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ready", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
```

- [ ] **Step 3.3b: Write a minimal `pkg/httpserver/server_test.go` smoke test**

```go
package httpserver

import (
	"io"
	"log/slog"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/mode"
)

func TestNew_Admin_PopulatesOnlyAdminEngine(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := New("test", mode.Admin, log)
	if e.Admin == nil {
		t.Error("Admin engine should be non-nil for mode.Admin")
	}
	if e.Storefront != nil {
		t.Error("Storefront engine should be nil for mode.Admin")
	}
}

func TestNew_Storefront_PopulatesOnlyStorefrontEngine(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := New("test", mode.Storefront, log)
	if e.Admin != nil {
		t.Error("Admin engine should be nil for mode.Storefront")
	}
	if e.Storefront == nil {
		t.Error("Storefront engine should be non-nil for mode.Storefront")
	}
}

func TestMergedForBoth_ReturnsNonNil(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if MergedForBoth("test", log) == nil {
		t.Error("MergedForBoth should return a non-nil engine")
	}
}
```

- [ ] **Step 3.4: Run health + httpserver tests**

```bash
cd services/marketplace-api
go test ./internal/health/... ./pkg/httpserver/...
```

Expected: `PASS`.

- [ ] **Step 3.5: Commit**

```bash
git add services/marketplace-api/pkg/httpserver services/marketplace-api/internal/health
git commit -m "feat(marketplace-api): httpserver + health/ready handlers"
```

---

### Task 4: cmd/marketplace-api/main.go

**Files:**
- Create: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 4.1: Write `cmd/marketplace-api/main.go`**

```go
// Command marketplace-api is the marketplace-api HTTP entrypoint.
//
// It does NOT run migrations. On startup it asserts the DB is at the
// expected schema version and refuses to start otherwise — the safety
// net that guarantees the API never runs against a wrong schema.
//
// MODE selects which Gin engine(s) to construct. In production, two
// Knative Services run the same image with MODE=admin and MODE=storefront
// respectively. In local dev, MODE=both (the default) runs a single
// process with both engines mounted on one port.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	marketplaceapi "github.com/mark8ly/marketplace-api"
	"github.com/mark8ly/marketplace-api/internal/health"
	"github.com/mark8ly/marketplace-api/internal/mode"
	"github.com/mark8ly/marketplace-api/pkg/config"
	"github.com/mark8ly/marketplace-api/pkg/db"
	"github.com/mark8ly/marketplace-api/pkg/httpserver"
	"github.com/mark8ly/marketplace-api/pkg/logger"
	"github.com/mark8ly/marketplace-api/pkg/migrate"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.Env)

	m, err := mode.Parse(cfg.Mode)
	if err != nil {
		log.Error("invalid MODE", "err", err)
		os.Exit(2)
	}
	log.Info("boot", slog.String("mode", string(m)), slog.Int("port", cfg.HTTPPort))

	// Verify schema version. Refuse to start on mismatch.
	mig, err := migrate.New(marketplaceapi.MigrationsFS, "migrations", cfg.DatabaseURL)
	if err != nil {
		log.Error("migrate init", "err", err)
		os.Exit(1)
	}
	if err := mig.AssertVersion(marketplaceapi.ExpectedSchemaVersion); err != nil {
		log.Error("schema version mismatch — run `make mp-migrate-up` first", "err", err)
		os.Exit(1)
	}

	// Open DB.
	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Error("db open", "err", err)
		os.Exit(1)
	}

	// Construct Gin engine(s) per MODE.
	healthHandler := health.New(conn)

	var srv *http.Server
	switch m {
	case mode.Both:
		// Single engine for local dev: both admin and storefront route
		// groups mount on one port so a developer can curl either without
		// running two processes.
		r := httpserver.MergedForBoth(cfg.Env, log)
		healthHandler.Register(r)
		// Future: admin/storefront route groups mount here in M5/M6.
		srv = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
			Handler: r,
		}
	case mode.Admin, mode.Storefront:
		e := httpserver.New(cfg.Env, m, log)
		engine := e.Admin
		if m == mode.Storefront {
			engine = e.Storefront
		}
		healthHandler.Register(engine)
		srv = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
			Handler: engine,
		}
	}

	// Start the server in a goroutine so we can signal-handle on the main.
	go func() {
		log.Info("listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown", "err", err)
		os.Exit(1)
	}
	log.Info("bye")
}
```

- [ ] **Step 4.2: Build it**

```bash
cd services/marketplace-api
go build ./cmd/marketplace-api/
```

Expected: produces a `marketplace-api` binary in the current directory with no errors.

- [ ] **Step 4.3: Commit**

```bash
git add services/marketplace-api/cmd/marketplace-api
git commit -m "feat(marketplace-api): main entrypoint with graceful shutdown + MODE switch"
```

---

### Task 5: cmd/migrate/main.go

**Files:**
- Create: `services/marketplace-api/cmd/migrate/main.go`

- [ ] **Step 5.1: Copy and adapt platform-api's `cmd/migrate/main.go`**

Copy `services/platform-api/cmd/migrate/main.go` to `services/marketplace-api/cmd/migrate/main.go`. Change these imports:

```go
platformapi "github.com/mark8ly/platform-api"
"github.com/mark8ly/platform-api/pkg/migrate"
```

to:

```go
marketplaceapi "github.com/mark8ly/marketplace-api"
"github.com/mark8ly/marketplace-api/pkg/migrate"
```

And anywhere the code references `platformapi.MigrationsFS`, change it to `marketplaceapi.MigrationsFS`.

Leave all other logic identical — the CLI commands (`up`, `down N`, `version`) and flag parsing should match platform-api exactly so operators can use the same muscle memory.

- [ ] **Step 5.2: Build it**

```bash
cd services/marketplace-api
go build ./cmd/migrate/
```

Expected: no errors, `migrate` binary produced.

- [ ] **Step 5.3: Commit**

```bash
git add services/marketplace-api/cmd/migrate
git commit -m "feat(marketplace-api): migrate CLI binary"
```

---

### Task 6: pkg/testdb

**Files:**
- Create: `services/marketplace-api/pkg/testdb/testdb.go`

- [ ] **Step 6.1: Copy `pkg/testdb/testdb.go` from platform-api**

Copy `services/platform-api/pkg/testdb/testdb.go` to `services/marketplace-api/pkg/testdb/testdb.go` with **no changes**. The package is self-contained and depends only on GORM + stdlib `testing`. The `TEST_DATABASE_URL` env var it reads is service-agnostic by design — the setup script points it at the marketplace-api test DB when running marketplace-api tests.

- [ ] **Step 6.2: Verify the build still passes**

```bash
cd services/marketplace-api
go build ./...
```

Expected: no errors.

- [ ] **Step 6.3: Commit**

```bash
git add services/marketplace-api/pkg/testdb
git commit -m "feat(marketplace-api): testdb helper for per-test transaction rollback"
```

---

### Task 7: Wire marketplace_db into dev Postgres + migrate up smoke test

**Files:**
- Modify: `infra/dev/docker-compose.yml`
- Modify: `infra/dev/postgres-init.sh` (if it exists, to add the new user grant)
- Create/modify: `infra/dev/.env.local.example`

- [ ] **Step 7.1: Add `marketplace_db` to the dev Postgres multi-DB env**

Open `infra/dev/docker-compose.yml`. Find the `postgres` service block and update:

```yaml
POSTGRES_MULTIPLE_DATABASES: platform_api,auth_bff,openfga
```

to:

```yaml
POSTGRES_MULTIPLE_DATABASES: platform_api,auth_bff,openfga,marketplace_db
```

- [ ] **Step 7.2: Verify `postgres-init.sh` creates the DB correctly**

Open `infra/dev/postgres-init.sh`. If it iterates over the comma-separated list and runs `CREATE DATABASE`, no change is needed. If it hardcodes database names, add `marketplace_db` explicitly. (Likely it iterates — the existing one supports platform_api, auth_bff, openfga through the same mechanism.)

- [ ] **Step 7.3: Add a marketplace-api compose service block**

Below the existing `platform-api` service block in `infra/dev/docker-compose.yml`, add:

```yaml
  marketplace-api-migrate:
    build:
      context: ../../services/marketplace-api
      dockerfile: Dockerfile
      target: migrate
    environment:
      DATABASE_URL: postgres://dev:dev@postgres:5432/marketplace_db?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    command: ["up"]
    restart: "no"

  marketplace-api:
    build:
      context: ../../services/marketplace-api
      dockerfile: Dockerfile
      target: runtime
    environment:
      ENV: dev
      MODE: both
      HTTP_PORT: 8087
      DATABASE_URL: postgres://dev:dev@postgres:5432/marketplace_db?sslmode=disable
    depends_on:
      marketplace-api-migrate:
        condition: service_completed_successfully
    ports:
      - "8087:8087"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8087/health"]
      interval: 5s
      timeout: 3s
      retries: 30
```

Note: the `Dockerfile` with `migrate` and `runtime` build targets is created in Task 8. This compose block will fail to build until Task 8 lands; that's intentional — Task 7 and Task 8 are committed together in Task 8's commit to avoid a broken intermediate state.

**Do not commit yet.** Leave the compose file dirty.

- [ ] **Step 7.4: Update `.env.local.example`**

Append:

```bash
# marketplace-api
MARKETPLACE_API_DATABASE_URL=postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable
MARKETPLACE_API_PORT=8087
```

---

### Task 8: Dockerfile (multi-stage: migrate + runtime targets)

**Files:**
- Create: `services/marketplace-api/Dockerfile`
- Create: `services/marketplace-api/.dockerignore`

- [ ] **Step 8.1: Write the Dockerfile**

Start from `services/platform-api/Dockerfile` as a reference and adapt. The Dockerfile must produce two tagged build targets — `migrate` (for the init container) and `runtime` (for the long-running service).

```dockerfile
# syntax=docker/dockerfile:1

# ─── builder ────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/marketplace-api ./cmd/marketplace-api
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# ─── migrate ────────────────────────────────────────────────────────────
FROM alpine:3.19 AS migrate
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/migrate /usr/local/bin/migrate
ENTRYPOINT ["/usr/local/bin/migrate"]

# ─── runtime ────────────────────────────────────────────────────────────
FROM alpine:3.19 AS runtime
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /out/marketplace-api /usr/local/bin/marketplace-api
EXPOSE 8087
ENTRYPOINT ["/usr/local/bin/marketplace-api"]
```

- [ ] **Step 8.2: Write `.dockerignore`**

```
.git
.github
docs
tests
**/*_test.go
Dockerfile
.dockerignore
README.md
```

- [ ] **Step 8.3: Build the image locally**

```bash
cd services/marketplace-api
docker build --target runtime -t marketplace-api:dev .
docker build --target migrate -t marketplace-api-migrate:dev .
```

Expected: both builds succeed. The final image sizes should be small (~20–30 MB each).

- [ ] **Step 8.4: Start the full dev stack and verify marketplace-api comes up**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
docker compose -f infra/dev/docker-compose.yml up -d postgres marketplace-api-migrate marketplace-api
```

Expected:
- `postgres` becomes healthy
- `marketplace-api-migrate` runs, logs `no change` on stdout (no `.sql` files exist yet — `go:embed migrations/*.sql` is a legal zero-match glob, `migrate up` creates only the `marketplace_db_schema_migrations` tracking table, reports `no change`, and exits `0`)
- `marketplace-api` starts, logs `listening addr=:8087` and `mode=both`

- [ ] **Step 8.5: Curl the health endpoint**

```bash
curl -sS http://localhost:8087/health
```

Expected: `{"status":"ok"}`

```bash
curl -sS http://localhost:8087/ready
```

Expected: `{"status":"ok"}`

- [ ] **Step 8.6: Verify the tracking table was created**

```bash
docker compose -f infra/dev/docker-compose.yml exec postgres \
    psql -U dev -d marketplace_db -c "\dt"
```

Expected output includes `marketplace_db_schema_migrations` with zero rows.

- [ ] **Step 8.7: Tear down**

```bash
docker compose -f infra/dev/docker-compose.yml stop marketplace-api marketplace-api-migrate
```

- [ ] **Step 8.8: Commit (Task 7 + Task 8 together)**

```bash
git add services/marketplace-api/Dockerfile services/marketplace-api/.dockerignore \
        infra/dev/docker-compose.yml infra/dev/.env.local.example
# infra/dev/postgres-init.sh only if it was modified
git add infra/dev/postgres-init.sh 2>/dev/null || true
git commit -m "feat(marketplace-api): Dockerfile + dev compose wiring"
```

---

### Task 9: README.md

**Files:**
- Create: `services/marketplace-api/README.md`

- [ ] **Step 9.1: Write the README**

```markdown
# marketplace-api

The marketplace runtime service. Consolidates what used to be ~20 `mp-*`
microservices into a single Go binary with per-domain internal packages.

Slice 1 scope: products, categories, and media. See
`docs/superpowers/specs/2026-04-09-products-feature-slice-1-design.md` for
the authoritative design.

## Local development

Run the dev stack from the repo root:

```bash
make dev
```

This brings up Postgres, OpenFGA, auth-bff, platform-api, and marketplace-api
via docker-compose. marketplace-api listens on `http://localhost:8087`.

Verify:

```bash
curl http://localhost:8087/health   # {"status":"ok"}
curl http://localhost:8087/ready    # {"status":"ok"}
```

## Binaries

- `cmd/marketplace-api` — the HTTP server. Reads `DATABASE_URL`, `MODE`,
  `HTTP_PORT`, `ENV` from env.
- `cmd/migrate` — golang-migrate CLI. Supports `up`, `down N`, `version`.

## MODE

`MODE` selects which Gin engine(s) the binary constructs on startup:

| MODE        | Admin engine | Storefront engine |
|-------------|--------------|-------------------|
| `admin`     | yes          | no                |
| `storefront`| no           | yes               |
| `both`      | yes          | yes               |

`both` is the default and is used in local dev. In the dev/prod cluster,
two Knative Services deploy the same image with `MODE=admin` and
`MODE=storefront` respectively. See
`docs/superpowers/specs/2026-04-09-products-feature-slice-1-design.md` §14.8.

## Scaffolding duplication

`pkg/config`, `pkg/db`, `pkg/logger`, `pkg/httpserver`, `pkg/migrate`, and
`pkg/testdb` are copy-pasted from `services/platform-api/pkg/`. This is
deliberate: inter-service compile-time coupling between microservice
runtimes is explicitly forbidden by the architecture decision.

When a third service emerges that needs the same scaffolding, extract
these packages into a shared `pkg/go-shared` Go module. Until then,
tolerate the duplication and keep the copies in sync manually if either
platform-api or marketplace-api needs a scaffolding change.

## Tests

```bash
go test ./...                                    # unit tests only
TEST_DATABASE_URL=postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable \
  go test -tags=integration ./...                # integration tests (when they exist)
```

## Database

- DB name: `marketplace_db` (on the shared dev Postgres)
- User: `dev` (dev only) / `marketplace_user` (prod)
- Migrations tracking table: `marketplace_db_schema_migrations`
- Slice 1 schema: see M2 plan (not yet landed as of M1 completion)
```

- [ ] **Step 9.2: Commit**

```bash
git add services/marketplace-api/README.md
git commit -m "docs(marketplace-api): README with local dev + MODE + scaffolding notes"
```

---

### Task 10: Kustomize overlay — marketplace-api-admin

> ⚠ Tasks 10–13 touch the `tesserix-infra` repo, not the `mark8ly` monorepo. They require cluster access and may be executed by a developer with infra permissions. If you are running this plan from an agent session, stop here and hand off to a human for infra work, then resume at Task 14.

**Files (in the `tesserix-infra` repo):**
- Create: `k8s/apps/marketplace/marketplace-api-admin/kustomization.yaml`
- Create: `k8s/apps/marketplace/marketplace-api-admin/knative-service.yaml`
- Create: `k8s/apps/marketplace/marketplace-api-admin/service-account.yaml`
- Create: `k8s/apps/marketplace/marketplace-api-admin/external-secret.yaml`

- [ ] **Step 10.0: Verify infra repo layout before starting**

Paths in Tasks 10–12 were written based on documented conventions — confirm them on the actual `tesserix-infra` checkout before executing:

```bash
cd tesserix-infra
ls k8s/apps/platform/                 # expect: a platform-api/ directory
ls k8s/apps/                          # confirm the marketplace/ namespace dir exists or create it
ls k8s/argocd/appsets/                # expect: services.yaml (the ApplicationSet registration target)
```

If any path differs (for example the platform-api overlay lives under a different grouping), update every subsequent path in Tasks 10–12 accordingly before executing the tasks. Do not guess — read the actual files and match what's there.

- [ ] **Step 10.1: Locate the closest existing reference**

Read `tesserix-infra/k8s/apps/platform/platform-api/` in full. The marketplace-api-admin overlay should mirror it with these differences:

- Service name: `marketplace-api-admin`
- Image: `asia-south1-docker.pkg.dev/tesserix-app/services/marketplace-api:<tag>`
- Env vars: add `MODE=admin`, `HTTP_PORT=8080`
- ExternalSecret references a new `marketplace-api-db-password` secret in GCP Secret Manager
- Knative `autoscaling.knative.dev/maxScale: "1"` (per spec §14.8 — slice 1 rate limiter)

- [ ] **Step 10.2: Write `knative-service.yaml`**

Use platform-api's `knative-service.yaml` as a literal starting point. Change every `platform-api` reference to `marketplace-api-admin`, change the image repository, change the database secret references to `marketplace-api-db-password`, add the two new env vars, and add the `maxScale` annotation.

Concretely, the env block must include:

```yaml
env:
  - name: ENV
    value: dev
  - name: MODE
    value: admin
  - name: HTTP_PORT
    value: "8080"
  - name: DATABASE_URL
    valueFrom:
      secretKeyRef:
        name: marketplace-api-db
        key: url
```

And the annotation (per spec §14.16 DoD — must reference a slice-2 ticket for the Redis rate limiter upgrade):

```yaml
annotations:
  # slice 1: in-memory rate limiter — per spec §14.8 + §14.16.
  # TODO(slice-2): unpin once Redis-backed rate limiter ships. Tracking: <SLICE2_TICKET_TBD>
  autoscaling.knative.dev/maxScale: "1"
```

Replace `<SLICE2_TICKET_TBD>` with the actual slice-2 ticket ID when that ticket exists. Until then the placeholder is acceptable — the important thing is the inline reference so a future reviewer can trace the constraint back to its rationale.

- [ ] **Step 10.3: Write `service-account.yaml`**

Copy platform-api's `service-account.yaml`, change the name to `marketplace-api-admin`. The Workload Identity binding annotation should reference a new GCP service account `marketplace-api@tesserix-app.iam.gserviceaccount.com` (which must be created in GCP by an infra admin as a prerequisite — tracked as a Task 10.0).

- [ ] **Step 10.4: Write `external-secret.yaml`**

Spec §13.8 item 4 requires the ExternalSecret to reference **two** secrets from GCP Secret Manager: the DB password AND the `X-Storefront-Key` shared secret used for storefront trust-boundary enforcement (spec §13.1.2). The `X-Storefront-Key` is provisioned in slice 1 even though it's only read by the storefront engine starting in M6 — creating the K8s secret up front is cheap and avoids a second infra PR later.

Naming layers to keep straight:

| Layer | Name |
|---|---|
| GCP Secret Manager secret (DB password) | `marketplace-api-db-password` |
| GCP Secret Manager secret (storefront shared key) | `marketplace-api-storefront-key` |
| K8s Secret produced by the ExternalSecret | `marketplace-api-db` |
| K8s Secret data keys | `url`, `storefront_key` |

Explicit YAML (adapt names/refreshInterval/secretStoreRef to match what platform-api uses in your actual tesserix-infra checkout):

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: marketplace-api-db
  namespace: marketplace
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: gcp-secret-manager       # match platform-api's ClusterSecretStore name
    kind: ClusterSecretStore
  target:
    name: marketplace-api-db       # K8s Secret produced at the cluster
    template:
      engineVersion: v2
      data:
        url: "postgres://marketplace_user:{{ .dbPassword }}@127.0.0.1:5432/marketplace_db?sslmode=disable"
        storefront_key: "{{ .storefrontKey }}"
  data:
    - secretKey: dbPassword
      remoteRef:
        key: marketplace-api-db-password
    - secretKey: storefrontKey
      remoteRef:
        key: marketplace-api-storefront-key
```

The database URL template points at `127.0.0.1:5432` because the Cloud SQL Auth Proxy runs as a sidecar on the same pod. If platform-api uses a different address shape (for example, a Unix socket path), match that instead.

The knative-service.yaml env block from Step 10.2 must additionally read the `storefront_key` from this secret, even though only `marketplace-api-storefront` consumes it at runtime. Adding a `STOREFRONT_KEY` env var on the admin service is harmless (the admin engine doesn't read it) and keeps both services consuming a single shared Secret:

```yaml
env:
  # ... DATABASE_URL as before
  - name: STOREFRONT_KEY
    valueFrom:
      secretKeyRef:
        name: marketplace-api-db
        key: storefront_key
```

**GCP prerequisite (Task 10.0 companion):** an infra admin must create the two GCP Secret Manager secrets (`marketplace-api-db-password`, `marketplace-api-storefront-key`) before the ExternalSecret can sync. Generate the storefront key with `openssl rand -base64 48`. Log the creation in the PR description so it's auditable.

- [ ] **Step 10.5: Write `kustomization.yaml`**

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: marketplace
resources:
  - service-account.yaml
  - external-secret.yaml
  - knative-service.yaml
```

- [ ] **Step 10.6: Validate with kustomize**

```bash
cd tesserix-infra
kustomize build k8s/apps/marketplace/marketplace-api-admin/
```

Expected: valid YAML output, no errors.

- [ ] **Step 10.7: Commit (in tesserix-infra)**

```bash
cd tesserix-infra
git add k8s/apps/marketplace/marketplace-api-admin
git commit -m "feat(infra): marketplace-api-admin Kustomize overlay"
```

---

### Task 11: Kustomize overlay — marketplace-api-storefront

**Files (in the `tesserix-infra` repo):**
- Create: `k8s/apps/marketplace/marketplace-api-storefront/kustomization.yaml`
- Create: `k8s/apps/marketplace/marketplace-api-storefront/knative-service.yaml`

- [ ] **Step 11.1: Write `knative-service.yaml`**

Same image, different name (`marketplace-api-storefront`), `MODE=storefront` env. **No `maxScale: 1` annotation** — storefront scales normally. ExternalSecret and ServiceAccount are shared with the admin overlay via kustomize references in the next step.

- [ ] **Step 11.2: Write `kustomization.yaml`**

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: marketplace
resources:
  - ../marketplace-api-admin/service-account.yaml
  - ../marketplace-api-admin/external-secret.yaml
  - knative-service.yaml
```

Reusing the service-account and external-secret from the admin overlay keeps them as single-source-of-truth rather than duplicated.

- [ ] **Step 11.3: Validate**

```bash
kustomize build k8s/apps/marketplace/marketplace-api-storefront/
```

- [ ] **Step 11.4: Commit**

```bash
git add k8s/apps/marketplace/marketplace-api-storefront
git commit -m "feat(infra): marketplace-api-storefront Kustomize overlay"
```

---

### Task 12: ArgoCD registration

**Files (in the `tesserix-infra` repo):**
- Modify: `k8s/argocd/appsets/services.yaml`

- [ ] **Step 12.1: Add marketplace-api-admin and marketplace-api-storefront to the ApplicationSet**

Open `k8s/argocd/appsets/services.yaml`. Find the marketplace-services ApplicationSet block (or the generator list that enumerates services). Add two entries:

```yaml
- service: marketplace-api-admin
  namespace: marketplace
- service: marketplace-api-storefront
  namespace: marketplace
```

The exact format depends on the generator style already in use — match the existing entries verbatim.

- [ ] **Step 12.2: Commit**

```bash
git add k8s/argocd/appsets/services.yaml
git commit -m "feat(infra): register marketplace-api services with ArgoCD"
```

- [ ] **Step 12.3: Push to the infra repo**

```bash
git push origin main
```

---

### Task 13: First image push + ArgoCD sync

This task is performed in a terminal with GCP credentials configured.

- [ ] **Step 13.1: Build and push the image to GAR**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api

gcloud auth configure-docker asia-south1-docker.pkg.dev

IMAGE=asia-south1-docker.pkg.dev/tesserix-app/services/marketplace-api
TAG=m1-scaffold-$(git rev-parse --short HEAD)

docker build --target runtime -t "$IMAGE:$TAG" -t "$IMAGE:latest" .
docker push "$IMAGE:$TAG"
docker push "$IMAGE:latest"
```

Expected: two tags pushed successfully.

- [ ] **Step 13.2: Update the Knative manifests to reference the new tag**

In the `tesserix-infra` repo, update both `knative-service.yaml` files (admin + storefront) to pin the image tag:

```yaml
image: asia-south1-docker.pkg.dev/tesserix-app/services/marketplace-api:m1-scaffold-<shortsha>
```

Commit and push:

```bash
cd tesserix-infra
git add k8s/apps/marketplace/marketplace-api-admin/knative-service.yaml \
        k8s/apps/marketplace/marketplace-api-storefront/knative-service.yaml
git commit -m "chore(infra): pin marketplace-api image to m1-scaffold-<shortsha>"
git push origin main
```

- [ ] **Step 13.3: Trigger ArgoCD sync**

```bash
argocd app sync marketplace-api-admin
argocd app sync marketplace-api-storefront
argocd app wait marketplace-api-admin --sync --health --timeout 300
argocd app wait marketplace-api-storefront --sync --health --timeout 300
```

Expected: both apps reach `Synced + Healthy`.

- [ ] **Step 13.4: Verify /health is reachable from inside the cluster**

```bash
kubectl -n marketplace run curl-test --rm -it --restart=Never --image=curlimages/curl -- \
    curl -sS http://marketplace-api-admin.marketplace.svc.cluster.local/health
```

Expected: `{"status":"ok"}`

```bash
kubectl -n marketplace run curl-test --rm -it --restart=Never --image=curlimages/curl -- \
    curl -sS http://marketplace-api-storefront.marketplace.svc.cluster.local/health
```

Expected: `{"status":"ok"}`

- [ ] **Step 13.5: Verify the two services started in different modes**

```bash
kubectl -n marketplace logs -l serving.knative.dev/service=marketplace-api-admin --tail 20 | grep '"mode"'
kubectl -n marketplace logs -l serving.knative.dev/service=marketplace-api-storefront --tail 20 | grep '"mode"'
```

Expected: admin logs contain `"mode":"admin"`, storefront logs contain `"mode":"storefront"`.

---

### Task 14: CI scaffold

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 14.1: Add marketplace-api to the Go matrix**

Open `.github/workflows/ci.yml`. Find the existing `matrix.service: [platform-api, auth-bff]` line and change it to:

```yaml
matrix:
  service: [platform-api, auth-bff, marketplace-api]
```

The rest of the `go` job body should already generalize over `matrix.service` — no other changes needed if the existing job is parametrized. If it isn't, compare against how platform-api and auth-bff are handled and replicate verbatim for marketplace-api.

- [ ] **Step 14.2: Pin the FGA + Postgres + fake-gcs-server versions in the job env**

Marketplace-api tests will eventually spin up real Postgres + FGA + fake-gcs-server containers (M3+). Pin them now so slice 1 never drifts. These env vars go at the **job level** (under `jobs.go:`, as a sibling of `runs-on:` and `strategy:`) so every matrix entry sees the same pinned versions. They do **not** go at the workflow top level (which would leak into unrelated jobs) and they do **not** go inside `strategy.matrix` (which is for dimension values, not env).

Diff-style example (the engineer applies this to wherever `jobs.go:` is declared in `ci.yml`):

```yaml
jobs:
  go:
    name: Go (${{ matrix.service }})
    runs-on: ubuntu-latest
+   env:
+     POSTGRES_IMAGE: postgres:15-alpine
+     FGA_IMAGE: openfga/openfga:v1.8.4
+     FAKE_GCS_IMAGE: fsouza/fake-gcs-server:1.49.3
    strategy:
      matrix:
        service: [platform-api, auth-bff, marketplace-api]
```

The pinned versions match the existing dev compose (`infra/dev/docker-compose.yml`) for Postgres and FGA. `fake-gcs-server` is pinned to a known-good recent version; adjust to the current latest stable when running this task if a newer patch release exists. Steps that spin up those containers (added in M3+ milestones, not this one) will reference the env vars by name.

- [ ] **Step 14.3: Validate the workflow locally**

```bash
actionlint .github/workflows/ci.yml
```

If `actionlint` is not installed, skim the file by eye and confirm the YAML is well-formed.

- [ ] **Step 14.4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add marketplace-api to Go test matrix with pinned Postgres/FGA/GCS versions"
```

- [ ] **Step 14.5: Push and verify green on GitHub**

```bash
git push origin main
```

Open the Actions tab on GitHub. Verify the `Go (marketplace-api)` matrix entry passes. Expected: all tests green (only the config, mode, health, and httpserver tests from Tasks 1–3 exist).

---

### Task 15: M1 verification + exit checklist

This task produces no commits; it verifies every M1 exit criterion from the spec.

- [ ] **Step 15.1: Local verification**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
docker compose -f infra/dev/docker-compose.yml up -d marketplace-api
curl -sS http://localhost:8087/health
curl -sS http://localhost:8087/ready
docker compose -f infra/dev/docker-compose.yml stop marketplace-api
```

Expected: both endpoints return `{"status":"ok"}`.

- [ ] **Step 15.2: Dev cluster verification**

```bash
kubectl -n marketplace get ksvc
kubectl -n marketplace get externalsecret
argocd app get marketplace-api-admin | grep -E "(Sync Status|Health Status)"
argocd app get marketplace-api-storefront | grep -E "(Sync Status|Health Status)"
```

Expected:
- Two Knative Services listed (`marketplace-api-admin`, `marketplace-api-storefront`) both `Ready`
- ExternalSecret `marketplace-api-db` in `Ready` state
- Both ArgoCD apps: `Synced + Healthy`

- [ ] **Step 15.3: Confirm every spec M1 exit criterion**

Check each against the implementation:

- [ ] `services/marketplace-api/` service binary and Dockerfile (Task 1–4, 8)
- [ ] `marketplace_db` database provisioned in dev Postgres (Task 7)
- [ ] `marketplace_db_schema_migrations` tracking table exists (Task 8 Step 6)
- [ ] Kustomize overlays for admin + storefront (Tasks 10–11)
- [ ] ExternalSecret referencing `marketplace-api-db-password` (Task 10.4)
- [ ] ArgoCD Application registration (Task 12)
- [ ] Dev-cluster deployment running and `/health` returns 200 at both services (Task 13.4)
- [ ] `marketplace-api-admin` has `maxScale: 1` annotation (Task 10.2)
- [ ] CI workflow scaffolded with pinned Postgres/FGA/GCS (Task 14)
- [ ] `/ready` verifies DB connectivity (Task 3, Task 8)
- [ ] MODE switch tested in three configurations (Task 1.5 + Task 13.5)
- [ ] README documents local dev + MODE + scaffolding-duplication decision (Task 9)
- [ ] Graceful shutdown implemented (Task 4)
- [ ] Per-service migrations table `marketplace_db_schema_migrations` (Task 2.2)
- [ ] Module path `github.com/mark8ly/marketplace-api` (Task 1.1)
- [ ] `go test ./...` passes on the whole module (Task 14 CI verification)

- [ ] **Step 15.4: Tag the milestone**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git tag -a m1-marketplace-api-scaffold -m "M1: marketplace-api scaffold complete — spec §13.8 exit criteria met"
git push origin m1-marketplace-api-scaffold
```

- [ ] **Step 15.5: Update the milestone tracker**

If there is a project tracker (spec §12 Definition of Done, or a project board), check off M1. Announce in whatever channel normally tracks milestone completion that M1 is done and M2 is cleared to start.

---

## Parallelization notes (for `subagent-driven-development`)

When executing this plan with multiple agents, the following tasks can run in parallel without conflict:

- **Tasks 1, 2, 3** can be parallelized because they touch different directories (`pkg/config`, `pkg/db`, `pkg/httpserver`) and the cross-package imports are simple stdlib + gorm. They must merge before Task 4 (`cmd/marketplace-api/main.go`) runs.
- **Task 5** (cmd/migrate) can run in parallel with Task 4 after Tasks 1–3 merge.
- **Task 6** (pkg/testdb) can run at any point after Task 1 (it only depends on GORM).
- **Task 9** (README) can run in parallel with Tasks 7–8 once the code patterns are settled.
- **Tasks 10, 11** can run in parallel (two independent overlay directories) once a human confirms the `tesserix-infra` repo is ready.
- **Task 14** (CI) can run in parallel with Tasks 10–13 because it touches a different repo layer.

Tasks **4, 7+8, 12, 13** are strictly serial — each depends on the previous completing.

---

## Estimated effort

Rough single-developer estimate, not including infra-repo access time or GCP IAM setup:

| Task | Effort |
|---|---|
| 1. Module + config + mode | 45 min |
| 2. DB + migrate + embed | 20 min |
| 3. httpserver + health | 30 min |
| 4. main.go | 40 min |
| 5. migrate CLI | 15 min |
| 6. testdb | 5 min |
| 7+8. Compose + Dockerfile | 45 min |
| 9. README | 15 min |
| 10. Admin overlay | 45 min |
| 11. Storefront overlay | 20 min |
| 12. ArgoCD registration | 15 min |
| 13. Image push + sync | 30 min |
| 14. CI scaffold | 20 min |
| 15. Verification | 15 min |
| **Total** | **~6 hours** |

A full day including GCP IAM setup (creating the `marketplace-api` GCP service account, the `marketplace-api-db-password` secret, and the Workload Identity binding) if those don't already exist.

---

## Exit gate to M2

M2 (schema migration + domain models) is cleared to start when:

1. Task 15 verification checklist is fully green
2. The `m1-marketplace-api-scaffold` tag exists in git
3. The dev-cluster `marketplace-api-admin` and `marketplace-api-storefront` both serve `/health` successfully
4. The CI `Go (marketplace-api)` matrix job is green on `main`
5. This plan document is committed to `docs/superpowers/plans/`

At that point, a follow-up plan is written for M2 following the same structure: spec-as-authoritative, bite-sized tasks, no inter-milestone assumptions.
