# GET /admin/health Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Serve `GET /api/v1/platform/admin/health` — per-dependency, self-reported product health for the platform console (#289).

**Architecture:** A `DependencyRegistry` drives the payload (copying `KPIRegistry` from #282) so a dependency cannot silently vanish. Four dependencies are measured from tables that record work the system actually did; five are declared `not_instrumented` inline. All measurement is read from Postgres, never from in-process counters. A failed check renders `unknown`, never `ok`, and never fails the endpoint.

**Tech Stack:** Go 1.26, Gin, GORM, testify, Postgres. Package `internal/handlers/platformadmin` (external test package `platformadmin_test`).

**Spec:** `docs/superpowers/specs/2026-08-24-admin-health-design.md`

## Global Constraints

- Envelope is `{"data": {...}}` with **no `pagination`** — this is not a list.
- `dependencies` is an array in registry order, allocated `make([]dependencyRow, 0, len(DependencyRegistry))`. A nil slice marshals to `null` and defeats the caller's `?? []`.
- Timestamps RFC3339 UTC with offset.
- A `not_instrumented` entry carries **no `metrics` key at all** — not `{}`, not zeroes.
- Statuses are exactly `ok`, `degraded`, `unknown`, `not_instrumented`.
- **Every check takes `asOf time.Time` from the caller and compares against that parameter in SQL — never Postgres `now()`.** Trap 6's first instance was a fixture that compared Go's clock to `now()` and landed inside a sub-second tolerance. A caller-supplied `asOf` is what makes an exact-boundary fixture possible at all.
- **Boundary rule, applied uniformly: degraded when `age >= window`**, i.e. `ts <= asOf.Add(-window)`.
- **Boundary fixtures offset by 1 millisecond, never 1 nanosecond.** Postgres `timestamptz` has microsecond resolution; a nanosecond offset truncates, and the "at the boundary" and "just inside" fixtures become the same row.
- Integration tests: `//go:build integration`, external package, run with **`-p 1`** (trap 5), DSN on the **LAN IP** `postgres://dev:dev@192.168.1.110:5432/marketplace_db` — not `localhost`.
- Pre-existing unrelated failures in `internal/subscription` and `internal/billing/trial` (#316/#317): scope runs with `-run`.
- Commits: single-line conventional messages, no signature.

---

### Task 1: Extract `csvjob.OrphanWindow`

The csv threshold must be a *shared* definition, not a copied number. Today `main.go` passes a bare `15*time.Minute` literal and `csvjob` exports no constant, so there is nothing to share yet. This task creates it.

**Files:**
- Modify: `services/marketplace-api/internal/csvjob/worker.go` (near `const MaxImportRows` at line 18)
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go:1373`
- Test: `services/marketplace-api/internal/csvjob/worker_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `csvjob.OrphanWindow` (`time.Duration`, value `15 * time.Minute`). Task 2 reads it.

- [ ] **Step 1: Write the failing test**

Append to `internal/csvjob/worker_test.go`:

```go
// TestOrphanWindowIsFifteenMinutes pins the shared definition of "this job
// is orphaned". The health endpoint (#289) and the startup recovery scan
// both read this constant; if they ever disagree, the console reports a
// job healthy that the recovery scan is about to reset.
func TestOrphanWindowIsFifteenMinutes(t *testing.T) {
	require.Equal(t, 15*time.Minute, csvjob.OrphanWindow)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd services/marketplace-api && go test ./internal/csvjob/ -run TestOrphanWindowIsFifteenMinutes -v
```
Expected: FAIL — `undefined: csvjob.OrphanWindow`.

- [ ] **Step 3: Add the constant**

In `internal/csvjob/worker.go`, beneath `const MaxImportRows = 50000`:

```go
// OrphanWindow is how long a 'running' job may go without a heartbeat
// before the system considers it orphaned. Read by the startup recovery
// scan in cmd/marketplace-api/main.go and by the platform console's
// /admin/health endpoint (#289), so that the two cannot drift into
// disagreeing about the same job.
const OrphanWindow = 15 * time.Minute
```

- [ ] **Step 4: Replace the literal at the call site**

In `cmd/marketplace-api/main.go`, the recovery scan currently reads:

```go
if err := csvjob.RecoverOrphanedJobs(context.Background(), csvRepo, 15*time.Minute, log); err != nil {
```

Change to:

```go
if err := csvjob.RecoverOrphanedJobs(context.Background(), csvRepo, csvjob.OrphanWindow, log); err != nil {
```

- [ ] **Step 5: Run tests and build**

```bash
cd services/marketplace-api && go test ./internal/csvjob/ -run TestOrphanWindow -v && go build ./...
```
Expected: PASS, build clean.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/csvjob/worker.go services/marketplace-api/internal/csvjob/worker_test.go services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "refactor(csvjob): export OrphanWindow so the recovery scan and health endpoint share one definition"
```

---

### Task 2: Health types, registry, and the outbox + csv checks

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/health.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/health_checks.go`
- Test: `services/marketplace-api/internal/handlers/platformadmin/health_checks_integration_test.go`

**Interfaces:**
- Consumes: `csvjob.OrphanWindow` from Task 1.
- Produces, all read by Tasks 3–6:

```go
type OutboxHealth struct {
	Pending                 int64
	OldestPendingAgeSeconds int64
	Errored                 int64
}
type CSVJobsHealth struct {
	Queued                int64
	RunningStaleHeartbeat int64
}
type HealthSource interface {
	Outbox(ctx context.Context, asOf time.Time) (OutboxHealth, error)
	CSVJobs(ctx context.Context, asOf time.Time) (CSVJobsHealth, error)
	CampaignSends(ctx context.Context, asOf time.Time) (CampaignSendsHealth, error)
	StripeWebhooks(ctx context.Context, asOf time.Time) (StripeWebhooksHealth, error)
}
func NewDBHealthSource(db *gorm.DB) HealthSource
```

`CampaignSendsHealth` and `StripeWebhooksHealth` are defined in Task 3; declare them there, not here. To keep this task compiling on its own, `dbHealthSource` in this task implements `CampaignSends` and `StripeWebhooks` as stubs returning `errNotImplementedYet`, replaced in Task 3.

- [ ] **Step 1: Write the failing integration test**

Create `health_checks_integration_test.go`:

```go
//go:build integration

package platformadmin_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// asOf is a fixed instant so every fixture below sits at an exact offset
// from it. Nothing here calls time.Now() — see the plan's global
// constraints on why a caller-supplied asOf is what makes an
// exact-boundary fixture possible.
var healthAsOf = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

func TestOutboxHealthCountsPendingAndErrored(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	src := platformadmin.NewDBHealthSource(db)
	tenant := uuid.NewString()

	// Published: must not count as pending.
	published := healthAsOf.Add(-time.Hour)
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at, published_at)
		VALUES (?, 'product', ?, 'product.created', '{}'::jsonb, ?, ?)`,
		tenant, uuid.NewString(), published, published).Error)

	// Pending, 10 minutes old — the oldest pending row.
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at)
		VALUES (?, 'product', ?, 'product.created', '{}'::jsonb, ?)`,
		tenant, uuid.NewString(), healthAsOf.Add(-10*time.Minute)).Error)

	// Pending and errored, 2 minutes old.
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at, error)
		VALUES (?, 'order', ?, 'order.placed', '{}'::jsonb, ?, 'boom')`,
		tenant, uuid.NewString(), healthAsOf.Add(-2*time.Minute)).Error)

	got, err := src.Outbox(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Pending, "published row must not count as pending")
	require.Equal(t, int64(1), got.Errored)
	require.Equal(t, int64(600), got.OldestPendingAgeSeconds,
		"age must be measured from the caller's asOf, not Postgres now()")
}

