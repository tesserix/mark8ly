# Multi-warehouse PR 2: the allocator — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A pure `internal/allocation` package that decides which warehouse ships how much of each order line, with no database access and no callers.

**Architecture:** One package, two files, both pure functions over plain structs. `InPriorityOrder` implements the ordering rule; `Plan` walks that order filling lines greedily. Nothing imports it yet — PR 3 wires it into checkout. Because it touches no database, its interesting cases are table tests rather than fixtures, and it is the cheapest place in this feature to get the semantics right.

**Tech Stack:** Go 1.26, testify. No GORM, no SQL, no `context`.

**Spec:** `docs/superpowers/specs/2026-08-31-multi-warehouse-allocation-design.md` (see "The allocator")

## Global Constraints

- Work in the worktree `.claude/worktrees/177-allocator`, branch `feat/177-allocator`. Never switch the main checkout's branch.
- Run every Go command from `services/marketplace-api`, never path-scoped, always `-count=1`: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`.
- **This package has no integration tests and must not gain any.** It touches no database. If a test needs `testdb`, the design has gone wrong — stop and escalate.
- **Never mutate an input.** Repo style is strict on this: functions return new values rather than modifying what they were given. `InPriorityOrder` must not reorder the caller's slice.
- Commits: conventional, single line, no signature, no `Co-Authored-By` trailer, no emoji.
- Stage with explicit paths (`git add <path>`). Never `git add -A`.
- Every guard added must be mutation-tested: delete the guard, watch a test fail, restore it.
- Exported identifiers carry doc comments explaining *why*, matching the density of neighbouring packages (see `internal/campaignbudget`).

---

## File Structure

- **Create** `services/marketplace-api/internal/allocation/allocation.go` — the package doc, the types, and `InPriorityOrder`
- **Create** `services/marketplace-api/internal/allocation/allocation_test.go` — table tests for ordering
- **Create** `services/marketplace-api/internal/allocation/plan.go` — `Plan` and its errors
- **Create** `services/marketplace-api/internal/allocation/plan_test.go` — table tests for allocation

Two files rather than one because the ordering rule and the filling algorithm change for different reasons: ordering is a merchant-facing policy, filling is arithmetic.

---

### Task 1: Types and the ordering rule

**Files:**
- Create: `services/marketplace-api/internal/allocation/allocation.go`
- Test: `services/marketplace-api/internal/allocation/allocation_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Warehouse`, `Line`, `Availability`, `Assignment`, and `InPriorityOrder([]Warehouse) []Warehouse`. Task 2's `Plan` consumes all of them. PR 3 calls `InPriorityOrder` on rows loaded from `warehouses` before calling `Plan`.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/internal/allocation/allocation_test.go`:

```go
package allocation_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/allocation"
)

func at(day int) time.Time {
	return time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)
}

// The ordering must be TOTAL: any two warehouses must compare unequal on some
// key, or two requests could allocate the same order differently and a
// merchant would see the same cart ship from different cities.
func TestInPriorityOrder(t *testing.T) {
	tests := []struct {
		name  string
		input []allocation.Warehouse
		want  []string
	}{
		{
			name: "lower priority number ships first",
			input: []allocation.Warehouse{
				{ID: "b", Priority: 2, CreatedAt: at(1)},
				{ID: "a", Priority: 1, CreatedAt: at(2)},
			},
			want: []string{"a", "b"},
		},
		{
			name: "equal priority breaks to the default warehouse",
			input: []allocation.Warehouse{
				{ID: "b", Priority: 1, IsDefault: false, CreatedAt: at(1)},
				{ID: "a", Priority: 1, IsDefault: true, CreatedAt: at(2)},
			},
			want: []string{"a", "b"},
		},
		{
			name: "equal priority and default breaks to the older row",
			input: []allocation.Warehouse{
				{ID: "b", Priority: 1, IsDefault: true, CreatedAt: at(2)},
				{ID: "a", Priority: 1, IsDefault: true, CreatedAt: at(1)},
			},
			want: []string{"a", "b"},
		},
		{
			name: "identical on every key breaks to the id, so the order is total",
			input: []allocation.Warehouse{
				{ID: "b", Priority: 1, IsDefault: true, CreatedAt: at(1)},
				{ID: "a", Priority: 1, IsDefault: true, CreatedAt: at(1)},
			},
			want: []string{"a", "b"},
		},
		{
			name:  "empty stays empty",
			input: nil,
			want:  []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := allocation.InPriorityOrder(tc.input)
			ids := make([]string, 0, len(got))
			for _, w := range got {
				ids = append(ids, w.ID)
			}
			require.Equal(t, tc.want, ids)
		})
	}
}

// Repo style forbids mutating an input. A caller that ordered a slice for the
// allocator and then reused its own slice for display would otherwise see it
// silently reordered underneath them.
func TestInPriorityOrder_DoesNotMutateItsInput(t *testing.T) {
	input := []allocation.Warehouse{
		{ID: "b", Priority: 2, CreatedAt: at(1)},
		{ID: "a", Priority: 1, CreatedAt: at(2)},
	}

	_ = allocation.InPriorityOrder(input)

	require.Equal(t, "b", input[0].ID, "the caller's slice must be untouched")
	require.Equal(t, "a", input[1].ID)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd services/marketplace-api
go test ./internal/allocation/... -count=1
```

