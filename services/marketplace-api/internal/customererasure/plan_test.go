package customererasure

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// subjectEmail is deliberately distinctive so the "never interpolated"
// assertion below cannot pass by accident on a substring.
const subjectEmail = "erasure-subject-9f3a@example.test"

func testPlan(t *testing.T) (uuid.UUID, uuid.UUID, []Step) {
	t.Helper()
	storeID := uuid.New()
	requestID := uuid.New()
	return storeID, requestID, erasurePlan(storeID, subjectEmail, Token(requestID))
}

func TestErasurePlan_EveryStepIsWellFormed(t *testing.T) {
	_, _, steps := testPlan(t)
	require.NotEmpty(t, steps, "an empty plan would make every other assertion vacuous")

	for i, s := range steps {
		require.NotEmpty(t, s.Table, "step %d has no table", i)
		require.NotEmpty(t, s.SQL, "step %d (%s) has no SQL", i, s.Table)
		require.Contains(t, []Disposition{DispositionDelete, DispositionAnonymise}, s.Disposition,
			"step %d (%s) must declare a known disposition", i, s.Table)
		require.Equal(t, strings.Count(s.SQL, "?"), len(s.Args),
			"step %d (%s): every ? must have exactly one bound arg", i, s.Table)
	}
}

// An unscoped WHERE would erase a different merchant's customers. Every
// statement must be confined to the store, directly or through a subquery.
func TestErasurePlan_EveryStepIsScopedToTheStore(t *testing.T) {
	storeID, _, steps := testPlan(t)

	for i, s := range steps {
		require.Contains(t, s.SQL, "store_id = ?",
			"step %d (%s) is not scoped to a store", i, s.Table)
		require.Contains(t, s.Args, any(storeID),
			"step %d (%s) names store_id but does not bind the store", i, s.Table)
	}
}

// The subject's email must reach Postgres as a bound parameter and never as
// part of the statement text. This guards against a future edit reaching for
// fmt.Sprintf.
func TestErasurePlan_NeverInterpolatesTheEmail(t *testing.T) {
	_, _, steps := testPlan(t)

	sawEmailBound := false
	for i, s := range steps {
		require.NotContains(t, s.SQL, subjectEmail,
			"step %d (%s) interpolated the subject's email into the SQL string", i, s.Table)
		for _, a := range s.Args {
			if a == any(subjectEmail) {
				sawEmailBound = true
			}
		}
	}
	require.True(t, sawEmailBound, "no step bound the email at all — the plan cannot be matching the subject")
}

// customer_profiles is the identity row that groups 1 and 2 reach by
// subquery. If anything ran after it, that subquery would find nothing and
// the step would silently erase nothing.
func TestErasurePlan_DeletesCustomerProfilesLast(t *testing.T) {
	_, _, steps := testPlan(t)

	last := steps[len(steps)-1]
	require.Equal(t, "customer_profiles", last.Table)
	require.Equal(t, DispositionDelete, last.Disposition)

	for i, s := range steps[:len(steps)-1] {
		require.NotEqual(t, "customer_profiles", s.Table,
			"step %d also touches customer_profiles; it must appear exactly once, last", i)
	}
}

// order_addresses, order_events and returns all locate their rows through
// `orders WHERE customer_email = <subject>`. Anonymising orders first would
// orphan every one of them and they would erase nothing.
func TestErasurePlan_AnonymisesOrdersAfterEveryOrderScopedStep(t *testing.T) {
	_, _, steps := testPlan(t)

	ordersIdx := -1
	for i, s := range steps {
		if s.Table == "orders" {
			require.Equal(t, -1, ordersIdx, "orders must appear exactly once")
			ordersIdx = i
		}
	}
	require.NotEqual(t, -1, ordersIdx, "the plan must anonymise orders")

	for _, dependent := range []string{"order_addresses", "order_events", "returns"} {
		found := false
		for i, s := range steps {
			if s.Table != dependent {
				continue
			}
			found = true
			require.Less(t, i, ordersIdx,
				"%s reads orders.customer_email and must run before orders is anonymised", dependent)
		}
		require.True(t, found, "the plan must cover %s", dependent)
	}
}

// review_media is deleted by joining reviews on the subject's email, so it
// must run while reviews still carry that email.
func TestErasurePlan_DeletesReviewMediaBeforeAnonymisingReviews(t *testing.T) {
	_, _, steps := testPlan(t)

	mediaIdx, reviewsIdx := -1, -1
	for i, s := range steps {
		switch s.Table {
		case "review_media":
			mediaIdx = i
		case "reviews":
			reviewsIdx = i
		}
	}
	require.NotEqual(t, -1, mediaIdx, "the plan must delete review_media")
	require.NotEqual(t, -1, reviewsIdx, "the plan must anonymise reviews")
	require.Less(t, mediaIdx, reviewsIdx)
}

// country_code is required for tax reporting and is not identifying on its
// own. Erasing it would break the merchant's tax record for no privacy gain.
func TestErasurePlan_LeavesOrderAddressCountryCodeAlone(t *testing.T) {
	_, _, steps := testPlan(t)

	for _, s := range steps {
		if s.Table != "order_addresses" {
			continue
		}
		require.NotContains(t, s.SQL, "country_code",
			"order_addresses.country_code is retained for tax reporting")
		return
	}
	t.Fatal("the plan must cover order_addresses")
}

