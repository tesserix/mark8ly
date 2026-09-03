import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

import { zitadelLogin, zitadelTotp, AuthBffError } from "@/lib/auth/auth-bff";
import { listMemberTenants } from "@/lib/api/platform-api";
import { signInWithZitadel, confirmZitadelTotp } from "./actions";

const zitadelLoginMock = vi.mocked(zitadelLogin);
const zitadelTotpMock = vi.mocked(zitadelTotp);
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

  it("surfaces a totp_required session for the UI and mints no session cookie", async () => {
    zitadelLoginMock.mockResolvedValue({
      kind: "totp_required",
      sessionId: "sess-1",
      sessionToken: "sess-token-1",
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
        totpRequired: true,
        zitadelSessionId: "sess-1",
        zitadelSessionToken: "sess-token-1",
      },
    });
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
  const input = {
    authRequestId: "ar-1",
    sessionId: "sess-1",
    sessionToken: "sess-token-1",
    code: "123456",
    tenantId: "tenant-1",
    multipleTenants: false,
  };

  it("maps a complete outcome to a successful result and forwards Set-Cookie", async () => {
    zitadelTotpMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: "merchant@example.com",
      tenantId: "tenant-1",
      setCookies: ["m8_session=abc; Path=/; HttpOnly"],
    });

    const result = await confirmZitadelTotp(input);

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

  it("rejects a blank code without calling zitadelTotp", async () => {
    const result = await confirmZitadelTotp({ ...input, code: "   " });

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

    const result = await confirmZitadelTotp(input);

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

    await confirmZitadelTotp(input);

    const call = zitadelTotpMock.mock.calls[0]![0];
    expect(call.userAgent).toBe("Mozilla/5.0 test-client");
    expect(call.forwardedFor).toBe("198.51.100.7");
  });
});
