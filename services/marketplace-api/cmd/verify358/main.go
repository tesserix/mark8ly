// Command verify358 is an OPERATOR TOOL, not a service. It is never built
// into a container, never wired into CI, a cron, or any deployment — it
// exists solely so a human can exercise our own billingstripe client code
// (internal/billing/stripe) against a real Stripe TEST-mode subscription.
//
// WHY THIS EXISTS: #358 lets platform-admin move a card-backed trial's
// trial_end in Stripe via billingstripe.UpdateTrialEnd. Production holds
// ZERO store_subscriptions rows, so a production deploy can only prove the
// HTTP route is mounted and refuses unsigned callers — it can never
// exercise the Stripe call itself. A curl-based script (see
// scripts/verify-358-stripe.sh) can prove Stripe's REST API behaves as
// documented, but that only proves Stripe works — it does not prove OUR
// client code (internal/billing/stripe/update.go, subscription.go) calls
// it correctly. This program calls billingstripe.GetSubscription and
// billingstripe.UpdateTrialEnd directly — the exact functions
// internal/billing/trial/extend.go calls in production — so a pass here is
// evidence about the code we actually ship, not about Stripe's API.
//
// Usage:
//
//	STRIPE_TEST_KEY=sk_test_... go run ./cmd/verify358 \
//	    -sub sub_XXXX -trial-end 2026-10-25T00:00:00Z
//
// -trial-end accepts either RFC3339 or a raw Unix integer. The key is read
// from STRIPE_TEST_KEY and this program refuses anything not prefixed
// sk_test_, exactly like scripts/verify-358-stripe.sh — this program is not
// a wider grant than that script, only a more faithful one.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "verify358:", err)
		os.Exit(1)
	}
}

func run() error {
	subID := flag.String("sub", "", "Stripe subscription id (sub_...) to verify against")
	trialEndArg := flag.String("trial-end", "", "target trial_end: RFC3339 timestamp or Unix seconds")
	flag.Parse()

	if *subID == "" || *trialEndArg == "" {
		return fmt.Errorf("both -sub and -trial-end are required")
	}

	targetUnix, err := parseTrialEnd(*trialEndArg)
	if err != nil {
		return err
	}

	key := os.Getenv("STRIPE_TEST_KEY")
	if key == "" {
		return fmt.Errorf(`STRIPE_TEST_KEY is not set.

This program reads the key from the STRIPE_TEST_KEY environment variable —
it does not fetch one itself. Obtain a Stripe TEST-mode secret key (starts
with sk_test_) and export it, e.g.:

  export STRIPE_TEST_KEY=sk_test_...

Then re-run this program`)
	}
	if len(key) < 8 || key[:8] != "sk_test_" {
		return fmt.Errorf("REFUSING: STRIPE_TEST_KEY does not start with sk_test_ — this program must never touch live billing")
	}

	c := billingstripe.New(key)
	ctx := context.Background()

	before, err := billingstripe.GetSubscription(ctx, c, *subID)
	if err != nil {
		return fmt.Errorf("get subscription (before): %w", err)
	}
	beforePriceID := priceIDOf(before)

	updated, err := billingstripe.UpdateTrialEnd(ctx, c, billingstripe.UpdateTrialEndParams{
		SubscriptionID: *subID,
		TrialEnd:       targetUnix,
		IdempotencyKey: "verify358:" + *subID + ":" + strconv.FormatInt(targetUnix, 10),
	})
	if err != nil {
		return fmt.Errorf("UpdateTrialEnd: %w", err)
	}
	_ = updated // the authoritative read is the fresh GetSubscription below

	after, err := billingstripe.GetSubscription(ctx, c, *subID)
	if err != nil {
		return fmt.Errorf("get subscription (after): %w", err)
	}
	afterPriceID := priceIDOf(after)

	printReport(*subID, targetUnix, before, after, beforePriceID, afterPriceID)
	return nil
}

func parseTrialEnd(s string) (int64, error) {
	if unix, err := strconv.ParseInt(s, 10, 64); err == nil {
		return unix, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0, fmt.Errorf("-trial-end must be RFC3339 or a Unix integer, got %q: %w", s, err)
	}
	return t.Unix(), nil
}

func priceIDOf(s *billingstripe.Subscription) string {
	if s == nil || len(s.Items.Data) == 0 {
		return ""
	}
	return s.Items.Data[0].Price.ID
}

func pass(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

// printReport prints the exact integers Stripe returned, not just formatted
// dates — the acceptance criterion this tool exists to check is about an
// exact integer reaching Stripe, and a formatted date can round or truncate
// in ways that would hide a real mismatch.
func printReport(subID string, target int64, before, after *billingstripe.Subscription, beforePrice, afterPrice string) {
	fmt.Printf("subscription        = %s\n", subID)
	fmt.Printf("target trial_end    = %d\n\n", target)

	fmt.Println("field                 before          after           expected        result")
	fmt.Printf("trial_end             %-15d %-15d %-15d %s\n",
		before.TrialEnd, after.TrialEnd, target, pass(after.TrialEnd == target))
	fmt.Printf("billing_cycle_anchor  %-15d %-15d %-15d %s\n",
		before.BillingCycleAnchor, after.BillingCycleAnchor, target, pass(after.BillingCycleAnchor == target))
	fmt.Printf("status                %-15s %-15s %-15s %s\n",
		before.Status, after.Status, "trialing", pass(after.Status == "trialing"))
	fmt.Printf("price id              %-15s %-15s %-15s %s\n",
		beforePrice, afterPrice, beforePrice, pass(afterPrice == beforePrice))

	fmt.Println()
	fmt.Printf("billing_cycle_anchor moved from %d to %d\n", before.BillingCycleAnchor, after.BillingCycleAnchor)
}
