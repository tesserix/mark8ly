import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// `actions.ts` reads SESSION_ENCRYPT_KEY at module-evaluation time, which
// happens before any statement in this file runs — so the env var has to
// be set via vi.hoisted (which really does run before the imports below
// are evaluated), not a plain top-level assignment.
const { SESSION_ENCRYPT_KEY } = vi.hoisted(() => {
  const key = "test-session-key-32-bytes-pad!!";
  process.env.SESSION_ENCRYPT_KEY = key;
  return { SESSION_ENCRYPT_KEY: key };
});

const cookieStore: { name: string; value: string }[] = [];
const cookiesSetSpy = vi.fn((opts: { name: string; value: string }) => {
  cookieStore.push({ name: opts.name, value: opts.value });
});

let headerMap: Map<string, string>;

vi.mock("next/headers", () => ({
  headers: async () => ({
    get: (key: string) => headerMap.get(key.toLowerCase()) ?? null,
  }),
  cookies: async () => ({
    set: cookiesSetSpy,
    getAll: () => [],
  }),
}));

vi.mock("@/lib/auth/auth-bff", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth/auth-bff")>(
    "@/lib/auth/auth-bff",
  );
  return {
    ...actual,
    zitadelLogin: vi.fn(),
    zitadelTotp: vi.fn(),
    zitadelIdpFinish: vi.fn(),
    zitadelIdpComplete: vi.fn(),
  };
});

vi.mock("@/lib/api/platform-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/platform-api")>(
    "@/lib/api/platform-api",
  );
  return {
    ...actual,
    listMemberTenants: vi.fn(),
  };
});

import { zitadelLogin, zitadelTotp, zitadelIdpFinish, zitadelIdpComplete, AuthBffError } from "@/lib/auth/auth-bff";
import { listMemberTenants } from "@/lib/api/platform-api";
import { mintZitadelTotpCode, verifyZitadelTotpCode } from "@repo/ui/auth/zitadel-totp-code";
import { signInWithZitadel, confirmZitadelTotp, finishZitadelGoogleSignIn } from "./actions";

const zitadelLoginMock = vi.mocked(zitadelLogin);
const zitadelTotpMock = vi.mocked(zitadelTotp);
const zitadelIdpFinishMock = vi.mocked(zitadelIdpFinish);
const zitadelIdpCompleteMock = vi.mocked(zitadelIdpComplete);
const listMemberTenantsMock = vi.mocked(listMemberTenants);

const SUBMITTED_PASSWORD = "correct-horse-battery-staple";

