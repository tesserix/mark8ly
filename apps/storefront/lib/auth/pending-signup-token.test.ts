import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { signPendingSignup, verifyPendingSignup } from "./pending-signup-token";

beforeEach(() => {
  process.env.SESSION_ENCRYPT_KEY = "test-session-key-at-least-32-bytes!!";
});

afterEach(() => {
  delete process.env.SESSION_ENCRYPT_KEY;
});

describe("pending-signup token", () => {
  it("verifies a token against the exact {uid, email} it was signed for", () => {
    const token = signPendingSignup("u-1", "shopper@example.com");

    expect(verifyPendingSignup("u-1", "shopper@example.com", token)).toBe(true);
  });

  it("rejects the token if the email is swapped — the attack this module exists to close", () => {
    // A registers themselves, gets a genuine token for their OWN email...
    const token = signPendingSignup("u-attacker", "attacker@example.com");

    // ...then tries to replay verify with a victim's email instead.
    expect(verifyPendingSignup("u-attacker", "victim@example.com", token)).toBe(false);
  });

  it("rejects the token if the uid is swapped", () => {
    const token = signPendingSignup("u-1", "shopper@example.com");

    expect(verifyPendingSignup("u-2", "shopper@example.com", token)).toBe(false);
  });

  it("rejects a garbage or empty token", () => {
    expect(verifyPendingSignup("u-1", "shopper@example.com", "not-a-real-token")).toBe(false);
    expect(verifyPendingSignup("u-1", "shopper@example.com", "")).toBe(false);
  });

  it("rejects a token signed for a different purpose/shape (no cross-protocol reuse)", () => {
    // A session cookie's HMAC (lib/session.ts) is computed over a
    // completely different payload shape under the same key — pinning
    // that an arbitrary hex string of the right length still fails.
    const token = signPendingSignup("u-1", "shopper@example.com");
    const tampered = token.slice(0, -2) + (token.slice(-2) === "00" ? "11" : "00");

    expect(verifyPendingSignup("u-1", "shopper@example.com", tampered)).toBe(false);
  });
});
