import { test, expect } from "@playwright/test";

// Issue #685 — the browser-side copy of Zitadel's password policy.
//
// The rule this file protects: a client rule LOOSER than the server's is
// worse than no client rule at all, because it promises an acceptance the
// server will refuse. Onboarding's set-password form said min(8) while
// the live org policy requires 12 plus four character classes, so a
// merchant could submit happily and then be told, opaquely, that their
// store could not be created.

import {
  PASSWORD_MIN_LENGTH,
  PASSWORD_REQUIREMENTS_TEXT,
  validateNewPassword,
} from "../../lib/auth/password-policy";

test("the minimum matches the live org policy, not GIP's old floor", () => {
  expect(PASSWORD_MIN_LENGTH).toBe(12);
});

test("the requirements text names every rule before the first submit", () => {
  for (const rule of ["12", "uppercase", "lowercase", "number", "symbol"]) {
    expect(PASSWORD_REQUIREMENTS_TEXT.toLowerCase()).toContain(rule);
  }
});

test("a compliant password is accepted", () => {
  // Obviously fake, and still satisfies all five rules.
  expect(validateNewPassword("Not-A-Real-Password-1!")).toBeNull();
});

test("each rule is enforced, and the message restates the whole policy", () => {
  const rejected: Record<string, string> = {
    // Eleven characters — the exact shape of the reported incident.
    too_short: "Sh0rt-Pass!",
    no_uppercase: "not-a-real-password-1!",
    no_lowercase: "NOT-A-REAL-PASSWORD-1!",
    no_number: "Not-A-Real-Password!!",
    no_symbol: "NotARealPassword1234",
  };

  for (const [rule, password] of Object.entries(rejected)) {
    const message = validateNewPassword(password);
    expect(message, `${rule} was accepted`).not.toBeNull();
    // Fixing one rule commonly reveals the next, so every rejection
    // restates the full policy rather than drip-feeding.
    expect(message).toContain("12 characters");
    expect(message).toContain("symbol");
  }
});