beforeEach(() => {
  headerMap = new Map([
    ["host", "admin.mark8ly.com"],
    ["user-agent", "Mozilla/5.0 test-client"],
    ["x-forwarded-for", "198.51.100.7"],
  ]);
  listMemberTenantsMock.mockResolvedValue([
    { tenant_id: "tenant-1", name: "The Bondi Store", role: "owner" },
  ]);
  cookieStore.length = 0;
  cookiesSetSpy.mockClear();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("signInWithZitadel", () => {
  const input = {
    email: "merchant@example.com",
    password: SUBMITTED_PASSWORD,
    authRequestId: "ar-1",
  };

  it("maps a complete outcome to a successful result and forwards Set-Cookie the way the GIP path does", async () => {
    zitadelLoginMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: input.email,
      tenantId: "tenant-1",
      setCookies: ["m8_session=abc; Path=/; HttpOnly"],
    });

    const result = await signInWithZitadel(input);

    expect(result).toEqual({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
      },
    });
    expect(cookiesSetSpy).toHaveBeenCalledTimes(1);
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "m8_session", value: "abc" }),
    );
  });

  it("carries a callback_url through on a complete outcome", async () => {
    zitadelLoginMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: input.email,
      tenantId: "tenant-1",
      callbackUrl: "https://zitadel.example/callback",
      setCookies: [],
    });

    const result = await signInWithZitadel(input);

    expect(result).toEqual({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        callbackUrl: "https://zitadel.example/callback",
      },
    });
  });

  it("maps mfa_required to mfaRequired: true with no callbackUrl", async () => {
    zitadelLoginMock.mockResolvedValue({
      kind: "mfa_required",
      setCookies: ["m8_mfa_pending=xyz; Path=/; HttpOnly"],
    });

    const result = await signInWithZitadel(input);

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.data.mfaRequired).toBe(true);
      expect(result.data.emailOtpRequired).toBe(false);
      expect(result.data).not.toHaveProperty("callbackUrl");
    }
  });

  it("maps email_otp_required to emailOtpRequired: true", async () => {
    zitadelLoginMock.mockResolvedValue({
      kind: "email_otp_required",
      setCookies: ["m8_otp_pending=xyz; Path=/; HttpOnly"],
    });

    const result = await signInWithZitadel(input);

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.data.emailOtpRequired).toBe(true);
      expect(result.data.mfaRequired).toBe(false);
      expect(result.data).not.toHaveProperty("callbackUrl");
    }
  });

  it("surfaces a totp_required session for the UI, mints a signed tenant code, and mints no session cookie", async () => {
    zitadelLoginMock.mockResolvedValue({
      kind: "totp_required",
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      setCookies: [],
    });

    const result = await signInWithZitadel(input);

    expect(result.ok).toBe(true);
    if (!result.ok) throw new Error("expected an ok result");
    expect(result.data).toMatchObject({
      tenantId: "tenant-1",
      multipleTenants: false,
      mfaRequired: false,
      emailOtpRequired: false,
      totpRequired: true,
      zitadelSessionId: "sess-1",
      zitadelSessionToken: "sess-token-1",
    });
    expect(typeof result.data.zitadelTenantCode).toBe("string");
    // The tenant carried inside the opaque code is the server-resolved
    // one, not anything a client could have chosen.
    const claims = verifyZitadelTotpCode(result.data.zitadelTenantCode!, SESSION_ENCRYPT_KEY);
    expect(claims.tenant_id).toBe("tenant-1");
    expect(claims.multiple_tenants).toBe(false);
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it("surfaces a handoff URL", async () => {
    zitadelLoginMock.mockResolvedValue({
      kind: "handoff",
      handoffUrl: "https://zitadel.example/ui/login",
      setCookies: [],
    });

    const result = await signInWithZitadel(input);

    expect(result).toEqual({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        handoffUrl: "https://zitadel.example/ui/login",
      },
    });
  });

  it("maps an AuthBffError to {ok: false, code, message} via fail(), without leaking the password", async () => {
    zitadelLoginMock.mockRejectedValue(
      new AuthBffError(401, "invalid_credentials", "Wrong login name or password"),
    );

    const result = await signInWithZitadel(input);

    expect(result).toEqual({
      ok: false,
      code: "invalid_credentials",
      message: "Wrong login name or password",
    });
    expect(JSON.stringify(result)).not.toContain(SUBMITTED_PASSWORD);
  });

  it("passes the User-Agent and X-Forwarded-For from the incoming request to zitadelLogin, not empty strings", async () => {
    zitadelLoginMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: input.email,
      tenantId: "tenant-1",
      setCookies: [],
    });

    await signInWithZitadel(input);

    expect(zitadelLoginMock).toHaveBeenCalledTimes(1);
    const call = zitadelLoginMock.mock.calls[0]![0];
    expect(call.userAgent).toBe("Mozilla/5.0 test-client");
    expect(call.forwardedFor).toBe("198.51.100.7");
    expect(call.userAgent).not.toBe("");
    expect(call.forwardedFor).not.toBe("");
  });

  it("resolves the same tenant signIn would for a multi-tenant account, honoring the host-slug match", async () => {
    headerMap.set("host", "demo-store-admin.mark8ly.com");
    // Two memberships; without the host match the picker would default to
    // tenants[0] ("tenant-1"). platform-api's by-slug lookup is mocked via
    // fetch in tenantIdForHostSlug — since it is best-effort and swallows
    // network errors, with no fetch mock it resolves to null and falls
    // back to tenants[0]. This test only pins that both paths funnel
    // through the exact same resolveWorkspaceTenant logic (multipleTenants
    // reflects tenants.length > 1 when the host does not resolve).
    listMemberTenantsMock.mockResolvedValue([
      { tenant_id: "tenant-1", name: "Store One", role: "owner" },
      { tenant_id: "tenant-2", name: "Store Two", role: "staff" },
    ]);
    zitadelLoginMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: input.email,
      tenantId: "tenant-1",
      setCookies: [],
    });

    const result = await signInWithZitadel(input);

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.data.multipleTenants).toBe(true);
    }
  });

  it("maps an unexpected (non-AuthBffError) failure to a generic message via fail(), logging the detail instead", async () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const internal = new Error("LoginResponseError: unrecognised login response shape");
    zitadelLoginMock.mockRejectedValue(internal);

    const result = await signInWithZitadel(input);

    expect(result).toEqual({
      ok: false,
      code: "unknown",
      message: "Something went wrong. Please try again.",
    });
    expect(JSON.stringify(result)).not.toContain("LoginResponseError");
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      "login action failed with an unexpected error",
      internal,
    );
    consoleErrorSpy.mockRestore();
  });

  it("returns tenant_not_found when the account has no memberships", async () => {
    listMemberTenantsMock.mockResolvedValue([]);

    const result = await signInWithZitadel(input);

    expect(result).toEqual({
      ok: false,
      code: "tenant_not_found",
      message: "no store found for this account",
    });
    expect(zitadelLoginMock).not.toHaveBeenCalled();
  });
});

