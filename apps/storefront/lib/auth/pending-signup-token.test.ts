import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { signPendingSignup, verifyPendingSignup } from "./pending-signup-token";
import { encodeSession } from "../session";

beforeEach(() => {
  process.env.SESSION_ENCRYPT_KEY = "test-session-key-at-least-32-bytes!!";
});

afterEach(() => {
  delete process.env.SESSION_ENCRYPT_KEY;
  vi.unstubAllEnvs();
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

  it("rejects a single flipped bit in an otherwise-valid token", () => {
    // A corruption/tamper-detection check — distinct from the
    // cross-protocol-reuse property pinned below, which this test used to
    // be (mis-)titled as.
    const token = signPendingSignup("u-1", "shopper@example.com");
    const tampered = token.slice(0, -2) + (token.slice(-2) === "00" ? "11" : "00");

    expect(verifyPendingSignup("u-1", "shopper@example.com", tampered)).toBe(false);
  });

  it("rejects a genuine session-cookie HMAC replayed as a pending-signup token, even under the same key and matching uid/email", () => {
    // Both signPendingSignup and lib/session.ts's encodeSession may be
    // keyed by the same SESSION_ENCRYPT_KEY, but they HMAC completely
    // different payload shapes (a JSON-encoded [purpose, uid, email]
    // tuple here vs. a base64 JSON session object there) — so a value
    // legitimately signed for one must never verify as the other. This is
    // a REAL cross-protocol-reuse check, not a bit-flip with that name.
    const uid = "u-1";
    const email = "shopper@example.com";
    const cookie = encodeSession({
      uid,
      email,
      store_slug: "demo-store",
      store_id: "store-1",
      tenant_id: "tenant-1",
    });
    const cookieSignature = cookie.slice(cookie.lastIndexOf(".") + 1);

    expect(verifyPendingSignup(uid, email, cookieSignature)).toBe(false);
  });

  it("rejects a non-string token without throwing", () => {
    // Server actions cross a network boundary as JSON — a malformed or
    // malicious caller can send any JSON value for `token` regardless of
    // this function's TypeScript signature, so the runtime check must not
    // assume it already received a string.
    const uid = "u-1";
    const email = "shopper@example.com";
    for (const bogus of [123, null, undefined, {}, ["a"], true]) {
      expect(() =>
        verifyPendingSignup(uid, email, bogus as unknown as string),
      ).not.toThrow();
      expect(verifyPendingSignup(uid, email, bogus as unknown as string)).toBe(false);
    }
  });

  it("throws in production if SESSION_ENCRYPT_KEY is unset — never silently falls back to the dev key", () => {
    vi.stubEnv("NODE_ENV", "production");
    delete process.env.SESSION_ENCRYPT_KEY;

    expect(() => signPendingSignup("u-1", "shopper@example.com")).toThrow(
      "SESSION_ENCRYPT_KEY is required in production",
    );
  });
});
