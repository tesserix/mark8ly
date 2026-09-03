import { describe, expect, it } from "vitest";
import {
  isTotpRequiredResult,
  type CustomerSignInResult,
} from "./customer-sign-in-result";

describe("isTotpRequiredResult", () => {
  it("returns true for a totp_required result", () => {
    const result: CustomerSignInResult = {
      ok: false,
      code: "totp_required",
      message: "Enter the code from your authenticator app.",
      sessionId: "s-1",
      sessionToken: "tok-1",
    };

    expect(isTotpRequiredResult(result)).toBe(true);
  });

  it("returns false for a rejected (plain failure) result", () => {
    const result: CustomerSignInResult = {
      ok: false,
      code: "invalid_credentials",
      message: "Email or password is incorrect.",
    };

    expect(isTotpRequiredResult(result)).toBe(false);
  });

  it("returns false for an ok result", () => {
    const result: CustomerSignInResult = { ok: true };

    expect(isTotpRequiredResult(result)).toBe(false);
  });

  it("returns false for a handoff (signin_method_unsupported) result", () => {
    const result: CustomerSignInResult = {
      ok: false,
      code: "signin_method_unsupported",
      message:
        "This account uses a sign-in method this storefront can't complete yet. Please contact support for help signing in.",
    };

    expect(isTotpRequiredResult(result)).toBe(false);
  });
});
