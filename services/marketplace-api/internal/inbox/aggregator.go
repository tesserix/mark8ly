package inbox

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// MaxAggregateItems bounds how deep aggregate-mode pagination may go.
//
// Serving page N at limit L requires the first N*L merged items, so cost grows
// with depth. Past this bound the aggregator refuses rather than truncating: a
// short page that looks complete is worse than an error. Narrowing with
// Filter.Kind delegates to a single provider, which pages natively and is not
// capped.
const MaxAggregateItems = 500

// ErrPageTooDeep is returned when page*limit exceeds MaxAggregateItems.
var ErrPageTooDeep = errors.New("inbox: page too deep for aggregate mode; narrow with kind")

// ErrAllSourcesFailed is returned when no provider answered.
var ErrAllSourcesFailed = errors.New("inbox: every source failed")

// ErrUnknownKind is returned when Filter.Kind names no registered provider.
var ErrUnknownKind = errors.New("inbox: unknown kind")

// ErrGetNotSupported is returned when a kind exists but its provider cannot
// read back a single item.
//
// Distinct from ErrUnknownKind on purpose: "this queue does not exist" and
// "this queue exists but cannot answer that question" are different answers,
// and a console that conflates them tells an operator a real queue is gone.
var ErrGetNotSupported = errors.New("inbox: kind does not support single-item reads")

// ErrItemNotFound is returned when a kind's provider has no such item.
var ErrItemNotFound = errors.New("inbox: item not found")

// ItemGetter is the optional half of Provider: reading ONE item back by id.
//
// It is optional rather than part of Provider because acting on an item is a
// per-kind capability, and forcing every provider to implement it would mean
// four stubs returning "not supported" written as if they were real. A kind
// that cannot be acted on says so through the interface it does not implement
// (#281a).
type ItemGetter interface {
	Get(ctx context.Context, id string) (Item, error)
}

// Get resolves one item so a caller can read its declared Actions before
// executing one. Executing an action the item did not declare is what turns
// the declared list from a contract back into documentation.
func (a *Aggregator) Get(ctx context.Context, kind, id string) (Item, error) {
	for _, p := range a.providers {
		if p.Kind() != kind {
			continue
		}
		g, ok := p.(ItemGetter)
		if !ok {
			return Item{}, fmt.Errorf("%w: %q", ErrGetNotSupported, kind)
		}
		return g.Get(ctx, id)
	}
	return Item{}, fmt.Errorf("%w: %q", ErrUnknownKind, kind)
}

// Result is one page of merged work, plus the kinds that could not be reached.
type Result struct {
	Items    []Item
	Total    int64
	Degraded []string
}

// Aggregator fans out across providers, merges, sorts and paginates. It holds
// no per-kind knowledge; adding a kind is one registration.
type Aggregator struct{ providers []Provider }

func NewAggregator(providers ...Provider) *Aggregator {
	return &Aggregator{providers: providers}
}

func (a *Aggregator) List(ctx context.Context, f Filter) (Result, error) {
	page, limit := f.Page, f.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 25
	}

	// Single-kind requests delegate: that provider pages natively, so the
	// aggregate cap does not apply.
	if f.Kind != "" {
		for _, p := range a.providers {
			if p.Kind() != f.Kind {
				continue
			}
			// The delegated provider must receive the defaulted page/limit, not
			// the caller's raw filter: providers guard pagination with
			// `if f.Limit > 0`, so forwarding a zero limit returns every row.
			d := f
			d.Page, d.Limit = page, limit
			items, err := p.List(ctx, d)
			if err != nil {
				return Result{}, fmt.Errorf("inbox: %s: %w", f.Kind, err)
			}
			total, err := p.Count(ctx, d)
			if err != nil {
				return Result{}, fmt.Errorf("inbox: %s count: %w", f.Kind, err)
			}
			return Result{Items: items, Total: total}, nil
		}
		return Result{}, fmt.Errorf("%w: %q", ErrUnknownKind, f.Kind)
	}

	if page*limit > MaxAggregateItems {
		return Result{}, ErrPageTooDeep
	}

	// Fetch enough from each provider to fill the requested window after
	// merging, then merge, sort and slice.
	fanout := f
	fanout.Page = 1
	fanout.Limit = page * limit

	var (
		merged   []Item
		total    int64
		degraded []string
		ok       int
	)
	for _, p := range a.providers {
		items, err := p.List(ctx, fanout)
		if err != nil {
			degraded = append(degraded, p.Kind())
			continue
		}
		n, err := p.Count(ctx, fanout)
		if err != nil {
			degraded = append(degraded, p.Kind())
			continue
		}
		merged = append(merged, items...)
		total += n
		ok++
	}
	if ok == 0 && len(a.providers) > 0 {
		return Result{}, ErrAllSourcesFailed
	}

	sortItems(merged)

	start := (page - 1) * limit
	if start > len(merged) {
		start = len(merged)
	}
	end := start + limit
	if end > len(merged) {
		end = len(merged)
	}

	return Result{Items: merged[start:end], Total: total, Degraded: degraded}, nil
}

// sortItems orders overdue work first, then longest-waiting.
//
// Items with a due date sort ahead of items without one — a null due date is
// not "due at the epoch", it is "no deadline", and must not outrank a real
// breached SLA.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch {
		case a.DueAt != nil && b.DueAt != nil:
			if !a.DueAt.Equal(*b.DueAt) {
				return a.DueAt.Before(*b.DueAt)
			}
		case a.DueAt != nil:
			return true
		case b.DueAt != nil:
			return false
		}
		return a.WaitingSince.Before(b.WaitingSince)
	})
}