describe("confirmZitadelTotp", () => {
  const baseInput = {
    authRequestId: "ar-1",
    sessionId: "sess-1",
    sessionToken: "sess-token-1",
    code: "123456",
  };

  function validCode(tenantId = "tenant-1", multipleTenants = false): string {
    return mintZitadelTotpCode(
      { tenant_id: tenantId, multiple_tenants: multipleTenants },
      SESSION_ENCRYPT_KEY,
      300,
    );
  }

  it("maps a complete outcome to a successful result and forwards Set-Cookie", async () => {
    zitadelTotpMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: "merchant@example.com",
      tenantId: "tenant-1",
      setCookies: ["m8_session=abc; Path=/; HttpOnly"],
    });

    const result = await confirmZitadelTotp({ ...baseInput, zitadelTenantCode: validCode() });

    expect(result).toEqual({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
      },
    });
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "m8_session", value: "abc" }),
    );
  });

  it("uses the tenant from the verified claims for workspace_tenant — not a client-supplied field, there isn't one on the input type", async () => {
    zitadelTotpMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: "merchant@example.com",
      tenantId: "tenant-9",
      setCookies: [],
    });

    const result = await confirmZitadelTotp({
      ...baseInput,
      zitadelTenantCode: validCode("tenant-9", true),
    });

    expect(zitadelTotpMock).toHaveBeenCalledTimes(1);
    const call = zitadelTotpMock.mock.calls[0]![0];
    expect(call.workspaceTenant).toBe("tenant-9");
    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.data.tenantId).toBe("tenant-9");
      expect(result.data.multipleTenants).toBe(true);
    }
  });

  it("rejects a tampered code and returns {ok: false} rather than throwing", async () => {
    const tampered = validCode().replace(/\.[^.]+$/, ".bad-signature");

    const result = await confirmZitadelTotp({ ...baseInput, zitadelTenantCode: tampered });

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.code).toBe("invalid_signature");
    }
    expect(zitadelTotpMock).not.toHaveBeenCalled();
  });

  it("rejects an expired code and returns {ok: false} rather than throwing", async () => {
    const expired = mintZitadelTotpCode(
      { tenant_id: "tenant-1", multiple_tenants: false },
      SESSION_ENCRYPT_KEY,
      -1,
    );

    const result = await confirmZitadelTotp({ ...baseInput, zitadelTenantCode: expired });

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.code).toBe("expired");
    }
    expect(zitadelTotpMock).not.toHaveBeenCalled();
  });

  it("rejects a malformed code and returns {ok: false} rather than throwing", async () => {
    const result = await confirmZitadelTotp({ ...baseInput, zitadelTenantCode: "not-a-code" });

    expect(result.ok).toBe(false);
    expect(zitadelTotpMock).not.toHaveBeenCalled();
  });

  it("rejects a blank code without calling zitadelTotp", async () => {
    const result = await confirmZitadelTotp({
      ...baseInput,
      code: "   ",
      zitadelTenantCode: validCode(),
    });

    expect(result).toEqual({
      ok: false,
      code: "invalid_code",
      message: "Enter the 6-digit code from your authenticator app.",
    });
    expect(zitadelTotpMock).not.toHaveBeenCalled();
  });

  it("maps an AuthBffError via fail()", async () => {
    zitadelTotpMock.mockRejectedValue(
      new AuthBffError(401, "invalid_code", "Incorrect verification code"),
    );

    const result = await confirmZitadelTotp({ ...baseInput, zitadelTenantCode: validCode() });

    expect(result).toEqual({
      ok: false,
      code: "invalid_code",
      message: "Incorrect verification code",
    });
  });

  it("passes the User-Agent and X-Forwarded-For from the incoming request to zitadelTotp", async () => {
    zitadelTotpMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: "merchant@example.com",
      tenantId: "tenant-1",
      setCookies: [],
    });

    await confirmZitadelTotp({ ...baseInput, zitadelTenantCode: validCode() });

    const call = zitadelTotpMock.mock.calls[0]![0];
    expect(call.userAgent).toBe("Mozilla/5.0 test-client");
    expect(call.forwardedFor).toBe("198.51.100.7");
  });
});

