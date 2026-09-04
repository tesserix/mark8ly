import { describe, expect, it } from "vitest";
import {
  buildAdminGoogleReturnUrl,
  isAdminGoogleErrorCode,
  messageForAdminGoogleError,
} from "./google-sign-in-admin";

const KNOWN_CODES = [
  "google_sign_in_unavailable",
  "invalid_request",
  "no_admin_account",
  "unexpected_idp",
  "email_not_verified",
  "email_ambiguous",
  "invalid_intent",
  "zitadel_unavailable",
  "step_up_unsupported",
  "internal_error",
];

describe("buildAdminGoogleReturnUrl", () => {
  it("builds an https URL on the canonical admin host at /auth/idp/finish", () => {
    const url = buildAdminGoogleReturnUrl("admin.mark8ly.com", "V2_abc");
    expect(url).toBe("https://admin.mark8ly.com/auth/idp/finish?auth_request_id=V2_abc");
  });

  it("reflects back whatever host it is given, since the caller's request host is always correct", () => {
    const url = buildAdminGoogleReturnUrl("some-other-admin-host.mark8ly.com", "V2_abc");
    expect(url).toBe("https://some-other-admin-host.mark8ly.com/auth/idp/finish?auth_request_id=V2_abc");
  });

  it("percent-encodes an auth_request_id with special characters", () => {
    const url = buildAdminGoogleReturnUrl("admin.mark8ly.com", "abc def&x=1");
    const parsed = new URL(url);
    expect(parsed.searchParams.get("auth_request_id")).toBe("abc def&x=1");
  });
});

describe("isAdminGoogleErrorCode", () => {
  it("recognises every documented code", () => {
    for (const code of KNOWN_CODES) {
      expect(isAdminGoogleErrorCode(code)).toBe(true);
    }
  });

  it("rejects an unrecognised code", () => {
    expect(isAdminGoogleErrorCode("something_else")).toBe(false);
  });

  it("no longer recognises store_not_found or invalid_return_url — neither is reachable from this flow", () => {
    // store_not_found was the host-derived-tenant failure from the
    // earlier, broken version of this flow; invalid_return_url is
    // idpStart's own rejection, handled entirely inside
    // startAdminGoogleSignIn before the browser ever leaves this page.
    // Neither code can reach messageForAdminGoogleError any more, so
    // neither should still be "known" — keeping them recognised would
    // let their (now-deleted) copy silently come back the moment
    // someone re-adds either string anywhere.
    expect(isAdminGoogleErrorCode("store_not_found")).toBe(false);
    expect(isAdminGoogleErrorCode("invalid_return_url")).toBe(false);
  });
});

describe("messageForAdminGoogleError", () => {
  it("gives every documented code a non-empty, distinct message", () => {
    const messages = KNOWN_CODES.map(messageForAdminGoogleError);
    // google_sign_in_unavailable/zitadel_unavailable share copy — both
    // are genuinely the same situation from the merchant's point of view.
    expect(new Set(messages).size).toBeGreaterThanOrEqual(KNOWN_CODES.length - 1);
    for (const m of messages) {
      expect(m.length).toBeGreaterThan(0);
    }
  });

  it("no_admin_account never implies the sign-in itself failed or suggests retrying the same way", () => {
    const message = messageForAdminGoogleError("no_admin_account");
    expect(message.toLowerCase()).toContain("no admin account");
    expect(message.toLowerCase()).not.toMatch(/try again|retry/);
    expect(message.toLowerCase()).not.toMatch(/wrong|incorrect|invalid password|invalid credential/);
  });

  it("falls back to a generic message for an unrecognised code, never echoing it", () => {
    const message = messageForAdminGoogleError("some_internal_stack_trace_detail");
    expect(message).not.toContain("some_internal_stack_trace_detail");
    expect(message.length).toBeGreaterThan(0);
  });

  it("falls back to the generic message for store_not_found and invalid_return_url — neither is a known code any more", () => {
    for (const dead of ["store_not_found", "invalid_return_url"]) {
      const message = messageForAdminGoogleError(dead);
      expect(message).toBe(messageForAdminGoogleError("some_unrecognised_code"));
      // The whole point: this must never again tell a merchant to sign
      // in from their store's own admin address.
      expect(message.toLowerCase()).not.toContain("store's own admin address");
    }
  });

  it("never implies wrong credentials for any known code", () => {
    for (const code of KNOWN_CODES) {
      const message = messageForAdminGoogleError(code).toLowerCase();
      expect(message).not.toMatch(/wrong password|incorrect password|bad credentials/);
    }
  });
});