// TestCSVJobsStaleHeartbeatIsInclusiveAtTheBoundary puts the fixture ON the
// boundary instant rather than near it. A row exactly OrphanWindow old is
// stale; a row one millisecond younger is not. One millisecond, not one
// nanosecond: timestamptz truncates to microseconds and the two rows would
// otherwise be identical.
func TestCSVJobsStaleHeartbeatIsInclusiveAtTheBoundary(t *testing.T) {
	db := testdb.NewDB(t, "csv_import_jobs")
	src := platformadmin.NewDBHealthSource(db)

	insert := func(status string, heartbeat *time.Time) {
		require.NoError(t, db.Exec(`INSERT INTO csv_import_jobs
			(store_id, user_id, gcs_path, content_hash, status, heartbeat_at)
			VALUES (?, 'u', 'gs://x', 'h', ?, ?)`,
			uuid.NewString(), status, heartbeat).Error)
	}

	exactly := healthAsOf.Add(-csvjob.OrphanWindow)
	justInside := exactly.Add(time.Millisecond)

	insert("running", &exactly)    // stale: age == window
	insert("running", &justInside) // healthy: age < window
	insert("queued", nil)          // queued, never counted as stale

	got, err := src.CSVJobs(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Queued)
	require.Equal(t, int64(1), got.RunningStaleHeartbeat,
		"a heartbeat exactly OrphanWindow old is stale; one millisecond younger is not")
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
cd services/marketplace-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags integration -p 1 ./internal/handlers/platformadmin/ -run 'TestOutboxHealth|TestCSVJobsStale' -v
```
Expected: FAIL — `undefined: platformadmin.NewDBHealthSource`.

- [ ] **Step 3: Write `health.go` — types and registry**

```go
package platformadmin

import (
	"context"
	"time"
)

// Dependency status values. `unknown` exists so a check whose query FAILED
// can never be rendered as `ok` — the same rule the platform-api clients
// enforce with ErrUnavailable: an error and an empty result must never
// collapse into each other. `not_instrumented` is deliberately a separate
// value: "we did not look" and "we looked and the lookup broke" are
// different facts about the system.
const (
	StatusOK              = "ok"
	StatusDegraded        = "degraded"
	StatusUnknown         = "unknown"
	StatusNotInstrumented = "not_instrumented"
)

// Thresholds with no existing authority elsewhere in the system. The two
// heartbeat windows are NOT here — they are read from csvjob.OrphanWindow
// and campaign.StaleDuration, which already govern the recovery scans.
const (
	OutboxPendingThreshold     = 5 * time.Minute
	StripeUnprocessedThreshold = 15 * time.Minute
)

// dependencyKey is one dependency the console may be told about.
type dependencyKey struct {
	Name         string
	Instrumented bool
}

// DependencyRegistry declares EVERY dependency mark8ly knows about, and
// drives the payload — the handler does not decide membership with
// conditionals. Same reasoning as KPIRegistry in kpis.go: a dependency
// must not be able to fall silently out of the response.
//
// The five uninstrumented entries are not omitted and not `ok`. Nothing in
// the system records a last-run for the scheduled jobs, and no outcome log
// exists for the outbound integrations, so any status other than
// not_instrumented would be asserting something nothing records.
// Configuration presence is NOT health: a non-empty STRIPE_BILLING_SECRET_KEY
// says a deploy was configured, nothing more.
var DependencyRegistry = []dependencyKey{
	{Name: "outbox", Instrumented: true},
	{Name: "csv_import_jobs", Instrumented: true},
	{Name: "campaign_sends", Instrumented: true},
	{Name: "stripe_webhooks", Instrumented: true},
	{Name: "scheduled_jobs", Instrumented: false},
	{Name: "platform_api", Instrumented: false},
	{Name: "stripe_api", Instrumented: false},
	{Name: "email_delivery", Instrumented: false},
	{Name: "object_storage", Instrumented: false},
}

// OutboxHealth is the measured state of outbox_events.
type OutboxHealth struct {
	Pending                 int64
	OldestPendingAgeSeconds int64
	Errored                 int64
}

// CSVJobsHealth is the measured state of csv_import_jobs.
type CSVJobsHealth struct {
	Queued                int64
	RunningStaleHeartbeat int64
}

// HealthSource measures the four instrumented dependencies. Every method
// takes asOf from the caller and compares against it in SQL rather than
// using Postgres now(), so a test can place a fixture on the exact
// boundary instant.
type HealthSource interface {
	Outbox(ctx context.Context, asOf time.Time) (OutboxHealth, error)
	CSVJobs(ctx context.Context, asOf time.Time) (CSVJobsHealth, error)
	CampaignSends(ctx context.Context, asOf time.Time) (CampaignSendsHealth, error)
	StripeWebhooks(ctx context.Context, asOf time.Time) (StripeWebhooksHealth, error)
}
```

- [ ] **Step 4: Write `health_checks.go` — the DB-backed source**

```go
package platformadmin

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
)

