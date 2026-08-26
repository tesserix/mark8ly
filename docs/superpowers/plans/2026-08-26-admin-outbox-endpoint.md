# `GET /admin/outbox` (#331) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the platform console a cross-tenant read of `outbox_events` — what is stuck, what
failed and why — over the three-state model #336 established.

**Architecture:** A read-only endpoint on the existing signed platform-admin surface, modelled
field-for-field on `notifications.go` (#332). The query and filter live in `internal/outbox`; the
handler holds a one-method `OutboxLister` interface. `status` and `age_seconds` are derived in SQL
so the console reimplements neither. `payload` is excluded by construction.

**Tech Stack:** Go 1.26, GORM, Gin, Postgres 15, `testify/require`, build-tagged integration tests
via `pkg/testdb`.

**Spec:** `docs/superpowers/specs/2026-08-26-outbox-failure-state-design.md` — **§5 is this
endpoint's design** and was reviewed as part of #336. Read it before starting.

## Global Constraints

- Service root for every command: `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api`
- Integration DSN — LAN IP, **never `localhost`**: `TEST_DATABASE_URL=postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable`
- Integration tests require `-tags=integration` and `-count=1`. **Without the tag, build-tagged files are never compiled and the run is a false green** (trap 8).
- **Run whole packages, never a `-run`-scoped subset, for final evidence** (trap 19), and use `-v` and count `--- PASS` / `--- SKIP`: `exit=0` does not distinguish a pass from a skip (trap 20).
- Never pipe the measured `go test` — capture with `> file 2>&1` and report `$?` separately.
- Module path: `github.com/mark8ly/marketplace-api`.
- Commit messages: conventional commits, **single line**, no signatures.
- **Do not push, open a PR, merge, or deploy.** Local commits on `feat/331-admin-outbox` only.
- Branch base: `main` at `e8fc6dd7`.
- **This endpoint is a READ.** It gets **no** entry in `RequiredWriteCapabilities` — that map is for writes, and a read route added there would be wrong, not merely redundant.
- **`payload` must never appear in any response**, and no detail view is in scope. It is arbitrary JSONB that may carry customer data.
- **`error` is served as an opaque string.** The column is `text` with no `CHECK`; the vocabulary is Go-enforced only and the requeue path is a raw `UPDATE`. Do not switch on its value anywhere in the handler.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `internal/outbox/platform_list.go` | the cross-tenant platform read: filter, row, query | **Create** — new file, not appended to `repository.go`, which owns the publisher's write path |
| `internal/outbox/platform_list_integration_test.go` | query behaviour against real Postgres | **Create** |
| `internal/handlers/platformadmin/outbox.go` | HTTP handler, filter parsing, JSON projection | **Create** |
| `internal/handlers/platformadmin/outbox_test.go` | handler unit tests + golden response | **Create** |
| `internal/handlers/platformadmin/testdata/outbox_response.json` | pinned contract shape | **Create** |
| `internal/handlers/platformadmin/routes.go` | `Deps.Outbox` + mount | **Modify** |
| `internal/handlers/platformadmin/routes_outbox_test.go` | route mounted, and unmounted when nil | **Create** |
| `cmd/marketplace-api/main.go` | wire the repository into **both** `Deps` sites | **Modify** |

---

## Task 1: the platform query

**Files:**
- Create: `internal/outbox/platform_list.go`
- Test: `internal/outbox/platform_list_integration_test.go`

**Interfaces:**
- Consumes: `OutboxEvent` and its `TableName()` from `models.go`.
- Produces, used by Tasks 2 and 3:
  - `outbox.StatusPending` / `StatusFailed` / `StatusPublished` (`string` consts: `"pending"`, `"failed"`, `"published"`)
  - `outbox.DefaultPlatformPageSize = 50`, `outbox.MaxPlatformPageSize = 500`
  - `type outbox.PlatformListFilter struct { TenantID *uuid.UUID; Status string; EventType string; OlderThanMinutes int; From *time.Time; To *time.Time; Page int; Limit int }`
  - `type outbox.PlatformRow struct { ID, TenantID, Aggregate, AggregateID, EventType, Status string; CreatedAt time.Time; PublishedAt *time.Time; Error *string; AgeSeconds *int64 }`
  - `type outbox.PlatformListResult struct { Rows []PlatformRow; Total int64 }`
  - `func ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter, asOf time.Time) (PlatformListResult, error)`

**Why `asOf` is a parameter.** §5 requires `age_seconds` be computed from the same instant
`older_than_minutes` filters on. Passing the instant in makes that literally true rather than
approximately, and lets a test pin an exact age. `/admin/health`'s `Outbox(ctx, asOf)` on this same
surface already works this way.

- [ ] **Step 1: Write the failing tests**

Create `internal/outbox/platform_list_integration_test.go`:

```go
//go:build integration

package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// listAsOf is a fixed instant so age assertions are exact rather than
// approximate. Every fixture below is placed relative to it.
var listAsOf = time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

// seedRow inserts one outbox_events row in a chosen state. published and
// errMsg are independent so the three states can be built explicitly rather
// than inferred.
func seedRow(t *testing.T, db *gorm.DB, tenantID string, eventType string,
	createdAt time.Time, published *time.Time, errMsg *string) string {
	t.Helper()
	id := uuid.NewString()
	err := db.Exec(`
		INSERT INTO outbox_events
			(id, tenant_id, aggregate, aggregate_id, event_type, payload, created_at, published_at, error)
		VALUES (?, ?, 'product', ?, ?, '{"store_id":"11111111-1111-1111-1111-111111111111","secret":"do-not-leak"}'::jsonb, ?, ?, ?)`,
		id, tenantID, uuid.NewString(), eventType, createdAt, published, errMsg).Error
	require.NoError(t, err)
	return id
}

func TestIntegration_ListPlatform_DerivesAllThreeStates(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	pubAt := listAsOf.Add(-30 * time.Minute)
	failReason := outbox.ReasonPayloadUnparseable

	pendingID := seedRow(t, db, tenantID, "product.created", listAsOf.Add(-10*time.Minute), nil, nil)
	failedID := seedRow(t, db, tenantID, "product.updated", listAsOf.Add(-20*time.Minute), nil, &failReason)
	publishedID := seedRow(t, db, tenantID, "product.deleted", listAsOf.Add(-40*time.Minute), &pubAt, nil)

	got, err := outbox.ListPlatform(context.Background(), db, outbox.PlatformListFilter{}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Total)
	require.Len(t, got.Rows, 3)

	byID := map[string]outbox.PlatformRow{}
	for _, r := range got.Rows {
		byID[r.ID] = r
	}

	require.Equal(t, outbox.StatusPending, byID[pendingID].Status)
	require.Equal(t, outbox.StatusFailed, byID[failedID].Status)
	require.Equal(t, outbox.StatusPublished, byID[publishedID].Status)

	// Age is present and EXACT for unpublished rows.
	require.NotNil(t, byID[pendingID].AgeSeconds)
	require.Equal(t, int64(600), *byID[pendingID].AgeSeconds)
	require.NotNil(t, byID[failedID].AgeSeconds)
	require.Equal(t, int64(1200), *byID[failedID].AgeSeconds)

	// Absent for a published row: a settled row has no waiting time, and a
	// growing number there would read as "stuck" next to a real one.
	require.Nil(t, byID[publishedID].AgeSeconds,
		"a published row must have no age_seconds")

	// The failure reason is carried through verbatim.
	require.NotNil(t, byID[failedID].Error)
	require.Equal(t, outbox.ReasonPayloadUnparseable, *byID[failedID].Error)
	require.Nil(t, byID[pendingID].Error)
}

func TestIntegration_ListPlatform_StatusFilterNarrows(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	pubAt := listAsOf.Add(-30 * time.Minute)
	failReason := outbox.ReasonStoreNotFound

	seedRow(t, db, tenantID, "product.created", listAsOf.Add(-10*time.Minute), nil, nil)
	seedRow(t, db, tenantID, "product.updated", listAsOf.Add(-20*time.Minute), nil, &failReason)
	seedRow(t, db, tenantID, "product.deleted", listAsOf.Add(-40*time.Minute), &pubAt, nil)

	for _, tc := range []struct {
		status string
		want   int
	}{
		{outbox.StatusPending, 1},
		{outbox.StatusFailed, 1},
		{outbox.StatusPublished, 1},
	} {
		got, err := outbox.ListPlatform(context.Background(), db,
			outbox.PlatformListFilter{Status: tc.status}, listAsOf)
		require.NoError(t, err, tc.status)
		require.Equal(t, int64(tc.want), got.Total, "total for status=%s", tc.status)
		require.Len(t, got.Rows, tc.want, "rows for status=%s", tc.status)
		require.Equal(t, tc.status, got.Rows[0].Status)
	}

	// An unrecognised status narrows NOTHING rather than erroring or
	// returning empty — the established contract on this surface.
	got, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{Status: "banana"}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Total, "an unknown status must narrow nothing")

}

// older_than_minutes answers "what is stuck", so it applies to UNPUBLISHED
// rows only. A published row is settled however old it is.
func TestIntegration_ListPlatform_OlderThanMinutesExcludesPublished(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	oldPub := listAsOf.Add(-30 * time.Minute)

	oldPendingID := seedRow(t, db, tenantID, "product.created", listAsOf.Add(-60*time.Minute), nil, nil)
	seedRow(t, db, tenantID, "product.updated", listAsOf.Add(-1*time.Minute), nil, nil)     // young pending
	seedRow(t, db, tenantID, "product.deleted", listAsOf.Add(-600*time.Minute), &oldPub, nil) // very old, published

	got, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{OlderThanMinutes: 30}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.Rows, 1)
	require.Equal(t, oldPendingID, got.Rows[0].ID,
		"only the old UNPUBLISHED row may match; the older published row is settled")
}

func TestIntegration_ListPlatform_TenantAndEventTypeNarrow(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantA := uuid.NewString()
	tenantB := uuid.NewString()

	wantID := seedRow(t, db, tenantA, "product.created", listAsOf.Add(-5*time.Minute), nil, nil)
	seedRow(t, db, tenantA, "product.updated", listAsOf.Add(-5*time.Minute), nil, nil)
	seedRow(t, db, tenantB, "product.created", listAsOf.Add(-5*time.Minute), nil, nil)

	tid := uuid.MustParse(tenantA)
	got, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{TenantID: &tid, EventType: "product.created"}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Equal(t, wantID, got.Rows[0].ID)
}

func TestIntegration_ListPlatform_ClampsLimitAndPages(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	for i := 0; i < 3; i++ {
		seedRow(t, db, tenantID, "product.created",
			listAsOf.Add(-time.Duration(i+1)*time.Minute), nil, nil)
	}

	// An oversized limit CLAMPS rather than refusing.
	got, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{Limit: 100000}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Total)
	require.Len(t, got.Rows, 3)

	// Page 2 of 2-per-page returns the remaining row, and Total stays the
	// FULL count, not the page size.
	got, err = outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{Limit: 2, Page: 2}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Total)
	require.Len(t, got.Rows, 1)
}

func TestIntegration_ListPlatform_EmptyIsNotAnError(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	got, err := outbox.ListPlatform(context.Background(), db, outbox.PlatformListFilter{}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(0), got.Total)
	require.Empty(t, got.Rows)
}
```

- [ ] **Step 2: Run them to verify they fail**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -count=1 ./internal/outbox/ -v > /tmp/t331-red.txt 2>&1
echo "exit=$?"
tail -20 /tmp/t331-red.txt
```

Expected: **compile failure** — `undefined: outbox.ListPlatform`, `undefined: outbox.PlatformListFilter`,
`undefined: outbox.StatusPending`.

- [ ] **Step 3: Create the query**

Create `internal/outbox/platform_list.go`:

```go
// Package outbox: the cross-tenant platform read (#331). Deliberately a
// separate file from repository.go, which owns the publisher's write path —
// this is a read for the platform console and shares nothing with it but
// the table.
package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Derived status values. These are computed in SQL, not stored: the issue
// requires the console not reimplement the null-check, and deriving it
// server-side is what keeps one definition of "pending" in the estate.
const (
	StatusPending   = "pending"
	StatusFailed    = "failed"
	StatusPublished = "published"
)

// Page bounds, matching notification and ticket exactly so the platform
// surface behaves identically across its list endpoints.
const (
	DefaultPlatformPageSize = 50
	MaxPlatformPageSize     = 500
)

// PlatformListFilter narrows the cross-tenant read. Every field is
// optional, and an unrecognised value narrows NOTHING rather than erroring
// — the established contract across this surface.
//
// TenantID NARROWS rather than scopes: this endpoint is cross-tenant by
// design, and the console uses it to answer estate-wide questions.
type PlatformListFilter struct {
	TenantID  *uuid.UUID
	Status    string // StatusPending | StatusFailed | StatusPublished | "" (any)
	EventType string
	// OlderThanMinutes, when > 0, narrows to UNPUBLISHED rows at least that
	// old. It deliberately does NOT match published rows: this filter
	// answers "what is stuck", and a published row is settled however old
	// it is. Same reasoning as AgeSeconds being nil for published rows.
	OlderThanMinutes int
	From             *time.Time
	To               *time.Time
	Page             int
	Limit            int
}

// PlatformRow is one row of the platform read.
//
// There is no Payload field, and that is the point: the projection is
// field-by-field, so a column added to OutboxEvent tomorrow cannot leak
// through this surface. outbox_events.payload is arbitrary JSONB that may
// carry customer data, and a governance surface listing stuck events does
// not need it to be useful.
type PlatformRow struct {
	ID          string
	TenantID    string
	Aggregate   string
	AggregateID string
	EventType   string
	Status      string
	CreatedAt   time.Time
	PublishedAt *time.Time
	Error       *string
	// AgeSeconds is how long an UNPUBLISHED row has been waiting, measured
	// from the caller's asOf. It is nil for a published row: that row is
	// settled, so it has no waiting time, and a number that grew forever
	// there would read as "stuck" beside a genuinely stuck row.
	AgeSeconds *int64
}

// PlatformListResult carries the page plus the FULL match count.
type PlatformListResult struct {
	Rows  []PlatformRow
	Total int64
}

// ListPlatform returns a filtered, paginated, CROSS-TENANT page of outbox
// events for the platform console (#331).
//
// asOf is the instant both AgeSeconds and OlderThanMinutes are measured
// from. It is a parameter rather than time.Now() so the two can never
// disagree — a console that displayed an age computed at a different
// instant from the filter that selected the row would be quietly wrong —
// and so a test can pin an exact age.
func ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter,
	asOf time.Time) (PlatformListResult, error) {
	var result PlatformListResult

	q := db.WithContext(ctx).Model(&OutboxEvent{})

	if f.TenantID != nil {
		q = q.Where("tenant_id = ?", *f.TenantID)
	}
	if f.EventType != "" {
		q = q.Where("event_type = ?", f.EventType)
	}
	// An unrecognised status narrows nothing, matching how every other
	// unknown parameter on this surface behaves.
	switch f.Status {
	case StatusPending:
		q = q.Where("published_at IS NULL AND error IS NULL")
	case StatusFailed:
		q = q.Where("published_at IS NULL AND error IS NOT NULL")
	case StatusPublished:
		q = q.Where("published_at IS NOT NULL")
	}
	if f.OlderThanMinutes > 0 {
		cutoff := asOf.Add(-time.Duration(f.OlderThanMinutes) * time.Minute)
		q = q.Where("published_at IS NULL AND created_at <= ?", cutoff)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at <= ?", *f.To)
	}

	// Count BEFORE Select: the page below adds computed columns, and Total
	// must be the full match count, not the page size.
	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("outbox platform list count: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultPlatformPageSize
	}
	if limit > MaxPlatformPageSize {
		limit = MaxPlatformPageSize
	}
	page := f.Page
	if page < 1 {
		page = 1
	}

	// status and age_seconds are derived HERE, in SQL, so there is exactly
	// one definition of each in the estate. age_seconds is NULL for a
	// published row by the same CASE that makes its status 'published'.
	if err := q.
		Select(`id, tenant_id, aggregate, aggregate_id, event_type, created_at, published_at, error,
			CASE
				WHEN published_at IS NOT NULL THEN ?
				WHEN error IS NOT NULL        THEN ?
				ELSE                               ?
			END AS status,
			CASE
				WHEN published_at IS NULL
				THEN EXTRACT(EPOCH FROM (? - created_at))::bigint
				ELSE NULL
			END AS age_seconds`,
			StatusPublished, StatusFailed, StatusPending, asOf).
		Order("created_at DESC").
		Limit(limit).Offset((page - 1) * limit).
		Scan(&result.Rows).Error; err != nil {
		return result, fmt.Errorf("outbox platform list: %w", err)
	}
	return result, nil
}
```

- [ ] **Step 4: Run the whole package to verify GREEN**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -count=1 ./internal/outbox/ -v > /tmp/t331-green.txt 2>&1
echo "exit=$?"
echo "PASS=$(grep -cE '^\s*--- PASS' /tmp/t331-green.txt) FAIL=$(grep -cE '^\s*--- FAIL' /tmp/t331-green.txt) SKIP=$(grep -cE '^\s*--- SKIP' /tmp/t331-green.txt)"
```

