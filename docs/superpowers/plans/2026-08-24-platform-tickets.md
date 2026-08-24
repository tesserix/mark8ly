# GET /admin/tickets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `GET /api/v1/platform/admin/tickets` (#329) — a cross-store ticket read, so the console's already-live `platform.tickets` queue stops silently omitting every mark8ly merchant ticket.

**Architecture:** A new, explicitly cross-store repository method beside the existing store-scoped `List` (which stays fail-safe), and a handler on the `platformadmin` group mirroring #276's `/admin/audit-logs`.

**Tech Stack:** Go 1.26, Gin, GORM, Postgres, `services/marketplace-api`.

**Spec:** `docs/superpowers/specs/2026-08-24-platform-tickets-design.md`

## Global Constraints

- **Query through the `ticket` package's model so `TableName()` decides.** Tickets live in **`support_tickets`**; the bare `tickets` table in the same schema belongs to a different platform-engineering system and must not be touched. Never hand-write either table name.
- **Do NOT modify `ticket.ListFilter` or `List`.** They hardcode `WHERE store_id = ? AND tenant_id = ?`, so zero UUIDs match nothing — fail-safe. Making zero mean "all" would turn the merchant-facing path fail-open. Add a separate cross-store method.
- **Project, do not pass through.** Map `ticket.Ticket` field by field. `description` and `replies` must be absent as a property of the projection, not of what the query happened to select.
- There is **no `assignee` column** anywhere — no such filter.
- Envelope exactly `{"data": [...], "pagination": {"page","limit","total"}}`; empty is `200` + `[]` allocated with `make([]ticketRow, 0, n)`; `pagination.limit` is the **effective** (clamped) limit; timestamps RFC3339 UTC with offset; ids bare.
- **`pagination` and `listResponse` already exist** in `internal/handlers/platformadmin/audit_logs.go`. **Reuse `pagination`**; do NOT redeclare it (same package — it will not compile). `listResponse` is bound to `auditRow`, so tickets need their own response type name.
- Commits: single-line conventional, NO signature, NO `Co-Authored-By`. Module path `github.com/mark8ly/marketplace-api`.
- Integration tests: `//go:build integration`, `-p 1`, `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'` — LAN IP, never localhost. `testdb` **skips silently** when unset and a skip looks exactly like a pass; confirm from verbose output that tests RAN.
- Run **`go vet -tags=integration ./...`**: `go build`/`go vet`/`go test` never compile build-tagged files.
- Pre-existing unrelated failures in `internal/subscription` and `internal/billing/trial` (#316/#317) — not yours; scope with `-run` and say so.
- Ignore the pre-existing `go.work requires go >= 1.26.6` diagnostic.

---

### Task 1: the cross-store repository method

**Files:**
- Modify: `services/marketplace-api/internal/ticket/repository.go`
- Test: `services/marketplace-api/internal/ticket/platform_list_integration_test.go` (create)

**Interfaces:**
- Consumes: the existing `ticket.Ticket` model.
- Produces, used by Task 2:

```go
// MaxPlatformPageSize and DefaultPlatformPageSize mirror the audit package's
// values so every cross-tenant read on the platform surface clamps alike.
const MaxPlatformPageSize = 500
const DefaultPlatformPageSize = 50

// PlatformListFilter is the CROSS-STORE filter. It is deliberately a separate
// type from ListFilter: that one requires a store and a tenant and matches
// nothing without them, which is the safe failure for a merchant-facing query.
// Widening it to mean "all stores when unset" would make a forgotten field a
// cross-store leak.
type PlatformListFilter struct {
	StoreID  *uuid.UUID // optional NARROWING, not a scope
	Status   string
	Priority string
	From     *time.Time
	To       *time.Time
	Page     int
	Limit    int
}

func (gormRepository) ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter) (ListResult, error)
```

Add `ListPlatform` to the `Repository` interface. Check for other implementations of that interface (test fakes) and add the method there too — `grep -rn "ticket.Repository" --include='*.go'`.

- [ ] **Step 1: Write the failing integration test**

The test that matters is cross-store: a single-store fixture cannot distinguish this method from the store-scoped `List` it exists to complement.

```go
//go:build integration

package ticket_test

// TestListPlatform_SpansStoresAndTenants is the whole point of the method.
// Two tickets under two different stores in two different tenants must both
// come back from one unfiltered call. A fixture with one store would pass
// against the store-scoped List too, and prove nothing.
func TestListPlatform_SpansStoresAndTenants(t *testing.T) {
	db := testdb.NewDB(t, "support_tickets")
	repo := ticket.NewRepository()

	tenantA, storeA := uuid.New(), uuid.New()
	tenantB, storeB := uuid.New(), uuid.New()
	seedTicket(t, db, tenantA, storeA, "open", "high", "Alpha subject")
	seedTicket(t, db, tenantB, storeB, "open", "low", "Beta subject")

	got, err := repo.ListPlatform(context.Background(), db, ticket.PlatformListFilter{Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Total)

	subjects := map[string]bool{}
	for _, tk := range got.Tickets {
		subjects[tk.Subject] = true
	}
	require.True(t, subjects["Alpha subject"], "ticket from tenant A / store A must appear")
	require.True(t, subjects["Beta subject"], "ticket from tenant B / store B must appear")
}

// store_id NARROWS; it is not a required scope. Both directions asserted,
// because a filter that always applies and a filter that never applies both
// pass a one-sided test.
func TestListPlatform_StoreIDNarrowsRatherThanScopes(t *testing.T) {
	db := testdb.NewDB(t, "support_tickets")
	repo := ticket.NewRepository()

	tenantA, storeA := uuid.New(), uuid.New()
	tenantB, storeB := uuid.New(), uuid.New()
	seedTicket(t, db, tenantA, storeA, "open", "high", "Alpha subject")
	seedTicket(t, db, tenantB, storeB, "open", "low", "Beta subject")

	all, err := repo.ListPlatform(context.Background(), db, ticket.PlatformListFilter{Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), all.Total, "unset store_id must return every store")

	narrowed, err := repo.ListPlatform(context.Background(), db,
		ticket.PlatformListFilter{StoreID: &storeA, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), narrowed.Total)
	require.Equal(t, "Alpha subject", narrowed.Tickets[0].Subject)
}

// The existing store-scoped List must STAY fail-safe. If someone later makes a
// zero StoreID mean "all stores", this fails — which is the point.
func TestList_ZeroStoreIDStillMatchesNothing(t *testing.T) {
	db := testdb.NewDB(t, "support_tickets")
	repo := ticket.NewRepository()

	seedTicket(t, db, uuid.New(), uuid.New(), "open", "high", "Alpha subject")

	got, err := repo.List(context.Background(), db, ticket.ListFilter{PerPage: 50})
	require.NoError(t, err)
	require.Equal(t, int64(0), got.Total,
		"a zero StoreID must match NOTHING; 'all stores' would be fail-open on the merchant path")
}

func TestListPlatform_FiltersByStatusAndPriority(t *testing.T) {
	db := testdb.NewDB(t, "support_tickets")
	repo := ticket.NewRepository()
	tenant, store := uuid.New(), uuid.New()

	seedTicket(t, db, tenant, store, "open", "high", "Open high")
	seedTicket(t, db, tenant, store, "resolved", "high", "Resolved high")
	seedTicket(t, db, tenant, store, "open", "low", "Open low")

	byStatus, err := repo.ListPlatform(context.Background(), db,
		ticket.PlatformListFilter{Status: "open", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), byStatus.Total)

	byPriority, err := repo.ListPlatform(context.Background(), db,
		ticket.PlatformListFilter{Status: "open", Priority: "low", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), byPriority.Total)
	require.Equal(t, "Open low", byPriority.Tickets[0].Subject)
}
```

Write `seedTicket` in the same file. `support_tickets` requires `ticket_number`, `subject`, `description`, `submitted_by_name`, `submitted_by_email` as NOT NULL — read `internal/ticket/models.go` and the migration for the full set rather than guessing, and give each row a distinct subject so assertions cannot pass on the wrong row.

- [ ] **Step 2: Run to verify it fails**

```bash
cd services/marketplace-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags integration -p 1 ./internal/ticket/ -run 'TestListPlatform|TestList_Zero' -v
```
Expected: FAIL — `ListPlatform` undefined. Confirm from the output that the tests **ran** rather than skipped.

- [ ] **Step 3: Implement `ListPlatform`**

```go
func (gormRepository) ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter) (ListResult, error) {
	var result ListResult
	// Model(&Ticket{}) so TableName() picks support_tickets. The bare
	// `tickets` table belongs to a different system — see the model.
	q := db.WithContext(ctx).Model(&Ticket{})

	// StoreID NARROWS. Unset means every store, which is the whole point of
	// this method and exactly why it is not ListFilter.
	if f.StoreID != nil {
		q = q.Where("store_id = ?", *f.StoreID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Priority != "" {
		q = q.Where("priority = ?", f.Priority)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at <= ?", *f.To)
	}

	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("ticket platform list count: %w", err)
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

	if err := q.Order("created_at DESC").
		Limit(limit).Offset((page - 1) * limit).
		Find(&result.Tickets).Error; err != nil {
		return result, fmt.Errorf("ticket platform list: %w", err)
	}
	return result, nil
}
```

Do NOT preload `Replies` — the projection excludes them and loading them would pull customer-written content into memory for no reason.

- [ ] **Step 4: Run the tests** — expected PASS, and confirm they RAN.

- [ ] **Step 5: MUTATION — prove the cross-store test discriminates**

Add `q = q.Where("store_id = ?", f.StoreID)` unconditionally (the store-scoped behaviour this method exists to avoid). Re-run.
Expected: **FAIL** in `TestListPlatform_SpansStoresAndTenants`. Record the test name and the actual failure text. Revert.

If it still passes, the fixture is not spanning stores and the test proves nothing.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/ticket/
git commit -m "feat(ticket): cross-store ListPlatform beside the fail-safe store-scoped List (#329)"
```

---

### Task 2: the handler

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/tickets.go`
- Modify: `services/marketplace-api/internal/handlers/platformadmin/routes.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` (**both** `Register` sites)
- Test: `services/marketplace-api/internal/handlers/platformadmin/tickets_test.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/testdata/tickets_response.json`

**Interfaces:**
- Consumes: `ticket.PlatformListFilter`, `ticket.ListResult` from Task 1.
- Produces: a narrow interface on `Deps` so the handler is stubbable, matching how `EstateCounts` and `OnboardingFunnel` are declared in this package:

```go
type TicketLister interface {
	ListPlatform(ctx context.Context, db *gorm.DB, f ticket.PlatformListFilter) (ticket.ListResult, error)
}
```

- [ ] **Step 1: Write the failing tests**

```go
// Values are DISTINCT and NON-ZERO so an assertion cannot pass on a zero
// fabricated by a missing field. Two tickets, two stores, two tenants — the
// shape this endpoint exists to return.
func ticketsFixture() []ticket.Ticket {
	conv := "conv-abc123"
	resolved := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	return []ticket.Ticket{
		{
			ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			TenantID: uuid.MustParse("aaaaaaaa-1111-1111-1111-111111111111"),
			StoreID: uuid.MustParse("bbbbbbbb-1111-1111-1111-111111111111"),
			TicketNumber: "T-1042", Subject: "Refund not received",
			Description: "MUST NOT APPEAR IN THE RESPONSE",
			Status: "open", Priority: "high",
			SubmittedByName: "Ada Lovelace", SubmittedByEmail: "ada@example.com",
			ConversationID: &conv,
			CreatedAt: time.Date(2026, 8, 19, 8, 30, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 8, 19, 11, 45, 0, 0, time.UTC),
		},
		{
			ID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			TenantID: uuid.MustParse("aaaaaaaa-2222-2222-2222-222222222222"),
			StoreID: uuid.MustParse("bbbbbbbb-2222-2222-2222-222222222222"),
			TicketNumber: "T-2087", Subject: "Wrong size delivered",
			Description: "MUST NOT APPEAR IN THE RESPONSE EITHER",
			Status: "resolved", Priority: "low",
			SubmittedByName: "Grace Hopper", SubmittedByEmail: "grace@example.com",
			ResolvedAt: &resolved,
			CreatedAt: time.Date(2026, 8, 18, 7, 15, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC),
		},
	}
}

// THE test for the projection: assert against RAW JSON, because an
// unmarshalled struct cannot distinguish an absent key from an empty one.
func TestTickets_OmitsDescriptionAndReplies(t *testing.T) {
	rec := getTickets(t, &stubTicketLister{result: ticket.ListResult{
		Tickets: ticketsFixture(), Total: 2,
	}})
	require.Equal(t, http.StatusOK, rec.Code)

	var raw struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	require.Len(t, raw.Data, 2)
	for i, row := range raw.Data {
		_, hasDesc := row["description"]
		_, hasReplies := row["replies"]
		require.False(t, hasDesc, "row %d must not carry the customer-written description", i)
		require.False(t, hasReplies, "row %d must not carry replies", i)
	}
}

func TestTickets_EmptyIsArrayNotNull(t *testing.T) {
	rec := getTickets(t, &stubTicketLister{result: ticket.ListResult{}})
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"data":[]`,
		"a nil slice marshals to null and defeats the caller's ?? []")
}