// errNotImplementedYet is removed in Task 3 when the remaining two checks
// land. It exists only so this file compiles as its own commit.
var errNotImplementedYet = errors.New("platformadmin: health check not implemented yet")

// errNoDB is returned by every check when the source has no database.
// It exists so a nil DB degrades to `unknown` rather than panicking:
// (*gorm.DB).WithContext dereferences its receiver, so an unguarded nil
// would take down the request rather than reporting an honest non-answer.
var errNoDB = errors.New("platformadmin: health source has no database")

type dbHealthSource struct{ db *gorm.DB }

// NewDBHealthSource returns a HealthSource reading from Postgres. Every
// measurement is a query, never an in-process counter: the production
// admin Deployment is replicas:1 today, but that is a fact about the
// manifest rather than a guarantee, and a DB-backed answer stays correct
// if the pin is ever lifted.
func NewDBHealthSource(db *gorm.DB) HealthSource { return &dbHealthSource{db: db} }

func (s *dbHealthSource) Outbox(ctx context.Context, asOf time.Time) (OutboxHealth, error) {
	if s.db == nil {
		return OutboxHealth{}, errNoDB
	}
	var out OutboxHealth
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE published_at IS NULL)                    AS pending,
			COALESCE(EXTRACT(EPOCH FROM (? - MIN(created_at)
				FILTER (WHERE published_at IS NULL)))::bigint, 0)           AS oldest_pending_age_seconds,
			COUNT(*) FILTER (WHERE published_at IS NULL AND error IS NOT NULL) AS errored
		FROM outbox_events`, asOf).Scan(&out).Error
	if err != nil {
		return OutboxHealth{}, err
	}
	return out, nil
}

func (s *dbHealthSource) CSVJobs(ctx context.Context, asOf time.Time) (CSVJobsHealth, error) {
	if s.db == nil {
		return CSVJobsHealth{}, errNoDB
	}
	var out CSVJobsHealth
	// age >= OrphanWindow is stale, so the comparison is <= on the
	// timestamp. Inclusive at the boundary, matching the plan's uniform
	// rule and pinned by an exact-instant fixture.
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE status = 'queued')  AS queued,
			COUNT(*) FILTER (WHERE status = 'running'
				AND heartbeat_at IS NOT NULL
				AND heartbeat_at <= ?)                 AS running_stale_heartbeat
		FROM csv_import_jobs`, asOf.Add(-csvjob.OrphanWindow)).Scan(&out).Error
	if err != nil {
		return CSVJobsHealth{}, err
	}
	return out, nil
}

func (s *dbHealthSource) CampaignSends(context.Context, time.Time) (CampaignSendsHealth, error) {
	return CampaignSendsHealth{}, errNotImplementedYet
}

func (s *dbHealthSource) StripeWebhooks(context.Context, time.Time) (StripeWebhooksHealth, error) {
	return StripeWebhooksHealth{}, errNotImplementedYet
}
```

Add the two placeholder types to `health.go` so this compiles; Task 3 fills in their fields:

```go
// CampaignSendsHealth is the measured state of campaigns. Fields land in Task 3.
type CampaignSendsHealth struct{}

// StripeWebhooksHealth is the measured state of stripe_webhook_events. Fields land in Task 3.
type StripeWebhooksHealth struct{}
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
cd services/marketplace-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags integration -p 1 ./internal/handlers/platformadmin/ -run 'TestOutboxHealth|TestCSVJobsStale' -v
```
Expected: PASS.

- [ ] **Step 6: Prove the boundary fixture discriminates**

Mutate `health_checks.go`: change `heartbeat_at <= ?` to `heartbeat_at < ?`. Re-run `TestCSVJobsStaleHeartbeatIsInclusiveAtTheBoundary`.
Expected: **FAIL**, `RunningStaleHeartbeat` 0 rather than 1. Record the test name and the failure text. Revert the mutation.

If it still passes, the fixture is not on the boundary and the test proves nothing — fix the fixture before continuing.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/health.go services/marketplace-api/internal/handlers/platformadmin/health_checks.go services/marketplace-api/internal/handlers/platformadmin/health_checks_integration_test.go
git commit -m "feat(platformadmin): health registry with outbox and csv-job checks (#289)"
```

---

### Task 3: The campaign and stripe-webhook checks

**Files:**
- Modify: `services/marketplace-api/internal/handlers/platformadmin/health.go`
- Modify: `services/marketplace-api/internal/handlers/platformadmin/health_checks.go`
- Test: `services/marketplace-api/internal/handlers/platformadmin/health_checks_integration_test.go`

**Interfaces:**
- Consumes: `HealthSource`, `dbHealthSource`, `healthAsOf` from Task 2.
- Produces:

```go
type CampaignSendsHealth struct {
	Sending               int64
	SendingStaleHeartbeat int64
}
type StripeWebhooksHealth struct {
	Unprocessed                 int64
	OldestUnprocessedAgeSeconds int64
	ManualReviewRequired        int64
}
```

- [ ] **Step 1: Write the failing tests**