Expected: every new test passes, every #336/#374 test still passes, and **SKIP=0**.

- [ ] **Step 5: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/outbox/platform_list.go \
        services/marketplace-api/internal/outbox/platform_list_integration_test.go
git commit -m "feat(outbox): cross-tenant platform list with derived status and age"
```

---

## Task 2: the handler

**Files:**
- Create: `internal/handlers/platformadmin/outbox.go`
- Create: `internal/handlers/platformadmin/outbox_test.go`
- Create: `internal/handlers/platformadmin/testdata/outbox_response.json`

**Interfaces:**
- Consumes: everything Task 1 produced.
- Produces, used by Task 3:
  - `type platformadmin.OutboxLister interface { ListPlatform(ctx context.Context, db *gorm.DB, f outbox.PlatformListFilter, asOf time.Time) (outbox.PlatformListResult, error) }`
  - `func platformadmin.NewOutboxHandler(db *gorm.DB, lister OutboxLister, logger *slog.Logger) *OutboxHandler`
  - `func (h *OutboxHandler) Register(g *gin.RouterGroup)` — mounts `GET /admin/outbox`

**Read `notifications.go` first** and match its structure: the narrow lister interface, `parseFilter`
returning no error, the field-by-field row mapper, the pre-allocated slice, the `pagination` struct
(already declared in `audit_logs.go` — do not redeclare it).

- [ ] **Step 1: Write the failing tests**

Create `internal/handlers/platformadmin/outbox_test.go`:

```go
package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/outbox"
)