Expected: FAIL — the package does not exist yet (`no required module provides package`).

- [ ] **Step 3: Write the implementation**

Create `services/marketplace-api/internal/allocation/allocation.go`:

```go
// Package allocation decides which warehouse ships how much of each line of
// an order (#177).
//
// It is deliberately pure: no database, no context, no clock. The inputs are
// a store's warehouses in the order they should be filled from, a snapshot of
// what is available where, and the lines to place. Everything that makes the
// decision hard to reason about — locking, transactions, the fact that the
// snapshot can be stale by the time it is used — lives in the caller.
//
// The plan this package returns is ADVISORY. stockhold.Hold's per-location
// availability check under SELECT ... FOR UPDATE remains the thing that
// actually stops two orders taking the last unit; a plan that loses that race
// simply fails, exactly as a single-warehouse order does today.
package allocation

import (
	"cmp"
	"slices"
	"time"
)

// Warehouse is one candidate origin, reduced to what the decision needs.
type Warehouse struct {
	ID        string
	Priority  int
	IsDefault bool
	CreatedAt time.Time
}

// Line is one order line to place.
type Line struct {
	VariantID string
	Quantity  int
	// SellsPastZero mirrors product_variants.inventory_policy == "continue":
	// the merchant sells this variant past zero on purpose, so availability
	// does not constrain where it ships from.
	SellsPastZero bool
}

// Availability is units on hand per variant, per warehouse: avail[variantID][warehouseID].
// A missing entry means zero — callers do not have to enumerate empty pairs.
type Availability map[string]map[string]int

// At returns the units of variantID available at warehouseID.
func (a Availability) At(variantID, warehouseID string) int {
	return a[variantID][warehouseID]
}

// Assignment is one unit of the answer: ship Quantity of VariantID from
// WarehouseID. A line split across two warehouses produces two Assignments.
type Assignment struct {
	WarehouseID string
	VariantID   string
	Quantity    int
}

// InPriorityOrder returns a NEW slice ordered the way allocation fills:
// merchant-set priority first, then the store's default warehouse, then the
// older row, then the id.
//
// The final id tiebreak exists to make the ordering TOTAL. Without it two
// warehouses created in the same second with equal priority could compare
// equal, and two identical carts could then allocate differently — a merchant
// would see the same order ship from different cities with nothing to explain
// it.
//
// The input slice is not modified.
func InPriorityOrder(warehouses []Warehouse) []Warehouse {
	ordered := slices.Clone(warehouses)
	if ordered == nil {
		ordered = []Warehouse{}
	}
	slices.SortStableFunc(ordered, func(a, b Warehouse) int {
		if c := cmp.Compare(a.Priority, b.Priority); c != 0 {
			return c
		}
		// IsDefault true sorts first, so invert the boolean comparison.
		if a.IsDefault != b.IsDefault {
			if a.IsDefault {
				return -1
			}
			return 1
		}
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return ordered
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd services/marketplace-api
go test ./internal/allocation/... -count=1 -v
```

Expected: all subtests PASS.

- [ ] **Step 5: Mutation-test the totality tiebreak**

Delete the final `return cmp.Compare(a.ID, b.ID)` line and replace it with `return 0`, then:

```bash
cd services/marketplace-api
go test ./internal/allocation/... -count=1 -run TestInPriorityOrder
```