// An oversized limit clamps and pagination.limit reports the EFFECTIVE value,
// so total/limit is a correct page count.
func TestTickets_LimitClampsAndIsReportedEffective(t *testing.T) {
	stub := &stubTicketLister{result: ticket.ListResult{Total: 1200}}
	rec := getTicketsWithQuery(t, stub, "?limit=100000")
	require.Equal(t, ticket.MaxPlatformPageSize, stub.gotFilter.Limit, "the repo must receive the clamped limit")
	var body struct{ Pagination struct{ Limit int `json:"limit"` } `json:"pagination"` }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, ticket.MaxPlatformPageSize, body.Pagination.Limit)
}

// A missing parameter takes the default; it never errors. Assert what the
// REPOSITORY received — asserting only the response would pass even if the
// handler sent 0 and the repo happened to default it downstream.
func TestTickets_MissingLimitTakesDefault(t *testing.T) {
	stub := &stubTicketLister{}
	rec := getTicketsWithQuery(t, stub, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, ticket.DefaultPlatformPageSize, stub.gotFilter.Limit)
}

// A non-numeric limit is not an error either: it takes the default, matching
// how #276 treats a malformed parameter.
func TestTickets_MalformedLimitTakesDefaultNotError(t *testing.T) {
	stub := &stubTicketLister{}
	rec := getTicketsWithQuery(t, stub, "?limit=not-a-number")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, ticket.DefaultPlatformPageSize, stub.gotFilter.Limit)
}

