import { describe, expect, it } from "vitest";

import { customerSignupErrorMessage } from "./customer-signup-messages";

describe("customerSignupErrorMessage", () => {
  it("email_taken and verification_email_failed get distinct, non-generic messages", () => {
    const taken = customerSignupErrorMessage("email_taken");
    const failed = customerSignupErrorMessage("verification_email_failed");

    expect(taken).not.toBe(failed);
  });

  it("email_taken's message is actionable (sign in instead / contact support) and never suggests retrying registration", () => {
    const message = customerSignupErrorMessage("email_taken");

    expect(message.toLowerCase()).toMatch(/sign in|support/);
    expect(message.toLowerCase()).not.toContain("try again");
    expect(message.toLowerCase()).not.toContain("try creating your account again");
  });

  it("verification_email_failed's message says retrying genuinely works", () => {
    const message = customerSignupErrorMessage("verification_email_failed");

    expect(message.toLowerCase()).toMatch(/try (creating your account )?again/);
  });

  it("invalid_verification_code's message does not say the password is wrong", () => {
    const message = customerSignupErrorMessage("invalid_verification_code");

    expect(message.toLowerCase()).not.toContain("password");
    expect(message.toLowerCase()).toContain("code");
  });

  it("every documented outcome code renders its own distinct message", () => {
    const codes = [
      "email_taken",
      "email_ambiguous",
      "weak_password",
      "invalid_verification_code",
      "email_not_verified",
      "verification_email_failed",
      "zitadel_unavailable",
    ];
    const messages = codes.map((code) => customerSignupErrorMessage(code));
    expect(new Set(messages).size).toBe(codes.length);
  });

  it("none of the known messages mention auth-bff or Zitadel internals", () => {
    const codes = [
      "email_taken",
      "email_ambiguous",
      "weak_password",
      "invalid_verification_code",
      "email_not_verified",
      "verification_email_failed",
      "zitadel_unavailable",
    ];
    for (const code of codes) {
      const message = customerSignupErrorMessage(code);
      expect(message.toLowerCase()).not.toContain("auth-bff");
      expect(message).not.toContain("Zitadel");
    }
  });

  it("falls back to a generic (but still honest) message for an unknown code, never echoing the raw code", () => {
    const message = customerSignupErrorMessage("some_internal_code_nobody_wrote_copy_for");

    expect(message).not.toContain("some_internal_code_nobody_wrote_copy_for");
    expect(message).toBe("Something went wrong creating your account. Please try again.");
  });
});