// stubOutboxLister records the filter it was called with, so the tests can
// assert on PARSING as well as on rendering.
type stubOutboxLister struct {
	gotFilter outbox.PlatformListFilter
	result    outbox.PlatformListResult
	err       error
}

func (s *stubOutboxLister) ListPlatform(_ context.Context, _ *gorm.DB,
	f outbox.PlatformListFilter, _ time.Time) (outbox.PlatformListResult, error) {
	s.gotFilter = f
	return s.result, s.err
}

func outboxRouter(t *testing.T, lister platformadmin.OutboxLister) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewOutboxHandler(nil, lister, nil).Register(r.Group(""))
	return r
}

func getOutbox(t *testing.T, lister platformadmin.OutboxLister, query string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	outboxRouter(t, lister).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/admin/outbox"+query, nil))
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return rec, body
}

func TestOutboxEmptyIsTwoHundredAndAnArrayNotNull(t *testing.T) {
	rec, _ := getOutbox(t, &stubOutboxLister{}, "")
	require.Equal(t, http.StatusOK, rec.Code)
	// A nil slice marshals to null and defeats a caller's `?? []` exactly
	// when there is no data. The literal matters more than the parsed form.
	require.Contains(t, rec.Body.String(), `"data":[]`)
	require.NotContains(t, rec.Body.String(), `"data":null`)
}