Expected: FAIL on the "identical on every key" case. If it PASSES, the test is not pinning totality — note that `SortStableFunc` preserves input order for equal elements, so the test case must supply its inputs in the order that a working tiebreak would REVERSE (`b` before `a`, as written above). Restore the line and confirm green.

- [ ] **Step 6: Verify and commit**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./... && gofmt -l .
go test ./... -count=1
```

```bash
git add services/marketplace-api/internal/allocation/allocation.go \
        services/marketplace-api/internal/allocation/allocation_test.go
git commit -m "feat(allocation): warehouse ordering rule and the allocator's value types (#177)"
```

---

### Task 2: The filling algorithm

**Files:**
- Create: `services/marketplace-api/internal/allocation/plan.go`
- Test: `services/marketplace-api/internal/allocation/plan_test.go`

**Interfaces:**
- Consumes: `Warehouse`, `Line`, `Availability`, `Assignment` from Task 1.
- Produces: `Plan(warehouses []Warehouse, avail Availability, lines []Line) ([]Assignment, error)`, `CannotFillError{VariantID string, Short int}`, and `ErrNoWarehouse`. PR 3 calls `Plan` inside the order transaction and writes one `order_allocations` row per returned `Assignment`.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/internal/allocation/plan_test.go`:

```go
package allocation_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/allocation"
)

func wh(id string, priority int) allocation.Warehouse {
	return allocation.Warehouse{ID: id, Priority: priority, CreatedAt: at(1)}
}

func TestPlan(t *testing.T) {
	tests := []struct {
		name       string
		warehouses []allocation.Warehouse
		avail      allocation.Availability
		lines      []allocation.Line
		want       []allocation.Assignment
	}{
		{
			name:       "one warehouse fills the line",
			warehouses: []allocation.Warehouse{wh("a", 1)},
			avail:      allocation.Availability{"v1": {"a": 5}},
			lines:      []allocation.Line{{VariantID: "v1", Quantity: 3}},
			want: []allocation.Assignment{
				{WarehouseID: "a", VariantID: "v1", Quantity: 3},
			},
		},
		{
			name:       "the higher-priority warehouse is filled from first",
			warehouses: []allocation.Warehouse{wh("a", 1), wh("b", 2)},
			avail:      allocation.Availability{"v1": {"a": 5, "b": 5}},
			lines:      []allocation.Line{{VariantID: "v1", Quantity: 3}},
			want: []allocation.Assignment{
				{WarehouseID: "a", VariantID: "v1", Quantity: 3},
			},
		},
		{
			name:       "a line no single warehouse can fill is split",
			warehouses: []allocation.Warehouse{wh("a", 1), wh("b", 2)},
			avail:      allocation.Availability{"v1": {"a": 3, "b": 4}},
			lines:      []allocation.Line{{VariantID: "v1", Quantity: 5}},
			want: []allocation.Assignment{
				{WarehouseID: "a", VariantID: "v1", Quantity: 3},
				{WarehouseID: "b", VariantID: "v1", Quantity: 2},
			},
		},
		{
			name:       "a warehouse holding none of a variant is skipped, not zero-assigned",
			warehouses: []allocation.Warehouse{wh("a", 1), wh("b", 2)},
			avail:      allocation.Availability{"v1": {"b": 4}},
			lines:      []allocation.Line{{VariantID: "v1", Quantity: 4}},
			want: []allocation.Assignment{
				{WarehouseID: "b", VariantID: "v1", Quantity: 4},
			},
		},
		{
			name:       "several lines each allocate independently",
			warehouses: []allocation.Warehouse{wh("a", 1), wh("b", 2)},
			avail: allocation.Availability{
				"v1": {"a": 1, "b": 5},
				"v2": {"a": 9},
			},
			lines: []allocation.Line{
				{VariantID: "v1", Quantity: 3},
				{VariantID: "v2", Quantity: 2},
			},
			want: []allocation.Assignment{
				{WarehouseID: "a", VariantID: "v1", Quantity: 1},
				{WarehouseID: "a", VariantID: "v2", Quantity: 2},
				{WarehouseID: "b", VariantID: "v1", Quantity: 2},
			},
		},
		{
			name:       "a sells-past-zero line goes whole to the first warehouse regardless of stock",
			warehouses: []allocation.Warehouse{wh("a", 1), wh("b", 2)},
			avail:      allocation.Availability{"v1": {"b": 100}},
			lines:      []allocation.Line{{VariantID: "v1", Quantity: 7, SellsPastZero: true}},
			want: []allocation.Assignment{
				{WarehouseID: "a", VariantID: "v1", Quantity: 7},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := allocation.Plan(tc.warehouses, tc.avail, tc.lines)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// The allocator's central invariant: what it assigns for a line must add up to
// exactly what the line asked for. Under-assigning ships a short parcel;
// over-assigning takes stock the customer did not buy.
func TestPlan_AssignmentsSumToTheLineQuantity(t *testing.T) {
	warehouses := []allocation.Warehouse{wh("a", 1), wh("b", 2), wh("c", 3)}
	avail := allocation.Availability{"v1": {"a": 2, "b": 2, "c": 2}}

	got, err := allocation.Plan(warehouses, avail, []allocation.Line{{VariantID: "v1", Quantity: 5}})
	require.NoError(t, err)

	total := 0
	for _, a := range got {
		total += a.Quantity
	}
	require.Equal(t, 5, total)
}

func TestPlan_ReturnsCannotFillWhenNoCombinationSatisfiesTheLine(t *testing.T) {
	warehouses := []allocation.Warehouse{wh("a", 1), wh("b", 2)}
	avail := allocation.Availability{"v1": {"a": 2, "b": 1}}

	_, err := allocation.Plan(warehouses, avail, []allocation.Line{{VariantID: "v1", Quantity: 6}})

	var cannot allocation.CannotFillError
	require.True(t, errors.As(err, &cannot), "got %v", err)
	require.Equal(t, "v1", cannot.VariantID)
	require.Equal(t, 3, cannot.Short, "6 wanted, 3 available, so 3 short")
}

func TestPlan_RefusesAStoreWithNoWarehouses(t *testing.T) {
	_, err := allocation.Plan(nil, allocation.Availability{}, []allocation.Line{{VariantID: "v1", Quantity: 1}})
	require.ErrorIs(t, err, allocation.ErrNoWarehouse)
}

func TestPlan_RefusesANonPositiveLineQuantity(t *testing.T) {
	warehouses := []allocation.Warehouse{wh("a", 1)}
	avail := allocation.Availability{"v1": {"a": 5}}

	_, err := allocation.Plan(warehouses, avail, []allocation.Line{{VariantID: "v1", Quantity: 0}})
	require.Error(t, err, "a line for zero units is a caller bug, not an empty allocation")
}

func TestPlan_NoLinesIsNoAssignmentsNotAnError(t *testing.T) {
	got, err := allocation.Plan([]allocation.Warehouse{wh("a", 1)}, allocation.Availability{}, nil)
	require.NoError(t, err)
	require.Empty(t, got)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd services/marketplace-api
go test ./internal/allocation/... -count=1
```