Append to `health_checks_integration_test.go`:

```go
// The campaign window is campaign.StaleDuration — an exported constant
// that already governs RecoverStuckCampaigns. Same inclusive boundary rule
// and same 1ms offset as the csv test above.
func TestCampaignSendsStaleHeartbeatIsInclusiveAtTheBoundary(t *testing.T) {
	db := testdb.NewDB(t, "campaigns")
	src := platformadmin.NewDBHealthSource(db)

	exactly := healthAsOf.Add(-campaign.StaleDuration)
	justInside := exactly.Add(time.Millisecond)

	insert := func(status string, heartbeat *time.Time) {
		require.NoError(t, db.Exec(`INSERT INTO campaigns (id, store_id, name, status, heartbeat_at)
			VALUES (?, ?, 'c', ?, ?)`,
			uuid.New(), uuid.New(), status, heartbeat).Error)
	}
	insert("sending", &exactly)
	insert("sending", &justInside)
	insert("draft", nil)

	got, err := src.CampaignSends(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Sending, "only status='sending' rows count")
	require.Equal(t, int64(1), got.SendingStaleHeartbeat,
		"a heartbeat exactly StaleDuration old is stale; one millisecond younger is not")
}

func TestStripeWebhooksCountsUnprocessedAndManualReview(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")
	src := platformadmin.NewDBHealthSource(db)

	insert := func(id string, received time.Time, processed *time.Time, manual bool) {
		require.NoError(t, db.Exec(`INSERT INTO stripe_webhook_events
			(event_id, event_type, payload, received_at, processed_at, manual_review_required)
			VALUES (?, 'invoice.paid', '{}'::jsonb, ?, ?, ?)`,
			id, received, processed, manual).Error)
	}

	done := healthAsOf.Add(-time.Hour)
	insert("evt_done", done, &done, false)                              // processed
	insert("evt_old", healthAsOf.Add(-20*time.Minute), nil, false)      // unprocessed, oldest
	insert("evt_manual", healthAsOf.Add(-time.Minute), nil, true)       // needs a human

	got, err := src.StripeWebhooks(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Unprocessed, "processed row must not count")
	require.Equal(t, int64(1), got.ManualReviewRequired)
	require.Equal(t, int64(1200), got.OldestUnprocessedAgeSeconds)
}
```

Add `"github.com/mark8ly/marketplace-api/internal/campaign"` to the test file's imports.

- [ ] **Step 2: Run to verify they fail**

```bash
cd services/marketplace-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags integration -p 1 ./internal/handlers/platformadmin/ -run 'TestCampaignSends|TestStripeWebhooks' -v
```
Expected: FAIL — `health check not implemented yet`.

If the `campaigns` INSERT fails on a NOT NULL column this plan did not anticipate, read `internal/campaign/models.go` and add the missing columns to the fixture. Do not weaken the assertions to make the insert pass.

- [ ] **Step 3: Replace the placeholder types in `health.go`**

```go
// CampaignSendsHealth is the measured state of campaigns.
type CampaignSendsHealth struct {
	Sending               int64
	SendingStaleHeartbeat int64
}

// StripeWebhooksHealth is the measured state of stripe_webhook_events.
// Inbound only — receiving webhooks normally says nothing about whether
// our own outbound Stripe API calls are succeeding, which is why
// stripe_api is a separate, uninstrumented registry entry.
type StripeWebhooksHealth struct {
	Unprocessed                 int64
	OldestUnprocessedAgeSeconds int64
	ManualReviewRequired        int64
}
```

- [ ] **Step 4: Implement both checks in `health_checks.go`**

Delete `errNotImplementedYet` and its two stub methods — **keep `errNoDB`** — then add:

```go
func (s *dbHealthSource) CampaignSends(ctx context.Context, asOf time.Time) (CampaignSendsHealth, error) {
	if s.db == nil {
		return CampaignSendsHealth{}, errNoDB
	}
	var out CampaignSendsHealth
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE status = 'sending') AS sending,
			COUNT(*) FILTER (WHERE status = 'sending'
				AND heartbeat_at IS NOT NULL
				AND heartbeat_at <= ?)                 AS sending_stale_heartbeat
		FROM campaigns`, asOf.Add(-campaign.StaleDuration)).Scan(&out).Error
	if err != nil {
		return CampaignSendsHealth{}, err
	}
	return out, nil
}

func (s *dbHealthSource) StripeWebhooks(ctx context.Context, asOf time.Time) (StripeWebhooksHealth, error) {
	if s.db == nil {
		return StripeWebhooksHealth{}, errNoDB
	}
	var out StripeWebhooksHealth
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE processed_at IS NULL) AS unprocessed,
			COALESCE(EXTRACT(EPOCH FROM (? - MIN(received_at)
				FILTER (WHERE processed_at IS NULL)))::bigint, 0) AS oldest_unprocessed_age_seconds,
			COUNT(*) FILTER (WHERE manual_review_required)        AS manual_review_required
		FROM stripe_webhook_events`, asOf).Scan(&out).Error
	if err != nil {
		return StripeWebhooksHealth{}, err
	}
	return out, nil
}
```

Add `"github.com/mark8ly/marketplace-api/internal/campaign"` to the imports.

- [ ] **Step 5: Run all four check tests**

```bash
cd services/marketplace-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags integration -p 1 ./internal/handlers/platformadmin/ -run 'TestOutboxHealth|TestCSVJobs|TestCampaignSends|TestStripeWebhooks' -v
```
Expected: all PASS.

- [ ] **Step 6: Prove the campaign boundary fixture discriminates**