func TestOutboxRendersRowsAndOmitsAgeForPublished(t *testing.T) {
	pubAt := time.Date(2026, 8, 26, 11, 30, 0, 0, time.UTC)
	age := int64(600)
	reason := outbox.ReasonPayloadUnparseable
	lister := &stubOutboxLister{result: outbox.PlatformListResult{
		Total: 2,
		Rows: []outbox.PlatformRow{
			{
				ID: "11111111-1111-1111-1111-111111111111", TenantID: "22222222-2222-2222-2222-222222222222",
				Aggregate: "product", AggregateID: "33333333-3333-3333-3333-333333333333",
				EventType: "product.created", Status: outbox.StatusFailed,
				CreatedAt: time.Date(2026, 8, 26, 11, 50, 0, 0, time.UTC),
				Error:     &reason, AgeSeconds: &age,
			},
			{
				ID: "44444444-4444-4444-4444-444444444444", TenantID: "22222222-2222-2222-2222-222222222222",
				Aggregate: "order", AggregateID: "55555555-5555-5555-5555-555555555555",
				EventType: "order.placed", Status: outbox.StatusPublished,
				CreatedAt: time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC),
				PublishedAt: &pubAt,
			},
		},
	}}

	rec, _ := getOutbox(t, lister, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		Data []struct {
			ID          string  `json:"id"`
			Status      string  `json:"status"`
			CreatedAt   string  `json:"created_at"`
			AgeSeconds  *int64  `json:"age_seconds"`
			PublishedAt *string `json:"published_at"`
			Error       *string `json:"error"`
		} `json:"data"`
		Pagination struct {
			Page, Limit int
			Total       int64
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Data, 2)

	require.Equal(t, outbox.StatusFailed, got.Data[0].Status)
	require.NotNil(t, got.Data[0].AgeSeconds)
	require.Equal(t, int64(600), *got.Data[0].AgeSeconds)
	require.NotNil(t, got.Data[0].Error)
	require.Equal(t, outbox.ReasonPayloadUnparseable, *got.Data[0].Error)
	require.Nil(t, got.Data[0].PublishedAt)
	require.Equal(t, "2026-08-26T11:50:00Z", got.Data[0].CreatedAt, "RFC3339 UTC")

	require.Equal(t, outbox.StatusPublished, got.Data[1].Status)
	require.Nil(t, got.Data[1].AgeSeconds, "a published row must carry no age_seconds")
	require.NotNil(t, got.Data[1].PublishedAt)
	require.Equal(t, "2026-08-26T11:30:00Z", *got.Data[1].PublishedAt)

	require.Equal(t, int64(2), got.Pagination.Total)
}