Expected: FAIL to compile — `undefined: allocation.Plan`, `undefined: allocation.CannotFillError`, `undefined: allocation.ErrNoWarehouse`.

- [ ] **Step 3: Write the implementation**

Create `services/marketplace-api/internal/allocation/plan.go`:

```go
package allocation

import (
	"errors"
	"fmt"
)

// ErrNoWarehouse means the store has nowhere to ship from. Allocation cannot
// answer that with a plan, and a caller must not treat it as "out of stock":
// the merchant has a configuration problem, not an inventory one.
var ErrNoWarehouse = errors.New("allocation: store has no warehouses")

// CannotFillError names the first line no combination of warehouses can
// satisfy, and by how much it falls short.
//
// It carries the variant because a bare failure is unactionable on a cart of
// six items — the storefront needs to know which line to mark, exactly as
// storefront's outOfStockError does.
type CannotFillError struct {
	VariantID string
	Short     int
}

func (e CannotFillError) Error() string {
	return fmt.Sprintf("allocation: variant %s is %d short across all warehouses", e.VariantID, e.Short)
}

// Plan decides which warehouse ships how much of each line.
//
// warehouses must already be in fill order — pass them through
// InPriorityOrder. Taking an ordered slice rather than sorting internally is
// deliberate: a later ordering rule (nearest-pincode, say) becomes a
// different function feeding this one, not a change to this one.
//
// The algorithm walks warehouses in order and takes min(remaining, available)
// for every unsatisfied line at each. That fills from the merchant's first
// warehouse down, which is the sentence you can say to a merchant. It does
// NOT minimise the number of parcels: a true minimum is a bin-packing problem,
// and one fewer parcel in a rare case is worth less than an explainable rule.
//
// Assignments come back grouped by warehouse in fill order, and by line order
// within a warehouse, so the same inputs always produce the same slice.
func Plan(warehouses []Warehouse, avail Availability, lines []Line) ([]Assignment, error) {
	if len(lines) == 0 {
		return []Assignment{}, nil
	}
	if len(warehouses) == 0 {
		return nil, ErrNoWarehouse
	}

	remaining := make([]int, len(lines))
	for i, line := range lines {
		if line.Quantity <= 0 {
			return nil, fmt.Errorf("allocation: line %d (variant %s): quantity must be positive, got %d",
				i, line.VariantID, line.Quantity)
		}
		remaining[i] = line.Quantity
	}

	assignments := make([]Assignment, 0, len(lines))

	for w, warehouse := range warehouses {
		for i, line := range lines {
			if remaining[i] == 0 {
				continue
			}

			take := 0
			switch {
			case line.SellsPastZero:
				// The merchant sells this past zero on purpose, so stock does
				// not constrain it. Assign it whole to the first warehouse —
				// leaving it unassigned because no location has units would
				// produce an order line that ships from nothing.
				if w != 0 {
					continue
				}
				take = remaining[i]
			default:
				take = min(remaining[i], avail.At(line.VariantID, warehouse.ID))
			}

			if take == 0 {
				continue
			}
			assignments = append(assignments, Assignment{
				WarehouseID: warehouse.ID,
				VariantID:   line.VariantID,
				Quantity:    take,
			})
			remaining[i] -= take
		}
	}

	for i, left := range remaining {
		if left > 0 {
			return nil, CannotFillError{VariantID: lines[i].VariantID, Short: left}
		}
	}

	return assignments, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd services/marketplace-api
go test ./internal/allocation/... -count=1 -v
```

