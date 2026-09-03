import { describe, it, expect } from "vitest";
import { parseLoginResponse } from "./login-response";

describe("parseLoginResponse", () => {
  it("reads the nested auto-login success envelope", () => {
    expect(parseLoginResponse({ data: { uid: "u1", email: "a@b.test", tenant_id: "t1" } }))
      .toEqual({ kind: "complete", uid: "u1", email: "a@b.test", tenantId: "t1" });
  });

  it("reads mfa_required from INSIDE data, not the top level", () => {
    // Regression guard: #493/#502 were caused by reading the wrong level.
    expect(parseLoginResponse({ data: { uid: "u1", email: "a@b.test", tenant_id: "t1", mfa_required: true } }))
      .toEqual({ kind: "mfa_required" });
  });

  it("reads email_otp_required from INSIDE data", () => {
    expect(parseLoginResponse({ data: { uid: "u1", email: "a@b.test", tenant_id: "t1", email_otp_required: true } }))
      .toEqual({ kind: "email_otp_required" });
  });

  it("reads Zitadel's totp_required from the TOP level, not from data", () => {
    // Zitadel's own TOTP and auth-bff's usermfa gate are different mechanisms
    // returned at different nesting levels. Both must be handled.
    expect(parseLoginResponse({ totp_required: true, session_id: "s1", session_token: "tok" }))
      .toEqual({ kind: "totp_required", sessionId: "s1", sessionToken: "tok" });
  });

  it("reads a handoff", () => {
    expect(parseLoginResponse({ handoff_url: "https://auth.tesserix.app/ui/v2/login", auth_request_id: "V2_1" }))
      .toEqual({ kind: "handoff", handoffUrl: "https://auth.tesserix.app/ui/v2/login" });
  });

  it("carries callback_url through on a completed Zitadel login", () => {
    const out = parseLoginResponse({
      callback_url: "https://admin.mark8ly.com/auth/callback?code=c&state=s",
      data: { uid: "u1", email: "a@b.test", tenant_id: "t1" },
    });
    expect(out).toEqual({
      kind: "complete", uid: "u1", email: "a@b.test", tenantId: "t1",
      callbackUrl: "https://admin.mark8ly.com/auth/callback?code=c&state=s",
    });
  });

  it("prefers a step-up over completion when both are somehow present", () => {
    // Fail closed: never treat a login as done while a factor is outstanding.
    const out = parseLoginResponse({
      callback_url: "https://admin.mark8ly.com/auth/callback?code=c",
      data: { uid: "u1", email: "a@b.test", tenant_id: "t1", mfa_required: true },
    });
    expect(out.kind).toBe("mfa_required");
  });

  it("throws on an unrecognisable body rather than guessing", () => {
    expect(() => parseLoginResponse({ something: "else" })).toThrow();
    expect(() => parseLoginResponse(null)).toThrow();
  });

  it("does not treat a false flag as a step-up", () => {
    expect(parseLoginResponse({ data: { uid: "u1", email: "a@b.test", tenant_id: "t1", mfa_required: false } }).kind)
      .toBe("complete");
  });
});