Mutate `heartbeat_at <= ?` to `heartbeat_at < ?` in `CampaignSends`. Re-run `TestCampaignSendsStaleHeartbeatIsInclusiveAtTheBoundary`.
Expected: **FAIL** with `SendingStaleHeartbeat` 0. Record the test name and failure text. Revert.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/
git commit -m "feat(platformadmin): campaign-send and stripe-webhook health checks (#289)"
```

---

### Task 4: The handler

**Files:**
- Modify: `services/marketplace-api/internal/handlers/platformadmin/health.go`
- Test: `services/marketplace-api/internal/handlers/platformadmin/health_test.go`

**Interfaces:**
- Consumes: everything from Tasks 2–3.
- Produces:

```go
func NewHealthHandler(src HealthSource, logger *slog.Logger) *HealthHandler
func (h *HealthHandler) Register(g *gin.RouterGroup)   // GET /admin/health
```

- [ ] **Step 1: Write the failing tests**

Create `health_test.go`:

```go
package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// stubHealthSource returns canned measurements. Every value is DISTINCT
// and NON-ZERO so an assertion cannot pass on a fabricated zero produced
// by a missing map key — the corollary to trap 6 that bit twice.
type stubHealthSource struct {
	outbox    platformadmin.OutboxHealth
	csv       platformadmin.CSVJobsHealth
	campaign  platformadmin.CampaignSendsHealth
	stripe    platformadmin.StripeWebhooksHealth
	outboxErr error
}

func (s *stubHealthSource) Outbox(context.Context, time.Time) (platformadmin.OutboxHealth, error) {
	return s.outbox, s.outboxErr
}
func (s *stubHealthSource) CSVJobs(context.Context, time.Time) (platformadmin.CSVJobsHealth, error) {
	return s.csv, nil
}
func (s *stubHealthSource) CampaignSends(context.Context, time.Time) (platformadmin.CampaignSendsHealth, error) {
	return s.campaign, nil
}
func (s *stubHealthSource) StripeWebhooks(context.Context, time.Time) (platformadmin.StripeWebhooksHealth, error) {
	return s.stripe, nil
}

// healthFixture is the one shared stub set, so the golden fixture in Task 5
// and the assertions here cannot drift apart.
func healthFixture() *stubHealthSource {
	return &stubHealthSource{
		outbox:   platformadmin.OutboxHealth{Pending: 7, OldestPendingAgeSeconds: 61, Errored: 3},
		csv:      platformadmin.CSVJobsHealth{Queued: 5, RunningStaleHeartbeat: 2},
		campaign: platformadmin.CampaignSendsHealth{Sending: 9, SendingStaleHeartbeat: 4},
		stripe: platformadmin.StripeWebhooksHealth{
			Unprocessed: 6, OldestUnprocessedAgeSeconds: 62, ManualReviewRequired: 0,
		},
	}
}

func healthRouter(t *testing.T, src platformadmin.HealthSource) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewHealthHandler(src, nil).Register(r.Group(""))
	return r
}

type healthBody struct {
	Data struct {
		CheckedAt    string `json:"checked_at"`
		Dependencies []struct {
			Name    string            `json:"name"`
			Status  string            `json:"status"`
			Metrics map[string]int64  `json:"metrics"`
		} `json:"dependencies"`
	} `json:"data"`
}

func getHealth(t *testing.T, src platformadmin.HealthSource) (*httptest.ResponseRecorder, healthBody) {
	t.Helper()
	rec := httptest.NewRecorder()
	healthRouter(t, src).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/health", nil))
	var body healthBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return rec, body
}

// Every registry entry must appear, in registry order. The registry's whole
// purpose is that a dependency cannot silently fall out of the response.
func TestHealthReportsEveryRegistryEntryInOrder(t *testing.T) {
	_, body := getHealth(t, healthFixture())

	require.Len(t, body.Data.Dependencies, len(platformadmin.DependencyRegistry))
	for i, want := range platformadmin.DependencyRegistry {
		require.Equal(t, want.Name, body.Data.Dependencies[i].Name,
			"dependency %d out of registry order", i)
	}
}