Expected: every test PASSES.

Note on the expected ordering in the "several lines" case: assignments are emitted warehouse-outer, line-inner, so warehouse `a` contributes its `v1` and `v2` rows before warehouse `b` contributes its `v1` row. If your implementation emits line-outer instead, that test fails — and the fix is the implementation, not the test, because grouping by warehouse is what PR 4 needs to create one shipment per warehouse.

- [ ] **Step 5: Mutation-test the shortfall check**

Delete the final `for i, left := range remaining` loop that returns `CannotFillError`, then:

```bash
cd services/marketplace-api
go test ./internal/allocation/... -count=1
```

Expected: FAIL — `TestPlan_ReturnsCannotFillWhenNoCombinationSatisfiesTheLine`. Without that loop the function silently returns a SHORT plan, which downstream would become an order that ships fewer units than the customer paid for. Restore it and confirm green.

- [ ] **Step 6: Mutation-test the sells-past-zero branch**

Change `if w != 0 { continue }` to `if w != 0 { break }`, then re-run. Expected: still green — which is fine, `break` is equivalent here. Now instead delete the whole `case line.SellsPastZero:` arm so those lines fall through to the availability path, and re-run.

Expected: FAIL — "a sells-past-zero line goes whole to the first warehouse regardless of stock" fails, because the variant has no units at warehouse `a` and the line would go unassigned and then error as unfillable. Restore and confirm green.

- [ ] **Step 7: Verify and commit**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./... && gofmt -l .
go test ./... -count=1
```

```bash
git add services/marketplace-api/internal/allocation/plan.go \
        services/marketplace-api/internal/allocation/plan_test.go
git commit -m "feat(allocation): fill lines across warehouses in priority order (#177)"
```

---

## Done when

- `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `gofmt -l .` clean
- `go test ./... -count=1` green
- `go test ./internal/allocation/... -count=1` green with no database available at all — the package must not need one
- Three guards mutation-tested: the totality tiebreak, the shortfall check, and the sells-past-zero branch

## Explicitly NOT in this PR

- Any caller. `internal/allocation` is imported by nothing when this merges; PR 3 wires it into `commitStock` and `cart_holds.go`.
- Reading warehouses or availability from the database — that is PR 3's job, and it is why `Plan` takes a snapshot rather than a `*gorm.DB`.
- Writing `order_allocations` rows (PR 3).
- Re-planning when a hold is lost to a race. The spec rules that out until the race is observed.
- Nearest-pincode or most-stock ordering. `InPriorityOrder` is one comparator; another would be a sibling function.