// store_id reaches the repository as a NARROWING filter, not a scope.
func TestTickets_StoreIDIsPassedThroughAsNarrowing(t *testing.T) {
	stub := &stubTicketLister{}
	id := uuid.New()
	getTicketsWithQuery(t, stub, "?store_id="+id.String())
	require.NotNil(t, stub.gotFilter.StoreID)
	require.Equal(t, id, *stub.gotFilter.StoreID)

	stub2 := &stubTicketLister{}
	getTicketsWithQuery(t, stub2, "")
	require.Nil(t, stub2.gotFilter.StoreID, "absent store_id must stay nil, meaning every store")
}

// from/to win over since_hours when both are supplied, matching #276. Pin the
// EXACT instant that reaches the repository: asserting merely that From is
// non-nil would pass whichever source won.
func TestTickets_ExplicitRangeWinsOverSinceHours(t *testing.T) {
	stub := &stubTicketLister{}
	from := "2026-08-01T00:00:00Z"
	getTicketsWithQuery(t, stub, "?since_hours=24&from="+from)

	require.NotNil(t, stub.gotFilter.From)
	want, err := time.Parse(time.RFC3339, from)
	require.NoError(t, err)
	require.True(t, stub.gotFilter.From.Equal(want),
		"explicit from must win over since_hours; got %v", stub.gotFilter.From)
}