// An uninstrumented dependency carries NO metrics key — not {}, not zeroes.
// A zeroed metrics block is indistinguishable from a healthy one.
func TestHealthUninstrumentedEntriesHaveNoMetricsKey(t *testing.T) {
	rec, body := getHealth(t, healthFixture())
	require.Equal(t, http.StatusOK, rec.Code)

	var raw struct {
		Data struct {
			Dependencies []map[string]json.RawMessage `json:"dependencies"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

	seen := 0
	for i, dep := range body.Data.Dependencies {
		if dep.Status != platformadmin.StatusNotInstrumented {
			continue
		}
		seen++
		_, present := raw.Data.Dependencies[i]["metrics"]
		require.False(t, present, "%s is not_instrumented and must omit metrics entirely", dep.Name)
	}
	require.Equal(t, 5, seen, "expected five uninstrumented dependencies")
}

// A failed check is `unknown`, never `ok`, and never fails the endpoint —
// acceptance criterion 2. The error text must not reach the caller.
func TestHealthFailedCheckIsUnknownAndDoesNotFailTheEndpoint(t *testing.T) {
	src := healthFixture()
	src.outboxErr = errors.New("pq: password authentication failed for user \"dev\"")

	rec, body := getHealth(t, src)
	require.Equal(t, http.StatusOK, rec.Code, "a degraded dependency must not fail the endpoint")
	require.NotContains(t, rec.Body.String(), "password authentication",
		"driver error text must be logged server-side, never echoed")

	for _, dep := range body.Data.Dependencies {
		if dep.Name == "outbox" {
			require.Equal(t, platformadmin.StatusUnknown, dep.Status)
			require.Nil(t, dep.Metrics, "an unmeasured dependency must not ship fabricated zeroes")
		}
	}
	// The other checks still report.
	for _, dep := range body.Data.Dependencies {
		if dep.Name == "csv_import_jobs" {
			require.Equal(t, platformadmin.StatusDegraded, dep.Status)
		}
	}
}

// Thresholds: each instrumented dependency is degraded on its own rule.
func TestHealthStatusPerThreshold(t *testing.T) {
	src := &stubHealthSource{
		// Pending but young, and no errored rows: ok.
		outbox:   platformadmin.OutboxHealth{Pending: 4, OldestPendingAgeSeconds: 1, Errored: 0},
		csv:      platformadmin.CSVJobsHealth{Queued: 3, RunningStaleHeartbeat: 0},
		campaign: platformadmin.CampaignSendsHealth{Sending: 2, SendingStaleHeartbeat: 0},
		stripe:   platformadmin.StripeWebhooksHealth{Unprocessed: 1, OldestUnprocessedAgeSeconds: 1},
	}
	_, body := getHealth(t, src)
	for _, dep := range body.Data.Dependencies {
		switch dep.Name {
		case "outbox", "csv_import_jobs", "campaign_sends", "stripe_webhooks":
			require.Equal(t, platformadmin.StatusOK, dep.Status, "%s should be ok", dep.Name)
		}
	}

	// A single errored outbox row is degraded regardless of age.
	src.outbox = platformadmin.OutboxHealth{Pending: 1, OldestPendingAgeSeconds: 1, Errored: 1}
	_, body = getHealth(t, src)
	for _, dep := range body.Data.Dependencies {
		if dep.Name == "outbox" {
			require.Equal(t, platformadmin.StatusDegraded, dep.Status,
				"any errored row is degraded even when nothing is old")
		}
	}

	// manual_review_required is the system's own "a human must look" flag.
	src.outbox = platformadmin.OutboxHealth{Pending: 0, OldestPendingAgeSeconds: 0, Errored: 0}
	src.stripe = platformadmin.StripeWebhooksHealth{Unprocessed: 1, OldestUnprocessedAgeSeconds: 1, ManualReviewRequired: 1}
	_, body = getHealth(t, src)
	for _, dep := range body.Data.Dependencies {
		if dep.Name == "stripe_webhooks" {
			require.Equal(t, platformadmin.StatusDegraded, dep.Status)
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run TestHealth -v
```
Expected: FAIL — `undefined: platformadmin.NewHealthHandler`.

- [ ] **Step 3: Implement the handler in `health.go`**

Add to the imports: `"log/slog"`, `"net/http"`, `"github.com/gin-gonic/gin"`.

```go
// dependencyRow is one entry in the response. Metrics is omitempty so an
// uninstrumented or unknown dependency ships no metrics key at all — a
// zeroed block would be indistinguishable from a healthy one.
type dependencyRow struct {
	Name    string           `json:"name"`
	Status  string           `json:"status"`
	Metrics map[string]int64 `json:"metrics,omitempty"`
}

// HealthHandler serves GET /admin/health (#289) — "is this product
// working", as distinct from /health (is the process alive) and /ready
// (can it serve). Those two are correctly scoped and unchanged.
type HealthHandler struct {
	src    HealthSource
	logger *slog.Logger
	now    func() time.Time
}

// NewHealthHandler constructs the handler. logger may be nil.
func NewHealthHandler(src HealthSource, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{src: src, logger: logger, now: time.Now}
}

// Register mounts the route on the supplied group.
func (h *HealthHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/health", h.health)
}

// logCheckFailed records the real error server-side. It is never echoed to
// the caller — same discipline /ready already applies, so DSN fragments and
// driver error text do not leave the process.
func (h *HealthHandler) logCheckFailed(name string, err error) {
	if h.logger != nil {
		h.logger.Error("health check failed", "dependency", name, "err", err)
	}
}

func (h *HealthHandler) health(c *gin.Context) {
	ctx := c.Request.Context()
	asOf := h.now()

	// Gather first, keyed by name. A check that errors is absent from this
	// map, which is what makes it `unknown` below rather than a zeroed `ok`.
	measured := make(map[string]dependencyRow, 4)

	if v, err := h.src.Outbox(ctx, asOf); err != nil {
		h.logCheckFailed("outbox", err)
	} else {
		status := StatusOK
		if v.Errored > 0 || time.Duration(v.OldestPendingAgeSeconds)*time.Second >= OutboxPendingThreshold {
			status = StatusDegraded
		}
		measured["outbox"] = dependencyRow{Status: status, Metrics: map[string]int64{
			"pending":                    v.Pending,
			"oldest_pending_age_seconds": v.OldestPendingAgeSeconds,
			"errored":                    v.Errored,
		}}
	}

	if v, err := h.src.CSVJobs(ctx, asOf); err != nil {
		h.logCheckFailed("csv_import_jobs", err)
	} else {
		status := StatusOK
		if v.RunningStaleHeartbeat > 0 {
			status = StatusDegraded
		}
		measured["csv_import_jobs"] = dependencyRow{Status: status, Metrics: map[string]int64{
			"queued":                  v.Queued,
			"running_stale_heartbeat": v.RunningStaleHeartbeat,
		}}
	}

	if v, err := h.src.CampaignSends(ctx, asOf); err != nil {
		h.logCheckFailed("campaign_sends", err)
	} else {
		status := StatusOK
		if v.SendingStaleHeartbeat > 0 {
			status = StatusDegraded
		}
		measured["campaign_sends"] = dependencyRow{Status: status, Metrics: map[string]int64{
			"sending":                 v.Sending,
			"sending_stale_heartbeat": v.SendingStaleHeartbeat,
		}}
	}

	if v, err := h.src.StripeWebhooks(ctx, asOf); err != nil {
		h.logCheckFailed("stripe_webhooks", err)
	} else {
		status := StatusOK
		if v.ManualReviewRequired > 0 ||
			time.Duration(v.OldestUnprocessedAgeSeconds)*time.Second >= StripeUnprocessedThreshold {
			status = StatusDegraded
		}
		measured["stripe_webhooks"] = dependencyRow{Status: status, Metrics: map[string]int64{
			"unprocessed":                    v.Unprocessed,
			"oldest_unprocessed_age_seconds": v.OldestUnprocessedAgeSeconds,
			"manual_review_required":         v.ManualReviewRequired,
		}}
	}

	// Emit in registry order. Membership comes from the registry, never
	// from what the gather stage happened to produce.
	rows := make([]dependencyRow, 0, len(DependencyRegistry))
	for _, key := range DependencyRegistry {
		if !key.Instrumented {
			rows = append(rows, dependencyRow{Name: key.Name, Status: StatusNotInstrumented})
			continue
		}
		row, ok := measured[key.Name]
		if !ok {
			// Registered as instrumented, but no measurement — the check
			// errored, or a registry entry was added without a gather.
			// Either way the honest answer is `unknown` with no metrics.
			rows = append(rows, dependencyRow{Name: key.Name, Status: StatusUnknown})
			continue
		}
		row.Name = key.Name
		rows = append(rows, row)
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"checked_at":   asOf.UTC().Format(time.RFC3339),
		"dependencies": rows,
	}})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run TestHealth -v
```
Expected: PASS.

- [ ] **Step 5: Prove the registry test discriminates**

Delete the `{Name: "object_storage", Instrumented: false}` line from `DependencyRegistry` and re-run.
Expected: **FAIL** in `TestHealthReportsEveryRegistryEntryInOrder` and `TestHealthUninstrumentedEntriesHaveNoMetricsKey` (five expected, four seen). Record the test names and failure text. Restore the line.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/health.go services/marketplace-api/internal/handlers/platformadmin/health_test.go
git commit -m "feat(platformadmin): GET /admin/health handler (#289)"
```

---

### Task 5: Golden fixture, proved by mutation

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/testdata/health_response.json`
- Modify: `services/marketplace-api/internal/handlers/platformadmin/health_test.go`

**Interfaces:**
- Consumes: `healthFixture()`, `healthRouter()` from Task 4.
- Produces: nothing further.

- [ ] **Step 1: Write the failing golden test**

Append to `health_test.go`:

```go
// THE test. Real handler output compared to the committed contract.
// checked_at is replaced before comparison because it is the one field
// that legitimately varies per request; everything else is pinned.
func TestHealthMatchesContract(t *testing.T) {
	rec := httptest.NewRecorder()
	healthRouter(t, healthFixture()).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/admin/health", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	data := got["data"].(map[string]any)
	checkedAt, ok := data["checked_at"].(string)
	require.True(t, ok, "checked_at must be present and a string")
	_, err := time.Parse(time.RFC3339, checkedAt)
	require.NoError(t, err, "checked_at must be RFC3339 with offset")
	data["checked_at"] = "PINNED"

	normalised, err := json.Marshal(got)
	require.NoError(t, err)

	want, err := os.ReadFile("testdata/health_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(normalised))
}
```

Add `"os"` to the test imports.

- [ ] **Step 2: Run it to verify it fails**

```bash
cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run TestHealthMatchesContract -v
```
Expected: FAIL — `testdata/health_response.json` does not exist.

- [ ] **Step 3: Write the fixture**

Create `testdata/health_response.json`:

```json
{
  "data": {
    "checked_at": "PINNED",
    "dependencies": [
      { "name": "outbox", "status": "degraded",
        "metrics": { "pending": 7, "oldest_pending_age_seconds": 61, "errored": 3 } },
      { "name": "csv_import_jobs", "status": "degraded",
        "metrics": { "queued": 5, "running_stale_heartbeat": 2 } },
      { "name": "campaign_sends", "status": "degraded",
        "metrics": { "sending": 9, "sending_stale_heartbeat": 4 } },
      { "name": "stripe_webhooks", "status": "ok",
        "metrics": { "unprocessed": 6, "oldest_unprocessed_age_seconds": 62, "manual_review_required": 0 } },
      { "name": "scheduled_jobs", "status": "not_instrumented" },
      { "name": "platform_api", "status": "not_instrumented" },
      { "name": "stripe_api", "status": "not_instrumented" },
      { "name": "email_delivery", "status": "not_instrumented" },
      { "name": "object_storage", "status": "not_instrumented" }
    ]
  }
}
```

Note the fixture values are deliberately mixed: three degraded, one ok, so the golden file pins status derivation and not just field names. `outbox` is degraded on `errored: 3` while its age (61s) is *under* the 5-minute threshold — so the fixture also discriminates the two halves of that OR.

- [ ] **Step 4: Run to verify it passes**

```bash
cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run TestHealthMatchesContract -v
```
Expected: PASS. If it fails, fix the fixture to match real output — never loosen the assertion.

- [ ] **Step 5: Prove the fixture catches a RENAME**

In `health.go`, change the JSON tag on `dependencyRow.Status` from `json:"status"` to `json:"state"`. Re-run.
Expected: **FAIL**. Record the failure text. Revert.

- [ ] **Step 6: Prove the fixture catches an ADDITION**

Add a field to `dependencyRow`:

```go
	Extra string `json:"extra"`
```

Re-run.
Expected: **FAIL** — `require.JSONEq` compares both directions, so an unexpected key breaks it. Record the failure text. Revert.

A fixture that only catches omissions is theatre. Both mutations must fail before this task is done.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/testdata/health_response.json services/marketplace-api/internal/handlers/platformadmin/health_test.go
git commit -m "test(platformadmin): golden fixture for /admin/health (#289)"
```

---

### Task 6: Mount the route, and guard both wiring sites

`Register` builds the health source from `deps.DB`, exactly as it already
does for `NonceStore`, so **no new `Deps` field is needed and `main.go`
does not change**. The wiring test is written regardless: #323 records five
instances of the two `platformadmin.Register` call sites drifting apart,
with three different failure modes, and this is the cheap part.

**Files:**
- Modify: `services/marketplace-api/internal/handlers/platformadmin/routes.go`
- Test: `services/marketplace-api/internal/handlers/platformadmin/routes_test.go`
- Create: `services/marketplace-api/cmd/marketplace-api/wiring_test.go`

**Interfaces:**
- Consumes: `NewHealthHandler`, `NewDBHealthSource`.
- Produces: the mounted route.

- [ ] **Step 1: Write the failing route test**

Append to `routes_test.go`:

```go
// /admin/health mounts whenever the surface itself mounts. It needs only
// DB, so unlike the client-backed routes it has no nil-dependency guard.
func TestRegisterMountsHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:   stubAuditRepo{},
		Secret: "test-secret",
	})

	found := false
	for _, route := range r.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/v1/platform/admin/health" {
			found = true
		}
	}
	require.True(t, found, "/admin/health must mount")
}