func TestToken_IsDeterministicAndCarriesNoPersonalData(t *testing.T) {
	requestID := uuid.New()

	require.Equal(t, Token(requestID), Token(requestID), "the token must be stable for one request")
	require.NotEqual(t, Token(requestID), Token(uuid.New()), "two requests must not share a token")

	tok := Token(requestID)
	require.NotContains(t, tok, subjectEmail)
	require.NotContains(t, tok, RedactedName)
	require.True(t, strings.HasSuffix(tok, "@erased.invalid"),
		".invalid is reserved by RFC 2606 and can never be routed")
}

// stepIndex is the position of the single step writing to table, or -1.
func stepIndex(t *testing.T, steps []Step, table string) int {
	t.Helper()
	idx := -1
	for i, s := range steps {
		if s.Table == table {
			require.Equal(t, -1, idx, "%s must appear exactly once", table)
			idx = i
		}
	}
	return idx
}

// TestErasurePlan_StripsTheOutboxBeforeDeletingTheCart.
//
// outbox_events has no store_id of its own; it is scoped through
// `abandoned_carts WHERE store_id = ? AND customer_email = ?`. Deleting the
// cart first would empty that subquery and the strip would silently leave the
// customer's address and recovery URL in the payload — a green suite with the
// PII still on disk (#435).
func TestErasurePlan_StripsTheOutboxBeforeDeletingTheCart(t *testing.T) {
	_, _, steps := testPlan(t)

	outboxIdx := stepIndex(t, steps, "outbox_events")
	cartIdx := stepIndex(t, steps, "abandoned_carts")
	require.NotEqual(t, -1, outboxIdx, "the plan must strip outbox_events")
	require.NotEqual(t, -1, cartIdx, "the plan must delete abandoned_carts")
	require.Less(t, outboxIdx, cartIdx,
		"outbox_events locates its rows through abandoned_carts and must run before the cart is deleted")
}

// TestErasurePlan_StripsShipmentsBeforeAnonymisingOrders — shipments is
// order-scoped like order_addresses and returns.
func TestErasurePlan_StripsShipmentsBeforeAnonymisingOrders(t *testing.T) {
	_, _, steps := testPlan(t)

	shipIdx := stepIndex(t, steps, "shipments")
	ordersIdx := stepIndex(t, steps, "orders")
	require.NotEqual(t, -1, shipIdx, "the plan must strip shipments")
	require.Less(t, shipIdx, ordersIdx,
		"shipments reads orders.customer_email and must run before orders is anonymised")
}

// TestErasurePlan_StripsJSONBKeysByNameRatherThanRewritingTheBlob.
//
// A `SET metadata = '{}'` would erase a governance record's structure along
// with the address; a `jsonb_set` would rewrite a value the subject never
// owned. The `-` operator removes exactly the named key and leaves everything
// else byte-identical, which is what makes stripping a shared audit row safe.
func TestErasurePlan_StripsJSONBKeysByNameRatherThanRewritingTheBlob(t *testing.T) {
	_, _, steps := testPlan(t)

	wantKeys := map[string][]string{
		"audit_logs": {
			"customer_email", "email", "recipient_email",
			"submitter_email", "author_email", "actor_email",
		},
		"outbox_events": {"customer_email", "recovery_url"},
	}

	for table, keys := range wantKeys {
		idx := stepIndex(t, steps, table)
		require.NotEqual(t, -1, idx, "the plan must cover %s", table)
		sql := steps[idx].SQL
		for _, k := range keys {
			require.Contains(t, sql, "- '"+k+"'",
				"%s must strip the %q key by name", table, k)
		}
		require.NotContains(t, sql, "jsonb_set",
			"%s must remove keys, not rewrite values", table)
	}
}

// TestErasurePlan_EmptiesShipmentAddressesRatherThanNullingThem.
// shipments.ship_to and ship_from are NOT NULL; a NULL would raise, and the
// erasure is all-or-nothing, so it would take the whole request down.
func TestErasurePlan_EmptiesShipmentAddressesRatherThanNullingThem(t *testing.T) {
	_, _, steps := testPlan(t)

	idx := stepIndex(t, steps, "shipments")
	require.NotEqual(t, -1, idx)
	sql := steps[idx].SQL
	require.Contains(t, sql, "ship_to = '{}'::jsonb")
	require.Contains(t, sql, "ship_from = '{}'::jsonb")
	require.NotContains(t, sql, "ship_to = NULL")
	require.NotContains(t, sql, "ship_from = NULL")
}

// TestErasurePlan_NeverUsesTheInfixJSONBExistsOperator. GORM consumes `?` as
// a bind placeholder, so `metadata ? 'k'` misbinds every later argument —
// the same trap #369 documented in pruneOperatorFreeText. Every `?` in a step
// must be a placeholder, which TestErasurePlan_EveryStepIsWellFormed counts;
// this asserts the operator forms that would break that count are absent.
func TestErasurePlan_NeverUsesTheInfixJSONBExistsOperator(t *testing.T) {
	_, _, steps := testPlan(t)

	for i, s := range steps {
		for _, forbidden := range []string{"? '", "?| ", "?& ", "?|", "?&"} {
			require.NotContains(t, s.SQL, forbidden,
				"step %d (%s) uses a jsonb `?` operator; GORM would bind it as a placeholder — use jsonb_exists / ->> instead",
				i, s.Table)
		}
	}
}