// And with only since_hours, From is derived from it rather than left unset.
func TestTickets_SinceHoursAppliesWhenNoExplicitRange(t *testing.T) {
	stub := &stubTicketLister{}
	getTicketsWithQuery(t, stub, "?since_hours=24")
	require.NotNil(t, stub.gotFilter.From, "since_hours must produce a From bound")
}

// A repository failure must never render as an empty success — an operator
// reading `data: []` would conclude there are no tickets when the query blew up.
func TestTickets_RepoErrorIs500NotEmptySuccess(t *testing.T) {
	rec := getTickets(t, &stubTicketLister{err: errors.New("pq: connection refused")})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), `"data"`,
		"a failed read must not shape a result at all")
	require.NotContains(t, rec.Body.String(), "connection refused",
		"driver error text must be logged server-side, never echoed")
}
```

Fill in the helpers (`getTickets`, `getTicketsWithQuery`, `stubTicketLister` recording `gotFilter`) following how `kpisRouter` and the signed-request helpers already work in this package. Requests must carry a valid signature or they are rejected before the handler.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/handlers/platformadmin/ -run TestTickets -v`. Expected: undefined.

- [ ] **Step 3: Implement the handler**

Mirror `audit_logs.go`: a `ticketRow` projection, filter parsing with the same clamping rules, and the shared `pagination` type. **Reuse `pagination`; do not redeclare it** — it already exists in `audit_logs.go` in this package. Name the response type `ticketListResponse` (`listResponse` is bound to `auditRow`).

