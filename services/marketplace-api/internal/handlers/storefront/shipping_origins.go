// Package storefront — shipping_origins.go: groups a checkout/rate
// request's items by the warehouse(s) that will actually fulfill them,
// and combines a per-origin carrier quote into one answer (#541).
//
// Before this file, both the rates quote (shipping_rates.go) and the
// checkout charge (checkout_ext.go) priced from ONE fixed origin — the
// carrier config's linked warehouse — no matter where the stock backing
// the order actually sat. Since multi-warehouse shipped (#177), one
// variant's stock can span warehouses, so allocation can split an order
// into several physical parcels from several origins. Pricing that as one
// parcel from one origin under-quotes it; the merchant absorbed the
// difference.
//
// buildOriginGroups reuses the SAME allocation machinery checkout_stock.go
// uses to actually place stock at commit time (storeWarehousesInFillOrder,
// loadAvailability, allocation.Plan), so the quote and the eventual
// placement can never disagree about which warehouse ships what.
package storefront

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/allocation"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/warehouse"
)

// errMissingVariantID means a rate/checkout request carried an item with
// no variant_id (the field is deliberately not binding:"required" — an
// unstocked or custom line is otherwise legitimate). Splitting requires
// knowing which variant each line is, so a request with any such item
// falls back to the single-origin path entirely rather than guess.
var errMissingVariantID = errors.New("storefront: shipping origin split requires variant_id on every item")

// errNoWarehousesToSplit means the store has no live warehouse to
// allocate from. checkout_stock.go documents why a store with stock has
// always resolved one since #177 PR 6 — this is the defensive read for
// when that is ever false, not the expected case.
var errNoWarehousesToSplit = errors.New("storefront: store has no warehouses to split shipping origins across")

// quoteCartTokenSentinel stands in for "no cart" when reading availability
// for a quote or charge preview — see buildOriginGroups's loadAvailability
// call for why this can't just be "".
const quoteCartTokenSentinel = "00000000-0000-0000-0000-000000000000"

// originLineItem is one request line reduced to what origin-splitting
// needs: which variant, how many units, and the parcel data to report for
// whichever warehouse(s) end up shipping it. Parcel.Quantity is ignored —
// Quantity above is the source of truth, since one line's units can end
// up split across more than one resulting parcel.
type originLineItem struct {
	VariantID string
	Quantity  int
	Parcel    shipping.ParcelItem
}

// originGroup is one fulfilling origin and the parcels it ships.
type originGroup struct {
	Address shipping.Address
	Parcels []shipping.ParcelItem
}

// buildOriginGroups groups items by the warehouse(s) allocation.Plan says
// will actually fulfil them, resolving each warehouse's address.
//
// It returns an error whenever the split cannot be computed. Every caller
// treats that as "fall back to the existing single-origin quote/charge" —
// never as a reason to fail the request, or to price it wrong — matching
// #541's fallback list: a missing variant_id, no warehouses, an
// availability read failure, or allocation.Plan being unable to fill the
// cart (CannotFillError or any other error).
//
// db is read-only here (h.db, no transaction): a quote or a charge preview
// must never take the locks or side effects commitStock's hold placement
// does.
func buildOriginGroups(
	ctx context.Context,
	db *gorm.DB,
	warehouseRepo *warehouse.Repository,
	storeID string,
	items []originLineItem,
) ([]originGroup, error) {
	if len(items) == 0 {
		return nil, nil
	}
	for _, it := range items {
		if it.VariantID == "" {
			return nil, errMissingVariantID
		}
	}

	warehouses, err := storeWarehousesInFillOrder(ctx, db, storeID)
	if err != nil {
		return nil, err
	}
	if len(warehouses) == 0 {
		return nil, errNoWarehousesToSplit
	}

	variantIDs := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		vid := strings.ToLower(it.VariantID)
		if _, ok := seen[vid]; ok {
			continue
		}
		seen[vid] = struct{}{}
		variantIDs = append(variantIDs, vid)
	}

	// A quote or charge preview happens before THIS attempt has placed any
	// hold of its own, so there is nothing of the caller's own to exclude
	// — every live hold from every cart should be subtracted, which is
	// the conservative reading: a shopper who has not reserved anything
	// must not be quoted stock that competing carts are already holding.
	//
	// The obvious way to express "exclude nothing" is an empty cartToken,
	// since loadAvailability's exclusion is `sh.cart_token <> ?` and no
	// real hold can have an empty token. But stock_holds.cart_token is a
	// uuid column, and Postgres rejects "" as a uuid outright — the query
	// errors before it can exclude anything. quoteCartTokenSentinel is a
	// valid-shaped uuid instead, chosen so no real cart can ever hold it:
	// production cart tokens are always either parsed from a client
	// cookie (checked with uuid.Parse — this value would parse, but no
	// client is ever issued it) or freshly minted via uuid.NewString(),
	// which cannot collide with an all-zero id.
	avail, _, err := loadAvailability(ctx, db, quoteCartTokenSentinel, warehouses, variantIDs)
	if err != nil {
		return nil, err
	}

	allocLines := make([]allocation.Line, 0, len(items))
	for _, it := range items {
		allocLines = append(allocLines, allocation.Line{
			VariantID: strings.ToLower(it.VariantID),
			Quantity:  it.Quantity,
		})
	}

	assignments, err := allocation.Plan(warehouses, avail, allocLines)
	if err != nil {
		// Covers allocation.CannotFillError along with everything else
		// Plan can return (ErrNoWarehouse, ErrInvalidInput) — #541's
		// fallback list treats all of them the same way.
		return nil, err
	}

	return groupAssignmentsByWarehouse(ctx, db, warehouseRepo, items, assignments)
}