// A nil DB must not panic the request. This is the assertion behind the
// claim in routes.go that a nil database degrades to `unknown`: without
// the errNoDB guard in the source, (*gorm.DB).WithContext dereferences a
// nil receiver and this test panics rather than failing.
func TestRegisterHealthWithNilDBReportsUnknownNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:   stubAuditRepo{},
		Secret: "test-secret",
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/platform/admin/health", nil)
	require.NotPanics(t, func() { r.ServeHTTP(rec, req) })

	// The surface's own auth runs first; whatever it answers, the point is
	// that no nil dereference escaped the handler.
	require.NotEqual(t, http.StatusInternalServerError, rec.Code)
}
```

If `routes_test.go` does not already import `net/http/httptest`, add it.

If `stubAuditRepo` is named differently in `routes_test.go`, reuse whatever that file already uses for `Deps.Repo` — do not introduce a second stub.

- [ ] **Step 2: Run to verify it fails**

```bash
cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run TestRegisterMountsHealth -v
```
Expected: FAIL — route not found.

- [ ] **Step 3: Mount it in `routes.go`**

Inside `Register`, after the `NewAuditLogsHandler(...)` line:

```go
	// Health needs only the DB, so it mounts alongside the surface itself
	// rather than behind a nil-dependency guard like the client-backed
	// routes below. A nil DB is handled inside the source, which returns
	// errNoDB from every check rather than dereferencing a nil *gorm.DB;
	// the handler renders that as `unknown` — the honest non-answer, and
	// never a fabricated ok.
	NewHealthHandler(NewDBHealthSource(deps.DB), deps.Logger).Register(group)