// payload is excluded by construction. This asserts on the RAW body so it
// fails if the field appears under any name or nesting.
func TestOutboxNeverEmitsPayload(t *testing.T) {
	lister := &stubOutboxLister{result: outbox.PlatformListResult{
		Total: 1,
		Rows: []outbox.PlatformRow{{
			ID: "11111111-1111-1111-1111-111111111111", TenantID: "22222222-2222-2222-2222-222222222222",
			Aggregate: "product", AggregateID: "33333333-3333-3333-3333-333333333333",
			EventType: "product.created", Status: outbox.StatusPending,
			CreatedAt: time.Date(2026, 8, 26, 11, 50, 0, 0, time.UTC),
		}},
	}}
	rec, _ := getOutbox(t, lister, "")
	require.NotContains(t, rec.Body.String(), "payload")
}

func TestOutboxParsesFilters(t *testing.T) {
	lister := &stubOutboxLister{}
	tenant := uuid.NewString()
	_, _ = getOutbox(t, lister,
		"?status=failed&event_type=product.created&older_than_minutes=45&limit=10&page=3&tenant_id="+tenant)

	require.Equal(t, outbox.StatusFailed, lister.gotFilter.Status)
	require.Equal(t, "product.created", lister.gotFilter.EventType)
	require.Equal(t, 45, lister.gotFilter.OlderThanMinutes)
	require.Equal(t, 10, lister.gotFilter.Limit)
	require.Equal(t, 3, lister.gotFilter.Page)
	require.NotNil(t, lister.gotFilter.TenantID)
	require.Equal(t, tenant, lister.gotFilter.TenantID.String())
}

// Unknown and malformed values narrow nothing rather than erroring — the
// established contract across this surface.
func TestOutboxUnknownParametersNarrowNothing(t *testing.T) {
	lister := &stubOutboxLister{}
	rec, _ := getOutbox(t, lister, "?status=banana&tenant_id=not-a-uuid&limit=abc&older_than_minutes=-5")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "banana", lister.gotFilter.Status,
		"an unknown status is passed through; the query layer decides it narrows nothing")
	require.Nil(t, lister.gotFilter.TenantID, "an unparseable tenant_id is ignored, not an error")
	require.Zero(t, lister.gotFilter.Limit, "an unparseable limit falls back to the default downstream")
	require.Zero(t, lister.gotFilter.OlderThanMinutes, "a non-positive older_than_minutes is ignored")
}

func TestOutboxRepositoryErrorIsFiveHundred(t *testing.T) {
	lister := &stubOutboxLister{err: errors.New("boom")}
	rec, body := getOutbox(t, lister, "")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal_error", body["error"])
}

// The golden file pins the console's contract. If this fails because the
// shape changed deliberately, update the fixture to match the handler —
// never the handler to match the fixture.
func TestOutboxResponseMatchesGoldenShape(t *testing.T) {
	pubAt := time.Date(2026, 8, 26, 11, 30, 0, 0, time.UTC)
	age := int64(600)
	reason := outbox.ReasonStoreNotFound
	lister := &stubOutboxLister{result: outbox.PlatformListResult{
		Total: 2,
		Rows: []outbox.PlatformRow{
			{
				ID: "11111111-1111-1111-1111-111111111111", TenantID: "22222222-2222-2222-2222-222222222222",
				Aggregate: "product", AggregateID: "33333333-3333-3333-3333-333333333333",
				EventType: "product.created", Status: outbox.StatusFailed,
				CreatedAt: time.Date(2026, 8, 26, 11, 50, 0, 0, time.UTC),
				Error:     &reason, AgeSeconds: &age,
			},
			{
				ID: "44444444-4444-4444-4444-444444444444", TenantID: "22222222-2222-2222-2222-222222222222",
				Aggregate: "order", AggregateID: "55555555-5555-5555-5555-555555555555",
				EventType: "order.placed", Status: outbox.StatusPublished,
				CreatedAt: time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC),
				PublishedAt: &pubAt,
			},
		},
	}}
	rec, _ := getOutbox(t, lister, "")

	want, err := os.ReadFile("testdata/outbox_response.json")
	require.NoError(t, err)

	var gotAny, wantAny any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &gotAny))
	require.NoError(t, json.Unmarshal(want, &wantAny))
	require.Equal(t, wantAny, gotAny)
}
```

Create `internal/handlers/platformadmin/testdata/outbox_response.json`:

```json
{
  "data": [
    {
      "id": "11111111-1111-1111-1111-111111111111",
      "tenant_id": "22222222-2222-2222-2222-222222222222",
      "aggregate": "product",
      "aggregate_id": "33333333-3333-3333-3333-333333333333",
      "event_type": "product.created",
      "status": "failed",
      "created_at": "2026-08-26T11:50:00Z",
      "age_seconds": 600,
      "error": "store_not_found"
    },
    {
      "id": "44444444-4444-4444-4444-444444444444",
      "tenant_id": "22222222-2222-2222-2222-222222222222",
      "aggregate": "order",
      "aggregate_id": "55555555-5555-5555-5555-555555555555",
      "event_type": "order.placed",
      "status": "published",
      "created_at": "2026-08-26T11:00:00Z",
      "published_at": "2026-08-26T11:30:00Z"
    }
  ],
  "pagination": { "page": 1, "limit": 50, "total": 2 }
}
```

- [ ] **Step 2: Run them to verify they fail**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
go test -count=1 -run 'TestOutbox' ./internal/handlers/platformadmin/ -v 2>&1 | tail -20
```