// groupAssignmentsByWarehouse turns Plan's flat assignment list back into
// per-warehouse parcels, resolves each warehouse's address, and returns
// the groups sorted by warehouse id for a deterministic response.
func groupAssignmentsByWarehouse(
	ctx context.Context,
	db *gorm.DB,
	warehouseRepo *warehouse.Repository,
	items []originLineItem,
	assignments []allocation.Assignment,
) ([]originGroup, error) {
	// Per-variant queue of items still owed units, in request order. An
	// assignment for a variant draws from these queues front to back —
	// the same principle recordAllocations (checkout_stock.go) uses to
	// attribute a flat assignment list back to individual lines.
	type pending struct {
		itemIdx   int
		remaining int
	}
	queues := map[string][]*pending{}
	for i, it := range items {
		vid := strings.ToLower(it.VariantID)
		queues[vid] = append(queues[vid], &pending{itemIdx: i, remaining: it.Quantity})
	}

	parcelsByWarehouse := map[string][]shipping.ParcelItem{}
	order := make([]string, 0, 2)
	for _, a := range assignments {
		if _, ok := parcelsByWarehouse[a.WarehouseID]; !ok {
			order = append(order, a.WarehouseID)
			parcelsByWarehouse[a.WarehouseID] = nil
		}
		want := a.Quantity
		for _, p := range queues[a.VariantID] {
			if want == 0 {
				break
			}
			if p.remaining == 0 {
				continue
			}
			take := min(want, p.remaining)
			p.remaining -= take
			want -= take

			parcel := items[p.itemIdx].Parcel
			parcel.Quantity = take
			parcelsByWarehouse[a.WarehouseID] = append(parcelsByWarehouse[a.WarehouseID], parcel)
		}
		if want > 0 {
			// Plan assigned units of a variant that this item's queue
			// does not account for — a bug in this pairing logic, not a
			// data problem (mirrors recordAllocations's identical guard
			// in checkout_stock.go). Fail loudly rather than silently
			// drop units from the quote.
			return nil, fmt.Errorf(
				"storefront: cannot attribute %d units of variant %s at warehouse %s to any line",
				want, a.VariantID, a.WarehouseID)
		}
	}

	sort.Strings(order)
	groups := make([]originGroup, 0, len(order))
	for _, whID := range order {
		wh, err := warehouseRepo.ByID(ctx, db, whID)
		if err != nil {
			return nil, fmt.Errorf("storefront: resolve origin warehouse %s: %w", whID, err)
		}
		groups = append(groups, originGroup{
			Address: shipping.Address{
				Name:        wh.Name,
				Line1:       wh.Line1,
				Line2:       wh.Line2,
				City:        wh.City,
				Region:      wh.Region,
				PostalCode:  wh.PostalCode,
				CountryCode: wh.CountryCode,
				Phone:       wh.Phone,
			},
			Parcels: parcelsByWarehouse[whID],
		})
	}
	return groups, nil
}