describe("finishZitadelGoogleSignIn", () => {
  const input = {
    authRequestId: "ar-1",
    intentId: "intent-1",
    intentToken: "intent-token-1",
  };

  it("calls idp/finish WITHOUT a workspace tenant, resolves one from login_name, then calls idp/complete", async () => {
    zitadelIdpFinishMock.mockResolvedValue({
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      loginName: "merchant@example.com",
    });
    zitadelIdpCompleteMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: "merchant@example.com",
      tenantId: "tenant-1",
      setCookies: ["m8_session=abc; Path=/; HttpOnly"],
    });

    const result = await finishZitadelGoogleSignIn(input);

    expect(zitadelIdpFinishMock).toHaveBeenCalledTimes(1);
    const finishCall = zitadelIdpFinishMock.mock.calls[0]![0];
    expect(finishCall).toEqual({
      authRequestId: "ar-1",
      intentId: "intent-1",
      intentToken: "intent-token-1",
      userAgent: "Mozilla/5.0 test-client",
      forwardedFor: "198.51.100.7",
    });

    expect(listMemberTenantsMock).toHaveBeenCalledWith("merchant@example.com");

    expect(zitadelIdpCompleteMock).toHaveBeenCalledTimes(1);
    expect(zitadelIdpCompleteMock).toHaveBeenCalledWith({
      authRequestId: "ar-1",
      loginName: "merchant@example.com",
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      workspaceTenant: "tenant-1",
      userAgent: "Mozilla/5.0 test-client",
      forwardedFor: "198.51.100.7",
    });

    expect(result).toEqual({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
      },
    });
    expect(cookiesSetSpy).toHaveBeenCalledTimes(1);
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "m8_session", value: "abc" }),
    );
  });

  it("a tampered `user` value never enters this path at all — there is no parameter for it", async () => {
    zitadelIdpFinishMock.mockResolvedValue({
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      loginName: "merchant@example.com",
    });
    zitadelIdpCompleteMock.mockResolvedValue({
      kind: "complete",
      setCookies: [],
    });

    await finishZitadelGoogleSignIn(input);

    const finishCall = zitadelIdpFinishMock.mock.calls[0]![0];
    const completeCall = zitadelIdpCompleteMock.mock.calls[0]![0];
    expect(finishCall).not.toHaveProperty("user");
    expect(completeCall).not.toHaveProperty("user");
  });

  it("returns tenant_not_found and never calls idp/complete when the identity has no store membership", async () => {
    zitadelIdpFinishMock.mockResolvedValue({
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      loginName: "merchant@example.com",
    });
    listMemberTenantsMock.mockResolvedValue([]);

    const result = await finishZitadelGoogleSignIn(input);

    expect(result).toEqual({
      ok: false,
      code: "tenant_not_found",
      message: "no store found for this account",
    });
    expect(zitadelIdpCompleteMock).not.toHaveBeenCalled();
  });

  it("falls back to tenants[0] and flags multipleTenants on a multi-store account — this flow never has a slug to subdomain-match against", async () => {
    // headerMap's host is already the canonical admin.mark8ly.com (see the
    // top-level beforeEach) — the ONLY host this flow ever runs on, since
    // /login (and so this whole Google path) is unreachable anywhere else.
    // resolveWorkspaceTenant's subdomain refinement can never fire here
    // (there is no `{slug}-admin.mark8ly.com` to match), so a multi-store
    // account always falls through to tenants[0] with multipleTenants
    // flagged, exactly like the password path's equivalent test on that
    // same canonical host.
    listMemberTenantsMock.mockResolvedValue([
      { tenant_id: "tenant-1", name: "Store One", role: "owner" },
      { tenant_id: "tenant-2", name: "Store Two", role: "staff" },
    ]);
    zitadelIdpFinishMock.mockResolvedValue({
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      loginName: "merchant@example.com",
    });
    zitadelIdpCompleteMock.mockResolvedValue({ kind: "complete", setCookies: [] });

    const result = await finishZitadelGoogleSignIn(input);

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.data.tenantId).toBe("tenant-1");
      expect(result.data.multipleTenants).toBe(true);
    }
    expect(zitadelIdpCompleteMock).toHaveBeenCalledWith(
      expect.objectContaining({ workspaceTenant: "tenant-1" }),
    );
  });

  it("propagates an AuthBffError from idp/finish (e.g. no_admin_account) without calling idp/complete", async () => {
    zitadelIdpFinishMock.mockRejectedValue(
      new AuthBffError(403, "no_admin_account", "no admin account for this identity"),
    );

    const result = await finishZitadelGoogleSignIn(input);

    expect(result).toEqual({
      ok: false,
      code: "no_admin_account",
      message: "no admin account for this identity",
    });
    expect(listMemberTenantsMock).not.toHaveBeenCalled();
    expect(zitadelIdpCompleteMock).not.toHaveBeenCalled();
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it("sets no cookie when the identity has no store membership either", async () => {
    zitadelIdpFinishMock.mockResolvedValue({
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      loginName: "merchant@example.com",
    });
    listMemberTenantsMock.mockResolvedValue([]);

    await finishZitadelGoogleSignIn(input);

    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  // THE regression that matters for #686's email-OTP continuation. The
  // finish route now redirects this outcome to /login's code step instead
  // of refusing it. auth-bff mints only a PENDING cookie at this point,
  // and confirmEmailOTPLogin resumes from that cookie plus the typed code
  // — so if these Set-Cookie headers are dropped, the merchant reaches the
  // code screen, types a CORRECT code, and verification fails with no
  // pending challenge to resume. That is strictly worse than the honest
  // refusal it replaced, which is why it is pinned here rather than left
  // to the route test (which mocks this action wholesale).
  it("forwards the PENDING cookie on an email_otp_required outcome", async () => {
    zitadelIdpFinishMock.mockResolvedValue({
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      loginName: "merchant@example.com",
    });
    zitadelIdpCompleteMock.mockResolvedValue({
      kind: "email_otp_required",
      setCookies: [
        "m8_otp_pending=pending-abc; Path=/; HttpOnly; Secure; Max-Age=300",
        "m8_device=dev-1; Path=/; HttpOnly",
      ],
    });

    const result = await finishZitadelGoogleSignIn(input);

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.data.emailOtpRequired).toBe(true);
      expect(result.data.mfaRequired).toBe(false);
    }

    // EVERY Set-Cookie auth-bff sent, not just the first.
    expect(cookiesSetSpy).toHaveBeenCalledTimes(2);
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "m8_otp_pending",
        value: "pending-abc",
        httpOnly: true,
        secure: true,
        maxAge: 300,
      }),
    );
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "m8_device", value: "dev-1" }),
    );
    expect(cookieStore.map((c) => c.name)).toEqual(["m8_otp_pending", "m8_device"]);
  });

  it("carries multipleTenants through an email_otp_required outcome so the post-code landing is right", async () => {
    listMemberTenantsMock.mockResolvedValue([
      { tenant_id: "tenant-1", name: "Store One", role: "owner" },
      { tenant_id: "tenant-2", name: "Store Two", role: "staff" },
    ]);
    zitadelIdpFinishMock.mockResolvedValue({
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      loginName: "merchant@example.com",
    });
    zitadelIdpCompleteMock.mockResolvedValue({
      kind: "email_otp_required",
      setCookies: ["m8_otp_pending=pending-abc; Path=/; HttpOnly"],
    });

    const result = await finishZitadelGoogleSignIn(input);

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.data.multipleTenants).toBe(true);
      expect(result.data.emailOtpRequired).toBe(true);
    }
  });

  it("surfaces a step-up outcome from idp/complete with no session cookie minted", async () => {
    zitadelIdpFinishMock.mockResolvedValue({
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      loginName: "merchant@example.com",
    });
    zitadelIdpCompleteMock.mockResolvedValue({ kind: "mfa_required", setCookies: [] });

    const result = await finishZitadelGoogleSignIn(input);

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.data.mfaRequired).toBe(true);
    }
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });

  it("passes the User-Agent and X-Forwarded-For from the incoming request to both idp calls", async () => {
    zitadelIdpFinishMock.mockResolvedValue({
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
      loginName: "merchant@example.com",
    });
    zitadelIdpCompleteMock.mockResolvedValue({ kind: "complete", setCookies: [] });

    await finishZitadelGoogleSignIn(input);

    expect(zitadelIdpFinishMock.mock.calls[0]![0].userAgent).toBe("Mozilla/5.0 test-client");
    expect(zitadelIdpFinishMock.mock.calls[0]![0].forwardedFor).toBe("198.51.100.7");
    expect(zitadelIdpCompleteMock.mock.calls[0]![0].userAgent).toBe("Mozilla/5.0 test-client");
    expect(zitadelIdpCompleteMock.mock.calls[0]![0].forwardedFor).toBe("198.51.100.7");
  });
});