Expected: **compile failure** — `undefined: platformadmin.NewOutboxHandler`,
`undefined: platformadmin.OutboxLister`.

- [ ] **Step 3: Create the handler**

Create `internal/handlers/platformadmin/outbox.go`:

```go
package platformadmin

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
)

// OutboxLister is the subset of the outbox platform read this handler
// needs. Narrowed to one method for the same reason as NotificationLister
// in notifications.go and TicketLister in tickets.go.
type OutboxLister interface {
	ListPlatform(ctx context.Context, db *gorm.DB, f outbox.PlatformListFilter,
		asOf time.Time) (outbox.PlatformListResult, error)
}

// OutboxHandler serves GET /admin/outbox to the platform console — a
// cross-tenant read of outbox_events answering "what is stuck, what failed,
// and why" (#331).
//
// This surface exists because a row with published_at IS NULL and a non-null
// error, hours old, means a downstream integration is silently not
// happening, and nothing outside mark8ly could see it. Before #336 the
// `failed` state could not occur at all: nothing wrote outbox_events.error.
type OutboxHandler struct {
	db     *gorm.DB
	repo   OutboxLister
	logger *slog.Logger
	now    func() time.Time
}

// NewOutboxHandler constructs the handler. logger may be nil.
func NewOutboxHandler(db *gorm.DB, repo OutboxLister, logger *slog.Logger) *OutboxHandler {
	return &OutboxHandler{db: db, repo: repo, logger: logger, now: time.Now}
}

// Register mounts the route on the supplied group.
func (h *OutboxHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/outbox", h.List)
}

// outboxRow is the pinned contract shape.
//
// `payload` is DELIBERATELY absent, and absent by CONSTRUCTION: this struct
// is populated field by field from outbox.PlatformRow, which has no payload
// field either, so a column added to the model tomorrow cannot leak. It is
// arbitrary JSONB that may carry customer data, and a governance surface
// listing stuck events does not need it. Same reasoning that keeps
// `message` out of #332 and `description` out of #329.
//
// `error` is emitted as an OPAQUE string. outbox_events.error has no CHECK
// constraint and the operator requeue path is a raw UPDATE, so the codes
// this service writes are not the only values a consumer can observe. The
// console must render it with an unknown-value fallback, never a switch.
//
// `age_seconds` is absent for a published row: that row is settled and has
// no waiting time. A number that grew forever there would read as "stuck"
// beside a genuinely stuck row.
type outboxRow struct {
	ID          string  `json:"id"`
	TenantID    string  `json:"tenant_id"`
	Aggregate   string  `json:"aggregate"`
	AggregateID string  `json:"aggregate_id"`
	EventType   string  `json:"event_type"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	AgeSeconds  *int64  `json:"age_seconds,omitempty"`
	PublishedAt *string `json:"published_at,omitempty"`
	Error       *string `json:"error,omitempty"`
}

type outboxListResponse struct {
	Data       []outboxRow `json:"data"`
	Pagination pagination  `json:"pagination"`
}

// List handles GET /admin/outbox.
func (h *OutboxHandler) List(c *gin.Context) {
	// One instant for both the age and the older_than_minutes cutoff, so a
	// rendered age can never disagree with the filter that selected it.
	asOf := h.now().UTC()
	filter := h.parseFilter(c)

	result, err := h.repo.ListPlatform(c.Request.Context(), h.db, filter, asOf)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("platform outbox list", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "could not read outbox events",
		})
		return
	}

	// Allocate before appending: a nil slice marshals to null, which
	// defeats a caller's `?? []` precisely when there is no data.
	rows := make([]outboxRow, 0, len(result.Rows))
	for _, r := range result.Rows {
		rows = append(rows, toOutboxRow(r))
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = outbox.DefaultPlatformPageSize
	}
	if limit > outbox.MaxPlatformPageSize {
		limit = outbox.MaxPlatformPageSize
	}

	c.JSON(http.StatusOK, outboxListResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(filter.Page, 1),
			Limit: limit,
			Total: result.Total,
		},
	})
}

// toOutboxRow maps a query row to the pinned contract shape, FIELD BY
// FIELD. Nothing iterates the source struct, so the absence of payload is a
// property of this projection rather than of what the query happened to
// select.
func toOutboxRow(r outbox.PlatformRow) outboxRow {
	row := outboxRow{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Aggregate:   r.Aggregate,
		AggregateID: r.AggregateID,
		EventType:   r.EventType,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt.UTC().Format(time.RFC3339),
		AgeSeconds:  r.AgeSeconds,
		Error:       r.Error,
	}
	if r.PublishedAt != nil {
		s := r.PublishedAt.UTC().Format(time.RFC3339)
		row.PublishedAt = &s
	}
	return row
}

