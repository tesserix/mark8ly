package storefront

import (
	"strings"
	"testing"
)

// providersOf renders the ordered provider names for a readable assertion.
func providersOf(methods []paymentMethodResponse) string {
	names := make([]string, 0, len(methods))
	for _, m := range methods {
		names = append(names, m.Provider)
	}
	return strings.Join(names, ",")
}

func methodList(providers ...string) []paymentMethodResponse {
	out := make([]paymentMethodResponse, 0, len(providers))
	for _, p := range providers {
		out = append(out, paymentMethodResponse{Provider: p})
	}
	return out
}

// TestSortByPreference_FirstAllowlistEntryWins is the guard on which gateway the
// storefront pre-selects. The checkout page selects methods[0], so this
// ordering — not the DB's row order — is what makes Cashfree the default in
// India. A regression here silently sends INR checkouts to the wrong gateway.
func TestSortByPreference_FirstAllowlistEntryWins(t *testing.T) {
	cases := []struct {
		name       string
		preference []string
		methods    []paymentMethodResponse
		want       string
	}{
		{
			name:       "india post-000099 — cashfree preferred",
			preference: []string{"cashfree", "razorpay", "paypal"},
			methods:    methodList("paypal", "razorpay", "cashfree"),
			want:       "cashfree,razorpay,paypal",
		},
		{
			name:       "already in order stays put",
			preference: []string{"cashfree", "razorpay", "paypal"},
			methods:    methodList("cashfree", "razorpay", "paypal"),
			want:       "cashfree,razorpay,paypal",
		},
		{
			name:       "only some providers configured",
			preference: []string{"cashfree", "razorpay", "paypal"},
			methods:    methodList("paypal", "cashfree"),
			want:       "cashfree,paypal",
		},
		{
			name:       "a store with no cashfree config still prefers razorpay over paypal",
			preference: []string{"cashfree", "razorpay", "paypal"},
			methods:    methodList("paypal", "razorpay"),
			want:       "razorpay,paypal",
		},
		{
			name:       "unrelated country order is honoured, not hardcoded",
			preference: []string{"stripe", "paypal"},
			methods:    methodList("paypal", "stripe"),
			want:       "stripe,paypal",
		},
		{
			name:       "case differences in the seed do not break ranking",
			preference: []string{"Cashfree", "RAZORPAY"},
			methods:    methodList("razorpay", "cashfree"),
			want:       "cashfree,razorpay",
		},
		{
			name:       "single method is unchanged",
			preference: []string{"cashfree", "razorpay"},
			methods:    methodList("razorpay"),
			want:       "razorpay",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sortByPreference(tc.methods, tc.preference)
			if got := providersOf(tc.methods); got != tc.want {
				t.Fatalf("order = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSortByPreference_UnknownProviderSortsLast: a provider missing from the
// allowlist must never end up at position 0, because position 0 is what the
// storefront pre-selects. Sorting it last keeps an unexpected row visible
// without letting it become the default payment method.
func TestSortByPreference_UnknownProviderSortsLast(t *testing.T) {
	methods := methodList("mystery", "cashfree", "razorpay")
	sortByPreference(methods, []string{"cashfree", "razorpay"})
	if got := providersOf(methods); got != "cashfree,razorpay,mystery" {
		t.Fatalf("order = %q, want cashfree,razorpay,mystery", got)
	}
}

// TestSortByPreference_EmptyPreferenceIsStable guards the degenerate case: with
// no allowlist to rank against, the input order is preserved rather than
// shuffled into something arbitrary.
func TestSortByPreference_EmptyPreferenceIsStable(t *testing.T) {
	methods := methodList("razorpay", "cashfree", "paypal")
	sortByPreference(methods, nil)
	if got := providersOf(methods); got != "razorpay,cashfree,paypal" {
		t.Fatalf("order = %q, want the input order preserved", got)
	}
}

// TestSortByPreference_EmptyListDoesNotPanic — a store with no configured
// gateway reaches this path on every checkout page load.
func TestSortByPreference_EmptyListDoesNotPanic(t *testing.T) {
	sortByPreference(nil, []string{"cashfree"})
	sortByPreference([]paymentMethodResponse{}, []string{"cashfree"})
}