```

- [ ] **Step 4: Run to verify it passes**

```bash
cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run TestRegister -v
```
Expected: PASS, and no existing route test regresses.

- [ ] **Step 5: Write the wiring-equivalence test (#323)**

Create `cmd/marketplace-api/wiring_test.go`. This parses main.go's AST and
asserts both `platformadmin.Register` calls pass a `Deps` literal with the
same field set — which is exactly the drift #323 describes, catchable
without the full wiring refactor that issue proposes.

```go
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlatformadminRegisterSitesAgree guards the failure mode in #323: the
// mode.Both engine and the mode.Admin engine each call
// platformadmin.Register with their own Deps literal, and a field added to
// one and not the other means the two deployments differ silently. Three
// distinct failure modes have been observed, including a nil interface
// that panics at runtime.
func TestPlatformadminRegisterSitesAgree(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	require.NoError(t, err)

	var fieldSets [][]string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Register" {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "platformadmin" {
			return true
		}
		for _, arg := range call.Args {
			lit, ok := arg.(*ast.CompositeLit)
			if !ok {
				continue
			}
			var fields []string
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok {
					fields = append(fields, key.Name)
				}
			}
			if len(fields) > 0 {
				sort.Strings(fields)
				fieldSets = append(fieldSets, fields)
			}
		}
		return true
	})

	require.Len(t, fieldSets, 2,
		"expected exactly two platformadmin.Register call sites in main.go; "+
			"if a third was added, this test must be updated deliberately")
	require.Equal(t, fieldSets[0], fieldSets[1],
		"the two platformadmin.Register sites construct different Deps field sets — "+
			"one deployment will differ from the other (#323)")
}
```

- [ ] **Step 6: Run it**

```bash
cd services/marketplace-api/cmd/marketplace-api && go test -run TestPlatformadminRegisterSitesAgree -v
```
Expected: PASS.

- [ ] **Step 7: Prove the wiring test discriminates**

Temporarily delete one field (e.g. `Logger:`) from the **first** `platformadmin.Register` Deps literal in main.go. Re-run.
Expected: **FAIL** naming the differing field sets. Record the failure text. Restore the field.

If it passes with the field removed, the test is not reading what it claims to and must be fixed before this task is done.

- [ ] **Step 8: Full package run**

```bash
cd services/marketplace-api && go build ./... && go test ./internal/handlers/platformadmin/ ./internal/csvjob/ ./cmd/marketplace-api/
```
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/routes.go services/marketplace-api/internal/handlers/platformadmin/routes_test.go services/marketplace-api/cmd/marketplace-api/wiring_test.go
git commit -m "feat(platformadmin): mount /admin/health and guard both wiring sites (#289, #323)"
```

---

## After the plan

**File the follow-up issue** described in the spec's "Out of scope": making
`scheduled_jobs` individually reportable needs a `cron_runs` table written
by every scheduled job's call site. Until it exists, `scheduled_jobs`
reports `not_instrumented`, which is the truth.

**Verification after deploy** — separate the checks that carry information
from those that merely mean "no data reached this code":

- *Carries information, data-independent:* the route answers under
  signature; an unsigned request is rejected; the payload contains all nine
  registry entries; the five uninstrumented entries carry no `metrics` key.
- *Proves less:* production has 4 tenants and 4 stores and no merchant has
  entered the billing flow, so expect the four instrumented dependencies to
  report `ok` with zero counts. **A zero proves the query ran, not that it
  could ever return non-zero** — #282 shipped "verified" on exactly that
  reading. The boundary fixtures, not the production response, are what
  establish that these counters can move.

Deploys are Kargo-gated (CI → ghcr → Warehouse → Freight → Promotion →
rollout); expect 10–20 minutes from freight appearing, and gate
verification on every service the change touches.