// parseFilter never returns an error. A missing parameter takes the
// default, an unparseable one is ignored, and an oversized limit clamps
// downstream rather than refusing — matching audit logs (#276), tickets
// (#329) and notifications (#332).
func (h *OutboxHandler) parseFilter(c *gin.Context) outbox.PlatformListFilter {
	f := outbox.PlatformListFilter{
		Status:    strings.TrimSpace(c.Query("status")),
		EventType: strings.TrimSpace(c.Query("event_type")),
		Page:      1,
	}

	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("older_than_minutes")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.OlderThanMinutes = n
		}
	}
	if v := strings.TrimSpace(c.Query("since_hours")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			from := h.now().UTC().Add(-time.Duration(n) * time.Hour)
			f.From = &from
		}
	}
	// tenant_id NARROWS rather than scopes — this endpoint is cross-tenant
	// by design.
	if v := strings.TrimSpace(c.Query("tenant_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.TenantID = &id
		}
	}
	return f
}
```

**Note on `Page`:** `parseFilter` defaults it to `1` and the test
`TestOutboxUnknownParametersNarrowNothing` asserts `Limit` and `OlderThanMinutes` are zero but does
not assert `Page`. That is deliberate — `Page` has a real default, the other two are resolved
downstream.

- [ ] **Step 4: Run to verify GREEN**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
go test -count=1 ./internal/handlers/platformadmin/ -v > /tmp/t331-h.txt 2>&1
echo "exit=$?"
echo "PASS=$(grep -cE '^\s*--- PASS' /tmp/t331-h.txt) FAIL=$(grep -cE '^\s*--- FAIL' /tmp/t331-h.txt)"
```

Expected: all `TestOutbox*` pass, and every pre-existing test in the package still passes.

- [ ] **Step 5: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/handlers/platformadmin/outbox.go \
        services/marketplace-api/internal/handlers/platformadmin/outbox_test.go \
        services/marketplace-api/internal/handlers/platformadmin/testdata/outbox_response.json
git commit -m "feat(platformadmin): GET /admin/outbox handler and pinned contract shape"
```

---

## Task 3: mount it

**Files:**
- Modify: `internal/handlers/platformadmin/routes.go`
- Create: `internal/handlers/platformadmin/routes_outbox_test.go`
- Modify: `cmd/marketplace-api/main.go`

**Interfaces:**
- Consumes: `NewOutboxHandler`, `OutboxLister` from Task 2; `outbox.ListPlatform` from Task 1.
- Produces: `Deps.Outbox OutboxLister`, and a mounted `GET /api/v1/platform/admin/outbox`.

**The trap in this task.** `platformadmin.Deps` is constructed at **TWO** sites in `main.go` —
around lines 2037 and 2161 (one per server mode). Wiring only one leaves the route unmounted in the
other mode, and nothing in the build or the unit tests would catch it. Find both with
`grep -n 'platformadmin.Deps{' cmd/marketplace-api/main.go` and wire both.

`outbox.ListPlatform` is a plain function, not a method, so it cannot be assigned to an interface
directly. Follow the `TrialListerFunc` adapter pattern already used in this file for
`trial.ListExpiring` — see `Trials: platformadmin.TrialListerFunc(trial.ListExpiring)` in `main.go`,
and the `TrialListerFunc` definition in `routes.go`.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/platformadmin/routes_outbox_test.go`:

```go
package platformadmin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// The route must be mounted, and must sit BEHIND the signature check. An
// unsigned request gets 401 (or 503 when the surface is unconfigured) —
// never 404, which would mean the route does not exist, and never 200,
// which would mean a cross-tenant read is open.
func TestOutboxRouteIsMountedBehindAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Secret: "test-secret",
		Outbox: platformadmin.OutboxListerFunc(nil),
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/platform/admin/outbox", nil))
	require.NotEqual(t, http.StatusNotFound, rec.Code, "route must be mounted")
	require.NotEqual(t, http.StatusOK, rec.Code, "route must not answer an unsigned request")
}

// A nil dependency leaves the route unmounted, matching the nil-safe
// pattern every other read route on this surface uses.
func TestOutboxRouteUnmountedWhenDependencyNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Secret: "test-secret",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/platform/admin/outbox", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
go test -count=1 -run 'TestOutboxRoute' ./internal/handlers/platformadmin/ -v 2>&1 | tail -15
```

Expected: **compile failure** — `Deps has no field Outbox`, `undefined: platformadmin.OutboxListerFunc`.

- [ ] **Step 3: Add the adapter and the Deps field**

In `internal/handlers/platformadmin/outbox.go`, add below the `OutboxLister` interface:

```go
// OutboxListerFunc adapts a plain function to OutboxLister, so
// outbox.ListPlatform — which is a package function, not a method — can be
// wired directly in main.go. Same pattern as TrialListerFunc.
type OutboxListerFunc func(ctx context.Context, db *gorm.DB, f outbox.PlatformListFilter,
	asOf time.Time) (outbox.PlatformListResult, error)

func (fn OutboxListerFunc) ListPlatform(ctx context.Context, db *gorm.DB,
	f outbox.PlatformListFilter, asOf time.Time) (outbox.PlatformListResult, error) {
	return fn(ctx, db, f, asOf)
}
```

In `internal/handlers/platformadmin/routes.go`, add to `Deps` immediately after the `Notifications`
field:

```go
	// Outbox serves /admin/outbox (#331), the cross-tenant read of stuck and
	// failed outbox events. Nil leaves that route unmounted, matching the
	// nil-safe pattern used for the other optional read routes above.
	//
	// This became answerable only with #336: before it, nothing wrote
	// outbox_events.error, so the `failed` status could never have matched a
	// row and this endpoint would have reported a permanently empty set
	// while looking as though it worked.
	Outbox OutboxLister
```

