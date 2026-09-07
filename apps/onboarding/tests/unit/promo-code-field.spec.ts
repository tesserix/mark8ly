import { expect, test } from "@playwright/test";

import { promoMessage } from "../../lib/promo/message";
import {
  allowPromoCheck,
  resetPromoRateLimit,
  PROMO_MAX_ATTEMPTS,
  PROMO_WINDOW_MS,
} from "../../lib/promo/rateLimit";

/**
 * The onboarding promo field (#620).
 *
 * These cover the two pieces with a merchant-visible consequence that no
 * rendering test would catch: which sentence each server answer produces, and
 * the per-visitor cap on a surface that is, by nature, a guessing target.
 */

test.describe("promoMessage", () => {
  test("states the offer in the code's own numbers", () => {
    const msg = promoMessage({
      valid: true,
      trial_extension_days: 30,
      reject_reason: "",
    });
    expect(msg?.tone).toBe("accepted");
    expect(msg?.text).toContain("30 days");
  });

  test("says a day, not 1 days", () => {
    const msg = promoMessage({
      valid: true,
      trial_extension_days: 1,
      reject_reason: "",
    });
    expect(msg?.text).toContain("a day");
    expect(msg?.text).not.toContain("1 days");
  });

  test("claims no benefit when the response describes none", () => {
    const msg = promoMessage({
      valid: true,
      trial_extension_days: 0,
      reject_reason: "",
    });
    expect(msg?.tone).toBe("accepted");
    expect(msg?.text).not.toMatch(/\d/);
  });

  // The failure that costs a signup: a merchant holding a code that works is
  // told it does not, and goes away instead of finishing in Billing.
  test("redeem_in_billing never says the code is invalid", () => {
    const msg = promoMessage({
      valid: false,
      trial_extension_days: 0,
      reject_reason: "redeem_in_billing",
    });
    expect(msg?.text.toLowerCase()).not.toContain("recognise");
    expect(msg?.text.toLowerCase()).toContain("billing");
  });

  test("renders a distinct sentence for every refusal it knows", () => {
    const reasons = [
      "invalid_or_expired",
      "redeem_in_billing",
      "max_redemptions_reached",
      "max_per_email_reached",
    ];
    const seen = new Set<string>();
    for (const reject_reason of reasons) {
      const msg = promoMessage({
        valid: false,
        trial_extension_days: 0,
        reject_reason,
      });
      expect(msg, reject_reason).not.toBeNull();
      expect(seen.has(msg!.text), `${reject_reason} repeats another sentence`).toBe(false);
      seen.add(msg!.text);
    }
  });

  // A reason this build has never heard of must fall back to the safest
  // sentence rather than crash or say nothing.
  test("an unrecognised reason falls back rather than failing", () => {
    const msg = promoMessage({
      valid: false,
      trial_extension_days: 0,
      reject_reason: "some_reason_added_after_this_build",
    });
    expect(msg?.tone).toBe("refused");
    expect(msg?.text.length).toBeGreaterThan(0);
  });

  // "We could not ask" is NOT "your code is bad". Distinct tone and distinct
  // sentence, because the merchant should keep going, not retype.
  test("an unreachable check is not reported as an invalid code", () => {
    const msg = promoMessage(null);
    expect(msg?.tone).toBe("unknown");
    const invalid = promoMessage({
      valid: false,
      trial_extension_days: 0,
      reject_reason: "invalid_or_expired",
    });
    expect(msg?.text).not.toBe(invalid?.text);
  });
});

test.describe("allowPromoCheck", () => {
  test.beforeEach(() => resetPromoRateLimit());

  test("allows up to the cap, then refuses", () => {
    for (let i = 0; i < PROMO_MAX_ATTEMPTS; i++) {
      expect(allowPromoCheck("1.2.3.4"), `attempt ${i + 1}`).toBe(true);
    }
    expect(allowPromoCheck("1.2.3.4")).toBe(false);
  });

  // Buckets share one map. A keying mistake would show up only under real
  // traffic, as "promo codes stopped working for everyone".
  test("one visitor cannot exhaust another's budget", () => {
    for (let i = 0; i < PROMO_MAX_ATTEMPTS; i++) allowPromoCheck("1.2.3.4");
    expect(allowPromoCheck("5.6.7.8")).toBe(true);
  });

  test("the window expires", () => {
    const t0 = 1_000_000;
    for (let i = 0; i < PROMO_MAX_ATTEMPTS; i++) allowPromoCheck("1.2.3.4", t0);
    expect(allowPromoCheck("1.2.3.4", t0)).toBe(false);
    expect(allowPromoCheck("1.2.3.4", t0 + PROMO_WINDOW_MS + 1)).toBe(true);
  });
});
