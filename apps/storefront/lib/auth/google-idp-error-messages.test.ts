import { describe, expect, it } from "vitest";

import { googleIdpErrorMessage } from "./google-idp-error-messages";

describe("googleIdpErrorMessage", () => {
  it("returns null when there is no code", () => {
    expect(googleIdpErrorMessage(null)).toBeNull();
    expect(googleIdpErrorMessage(undefined)).toBeNull();
  });

  it("email_taken and email_not_verified get distinct, non-generic messages", () => {
    const taken = googleIdpErrorMessage("email_taken");
    const notVerified = googleIdpErrorMessage("email_not_verified");

    expect(taken).not.toBeNull();
    expect(notVerified).not.toBeNull();
    expect(taken).not.toBe(notVerified);
  });

  it("email_taken's message says the account can't be fixed by retrying and points to support, never implying a credential is wrong", () => {
    const message = googleIdpErrorMessage("email_taken")!;

    expect(message.toLowerCase()).toContain("support");
    expect(message.toLowerCase()).not.toContain("password");
    expect(message.toLowerCase()).not.toContain("incorrect");
  });

  it("none of the known messages mention auth-bff or Zitadel internals", () => {
    const codes = [
      "email_not_verified",
      "email_taken",
      "email_ambiguous",
      "unexpected_idp",
      "invalid_intent",
      "invalid_return_url",
      "zitadel_unavailable",
      "store_not_found",
      "invalid_host",
      "invalid_request",
    ];
    for (const code of codes) {
      const message = googleIdpErrorMessage(code)!;
      expect(message.toLowerCase()).not.toContain("auth-bff");
      expect(message.toLowerCase()).not.toContain("intentid");
      expect(message).not.toContain("Zitadel");
    }
  });

  it("falls back to a generic (but still honest) message for an unknown code, never echoing the raw code", () => {
    const message = googleIdpErrorMessage("some_internal_code_nobody_wrote_copy_for");

    expect(message).not.toContain("some_internal_code_nobody_wrote_copy_for");
    expect(message).toBe(
      "Something went wrong signing in with Google. Please try again.",
    );
  });
});