- [ ] **Step 4: Mount the route**

In `internal/handlers/platformadmin/routes.go`, immediately after the `deps.Notifications` block:

```go
	if deps.Outbox != nil {
		NewOutboxHandler(deps.DB, deps.Outbox, deps.Logger).Register(group)
	}
```

- [ ] **Step 5: Run to verify GREEN**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
go test -count=1 ./internal/handlers/platformadmin/ > /tmp/t331-r.txt 2>&1
echo "exit=$?"
tail -3 /tmp/t331-r.txt
```

Expected: whole package passes, including
`TestAllWriteRoutesDeclareACapability` in `routes_capability_coverage_test.go` — that test builds the
real router and fails if a **write** route lacks a capability entry. `/admin/outbox` is a GET, so it
must NOT appear in `RequiredWriteCapabilities`; if that test fails, something registered it as a
write.

- [ ] **Step 6: Wire BOTH main.go sites**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
grep -n 'platformadmin.Deps{' cmd/marketplace-api/main.go
```

There are two. In **each** one, add this line immediately after the existing `Notifications:` line,
matching the surrounding alignment:

```go
			Outbox:                platformadmin.OutboxListerFunc(outbox.ListPlatform),
```

`outbox` is already imported in `main.go` (the publisher is constructed there), so no import change
is needed — verify rather than assume.

- [ ] **Step 7: Verify both sites and build**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
echo "Deps sites: $(grep -c 'platformadmin.Deps{' cmd/marketplace-api/main.go)"
echo "Outbox wirings: $(grep -c 'Outbox:.*OutboxListerFunc' cmd/marketplace-api/main.go)"
go build ./... && echo "build ok"
```

**The two counts must be equal.** If wirings is 1 and sites is 2, one server mode silently lacks the
route.

- [ ] **Step 8: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/handlers/platformadmin/outbox.go \
        services/marketplace-api/internal/handlers/platformadmin/routes.go \
        services/marketplace-api/internal/handlers/platformadmin/routes_outbox_test.go \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(platformadmin): mount GET /admin/outbox in both server modes"
```

---

## Task 4: whole-branch verification

**Files:** none modified — this task produces evidence.

- [ ] **Step 1: Build-tagged compile check**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
go vet -tags=integration ./... > /tmp/t331-vet.txt 2>&1
echo "exit=$?"
tail -10 /tmp/t331-vet.txt
```

Expected: `exit=0`.

- [ ] **Step 2: Capture the branch failing set**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 ./... > /tmp/t331-branch.txt 2>&1
echo "go test exit=$?"
grep -E '^FAIL\s+github' /tmp/t331-branch.txt | awk '{print $2}' | sort -u > /tmp/t331-branch-pkgs.txt
wc -l < /tmp/t331-branch-pkgs.txt
```

A non-zero exit is EXPECTED — this repo has a large pre-existing failing set. The exit code is not
the verdict; the diff is.

- [ ] **Step 3: Capture the baseline**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git worktree remove /tmp/m8-331-baseline --force 2>/dev/null
git worktree add /tmp/m8-331-baseline e8fc6dd7
cd /tmp/m8-331-baseline/services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 ./... > /tmp/t331-main.txt 2>&1
echo "go test exit=$?"
grep -E '^FAIL\s+github' /tmp/t331-main.txt | awk '{print $2}' | sort -u > /tmp/t331-main-pkgs.txt
wc -l < /tmp/t331-main-pkgs.txt
```

**Run the two suites SEQUENTIALLY, never concurrently — they share one database** and would corrupt
each other's fixtures, making both results meaningless.

- [ ] **Step 4: Diff both directions**

```bash
echo ">>> failing on BRANCH but not baseline (must be empty):"
comm -13 /tmp/t331-main-pkgs.txt /tmp/t331-branch-pkgs.txt
echo ">>> failing on baseline but not BRANCH:"
comm -23 /tmp/t331-main-pkgs.txt /tmp/t331-branch-pkgs.txt
```

Expected: **both empty.** This branch fixes no pre-existing test and should break none, so identical
sets is the passing result. A package in the first list is a defect this branch introduced — **stop
and report it, do not fix it silently.** A package in the second list is also unexpected and needs
explaining.

- [ ] **Step 5: Clean up**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git worktree remove /tmp/m8-331-baseline --force
git worktree list
```

- [ ] **Step 6: Record the evidence**

Append a "Verification record" section to this plan file with the `go vet` result, both counts, the
both-directions comparison and the verdict, then commit:

```bash
git add docs/superpowers/plans/2026-08-26-admin-outbox-endpoint.md
git commit -m "docs: record the verification diff for the admin outbox endpoint branch"
```

---

## Not in this plan

- **A detail view, or any `payload` access.** Spec §5. It is arbitrary JSONB that may carry customer
  data; if the console later needs it, that is a separate decision with its own review.
- **Console-side work.** This ships the API only.
- **Production verification.** The surface is signed, so exercising it end-to-end needs the platform
  HMAC secret. Handled separately, with explicit go-ahead. Note production currently has **0 pending
  and 0 errored** rows, so a live call would legitimately return an empty page — which proves the
  route answers, not that the projection is right.
- **`total` cost for `status=published`.** Counts across a never-pruned table on a shared
  db-f1-micro. At 688 rows it is immaterial and it is the shape every sibling already has. Spec §5
  records the decision to revisit only if the table grows enough for the count to dominate.