```go
// ticketRow is the pinned contract shape. description and replies are
// DELIBERATELY absent: a cross-tenant governance surface must not become a
// way to read every merchant's customer correspondence. Same reasoning that
// keeps `payload` out of #331 and message bodies out of #332. A body view
// needs its own endpoint, capability and justification.
//
// There is no assignee field because no such column exists anywhere in the
// ticket schema — #329 asked for one; tickets have a submitter (#329 comment).
type ticketRow struct {
	ID             string  `json:"id"`
	TicketNumber   string  `json:"ticket_number"`
	TenantID       string  `json:"tenant_id"`
	StoreID        string  `json:"store_id"`
	Subject        string  `json:"subject"`
	Status         string  `json:"status"`
	Priority       string  `json:"priority"`
	RequesterName  string  `json:"requester_name"`
	RequesterEmail string  `json:"requester_email"`
	ConversationID *string `json:"conversation_id,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	ResolvedAt     *string `json:"resolved_at,omitempty"`
}
```

Map field by field in a `toTicketRow` function. Ids go out bare; timestamps `RFC3339` in UTC.

- [ ] **Step 4: Run the tests** — expected PASS.

- [ ] **Step 5: Mount and wire**

Register on the `platformadmin` group behind the non-nil guard pattern the neighbouring routes use, and construct the dependency at **both** `platformadmin.Register` call sites in `main.go`. `cmd/marketplace-api/wiring_test.go` asserts both `Deps` literals carry the same field set, so a one-site change fails it — let it.

- [ ] **Step 6: Golden fixture**

Write `testdata/tickets_response.json` from real handler output, compare with `require.JSONEq`, and prove by mutation that it catches a field **rename** and a field **addition**. Both must fail. A fixture that only catches omissions is theatre.

- [ ] **Step 7: MUTATIONS — two, both required**

1. Add `Description string \`json:"description"\`` to `ticketRow` and populate it → `TestTickets_OmitsDescriptionAndReplies` **and** the golden test must FAIL.
2. Ignore the `store_id` query parameter (always pass nil) → `TestTickets_StoreIDIsPassedThroughAsNarrowing` must FAIL.

Record the failing test names and actual failure text. Revert each.

- [ ] **Step 8: Full run and commit**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./... \
  && go test ./internal/ticket/ ./internal/handlers/platformadmin/ ./cmd/marketplace-api/
git add services/marketplace-api/
git commit -m "feat(platformadmin): GET /admin/tickets, the cross-store ticket read (#329)"
```

---

## After the plan

**Comment on #329** that the `assignee` filter was omitted because no such column exists anywhere in the ticket schema — tickets carry a submitter, not an assignee. This is the third issue in the series to name a field that does not exist (#277's tenant slug, #276's `metadata` shape), so it is worth recording on the umbrella too.

**Verification after deploy** — separate the checks that carry information from those that merely mean "no data reached this code":

- *Carries information:* the route answers under signature; an unsigned request is rejected while a bogus path 404s (so the 401 means "mounted"); `data` is `[]` and never `null`; an oversized `limit` clamps and `pagination.limit` reports the effective value.
- *Proves less:* an empty `200`. Production has 4 tenants and 4 stores; if no support tickets exist, an empty list proves the query ran, **not** that it spans stores. The cross-store integration fixture is what establishes that.

**Do not test by creating a ticket on a real merchant's store.**