// quoteSplitOrigins calls carrier.GetRates once per origin group — a
// separate carrier call per fulfilling warehouse, each with that
// warehouse's own address and only the parcels it ships — then combines
// the per-group results with combineOriginRates.
func quoteSplitOrigins(
	ctx context.Context,
	carrier shipping.Carrier,
	toAddr shipping.Address,
	currencyCode string,
	groups []originGroup,
) ([]shipping.Rate, error) {
	perGroup := make([][]shipping.Rate, 0, len(groups))
	for _, g := range groups {
		rates, err := carrier.GetRates(ctx, shipping.RateRequest{
			FromAddress:  g.Address,
			ToAddress:    toAddr,
			Items:        g.Parcels,
			CurrencyCode: currencyCode,
		})
		if err != nil {
			return nil, err
		}
		perGroup = append(perGroup, rates)
	}
	return combineOriginRates(perGroup), nil
}

// combineOriginRates applies the decided combine rule (#541):
//
//   - Only (carrier, service) pairs quoted by EVERY origin group are
//     offered — the intersection. A service only one origin can provide
//     cannot ship the whole order, so including it would under-quote,
//     which is the bug being fixed.
//   - price sums across groups.
//   - estimated_days takes the MAX across groups — the order is not
//     complete until the last parcel lands.
//
// An empty intersection, or no groups at all, yields no rates rather than
// a price that silently excludes a parcel.
func combineOriginRates(perGroup [][]shipping.Rate) []shipping.Rate {
	if len(perGroup) == 0 {
		return nil
	}

	type key struct{ carrier, service string }
	groupsOffering := map[key]int{}
	price := map[key]decimal.Decimal{}
	days := map[key]int{}
	currency := map[key]string{}
	var order []key

	for _, rates := range perGroup {
		// A single group listing the same (carrier, service) twice must
		// not be double-counted into the sum or the "every group" count.
		countedThisGroup := map[key]bool{}
		for _, r := range rates {
			k := key{r.Carrier, r.Service}
			if countedThisGroup[k] {
				continue
			}
			countedThisGroup[k] = true

			if groupsOffering[k] == 0 {
				order = append(order, k)
				price[k] = r.Price
				days[k] = r.EstimatedDays
				currency[k] = r.CurrencyCode
			} else {
				price[k] = price[k].Add(r.Price)
				if r.EstimatedDays > days[k] {
					days[k] = r.EstimatedDays
				}
			}
			groupsOffering[k]++
		}
	}

	result := make([]shipping.Rate, 0, len(order))
	for _, k := range order {
		if groupsOffering[k] != len(perGroup) {
			continue // not offered by every origin group
		}
		result = append(result, shipping.Rate{
			Carrier:       k.carrier,
			Service:       k.service,
			Price:         price[k],
			CurrencyCode:  currency[k],
			EstimatedDays: days[k],
		})
	}
	return result
}

// selectShippingPrice picks the rate matching service (case-insensitively),
// falling back to the first rate when none match — mirroring the
// long-standing inline logic in (*CheckoutExtHandler).calculateShipping —
// applies the handling fee once, and zeroes the price when the
// free-shipping threshold is met. ok is false only when rates is empty,
// so the caller can fall back rather than charge nothing for an order
// that has a real cost.
func selectShippingPrice(
	rates []shipping.Rate,
	service string,
	handlingFee decimal.Decimal,
	freeShippingMin *decimal.Decimal,
	subtotal decimal.Decimal,
) (price decimal.Decimal, ok bool) {
	if len(rates) == 0 {
		return decimal.Zero, false
	}
	chosen := rates[0]
	for _, r := range rates {
		if strings.EqualFold(r.Service, service) {
			chosen = r
			break
		}
	}
	if freeShippingMin != nil && !freeShippingMin.IsZero() && subtotal.GreaterThanOrEqual(*freeShippingMin) {
		return decimal.Zero, true
	}
	return chosen.Price.Add(handlingFee), true
}
