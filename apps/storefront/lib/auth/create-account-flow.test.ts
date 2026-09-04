import { describe, expect, it } from "vitest";

import {
  applyRegisterResult,
  applyVerifyResult,
  INITIAL_CREATE_ACCOUNT_STATE,
} from "./create-account-flow";

describe("applyRegisterResult", () => {
  it("a successful register moves to the verify step, carrying the trusted uid/email/token", () => {
    const state = applyRegisterResult({
      ok: true,
      uid: "u-1",
      email: "shopper@example.com",
      token: "sig-1",
    });

    expect(state).toEqual({
      step: { kind: "verify", uid: "u-1", email: "shopper@example.com", token: "sig-1" },
      error: null,
    });
  });

  it("a failed register (e.g. email_taken) stays on the form step with that outcome's message", () => {
    const state = applyRegisterResult({
      ok: false,
      code: "email_taken",
      message: "An account with this email address already exists.",
    });

    expect(state.step).toEqual({ kind: "form" });
    expect(state.error).toBe("An account with this email address already exists.");
  });
});

describe("applyVerifyResult", () => {
  const verifyStep = {
    kind: "verify" as const,
    uid: "u-1",
    email: "shopper@example.com",
    token: "sig-1",
  };

  it("a successful verify clears the error and leaves the verify step (caller redirects away)", () => {
    const state = applyVerifyResult(verifyStep, { ok: true });

    expect(state.error).toBeNull();
  });

  it("a wrong/expired code keeps the shopper on the SAME verify step, with the SAME uid/email — no progress lost", () => {
    const state = applyVerifyResult(verifyStep, {
      ok: false,
      code: "invalid_verification_code",
      message: "That code is incorrect or has expired.",
    });

    expect(state.step).toEqual(verifyStep);
    expect(state.error).toBe("That code is incorrect or has expired.");
  });

  it("a zitadel_unavailable failure also keeps the shopper on the same verify step, not back at the start", () => {
    const state = applyVerifyResult(verifyStep, {
      ok: false,
      code: "zitadel_unavailable",
      message: "Account creation is temporarily unavailable. Please try again shortly.",
    });

    expect(state.step).toEqual(verifyStep);
  });
});

describe("INITIAL_CREATE_ACCOUNT_STATE", () => {
  it("starts on the form step with no error", () => {
    expect(INITIAL_CREATE_ACCOUNT_STATE).toEqual({
      step: { kind: "form" },
      error: null,
    });
  });
});
