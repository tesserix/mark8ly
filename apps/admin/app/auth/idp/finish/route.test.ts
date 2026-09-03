import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const configMock = vi.hoisted(() => ({
  authProvider: "zitadel" as "gip" | "zitadel",
  zitadelIssuer: "https://auth.tesserix.app",
}));
vi.mock("@/lib/config", () => ({ publicConfig: configMock }));

const tenantIdForHostSlugMock = vi.fn();
vi.mock("@/lib/auth/tenant-host", () => ({
  tenantIdForHostSlug: (...args: unknown[]) => tenantIdForHostSlugMock(...args),
}));

const finishZitadelGoogleSignInMock = vi.fn();
vi.mock("@/app/login/actions", () => ({
  finishZitadelGoogleSignIn: (...args: unknown[]) => finishZitadelGoogleSignInMock(...args),
}));

import { GET } from "./route";

function makeRequest(search: string, host = "demo-store-admin.mark8ly.com"): Request {
  return new Request(`https://${host}/auth/idp/finish${search}`, {
    headers: { host, "x-forwarded-proto": "https" },
  });
}

beforeEach(() => {
  configMock.authProvider = "zitadel";
  tenantIdForHostSlugMock.mockResolvedValue("tenant-1");
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("GET /auth/idp/finish", () => {
  it("404s under GIP — this route is unreachable outside the Zitadel provider", async () => {
    configMock.authProvider = "gip";

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(404);
    expect(finishZitadelGoogleSignInMock).not.toHaveBeenCalled();
  });

  it("redirects with google_sign_in_unavailable when Zitadel reports its own failure, without calling auth-bff", async () => {
    const res = await GET(
      makeRequest("?id=i1&error=access_denied&error_description=user+cancelled&auth_request_id=ar-1"),
    );

    expect(res.status).toBe(303);
    const location = res.headers.get("location") ?? "";
    expect(location).toContain("/login?");
    expect(location).toContain("error=google_sign_in_unavailable");
    expect(location).toContain("authRequest=ar-1");
    expect(finishZitadelGoogleSignInMock).not.toHaveBeenCalled();
    expect(res.headers.get("set-cookie")).toBeNull();
  });

  it("redirects with invalid_request when id/token/auth_request_id is missing", async () => {
    const res = await GET(makeRequest("?id=i1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain("error=invalid_request");
    expect(finishZitadelGoogleSignInMock).not.toHaveBeenCalled();
    expect(res.headers.get("set-cookie")).toBeNull();
  });

  it("redirects with store_not_found when the host does not resolve to a tenant", async () => {
    tenantIdForHostSlugMock.mockResolvedValue(null);

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain("error=store_not_found");
    expect(finishZitadelGoogleSignInMock).not.toHaveBeenCalled();
    expect(res.headers.get("set-cookie")).toBeNull();
  });

  it("calls finishZitadelGoogleSignIn with id/token/auth_request_id/workspaceTenant — never with `user`", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        callbackUrl: "https://demo-store-admin.mark8ly.com/auth/callback?code=c",
      },
    });

    await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1&user=attacker-supplied-uid"));

    expect(finishZitadelGoogleSignInMock).toHaveBeenCalledTimes(1);
    const call = finishZitadelGoogleSignInMock.mock.calls[0]![0];
    expect(call).toEqual({
      authRequestId: "ar-1",
      intentId: "i1",
      intentToken: "t1",
      workspaceTenant: "tenant-1",
    });
    expect(call).not.toHaveProperty("user");
  });

  it("a tampered `user` param changes nothing about the outcome", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        callbackUrl: "https://demo-store-admin.mark8ly.com/auth/callback?code=c",
      },
    });

    const withoutUser = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));
    const withTamperedUser = await GET(
      makeRequest("?id=i1&token=t1&auth_request_id=ar-1&user=someone-elses-uid"),
    );

    expect(withoutUser.headers.get("location")).toBe(withTamperedUser.headers.get("location"));
    expect(finishZitadelGoogleSignInMock).toHaveBeenCalledTimes(2);
    for (const call of finishZitadelGoogleSignInMock.mock.calls) {
      expect(call[0]).not.toHaveProperty("user");
    }
  });

  it("redirects to a trusted callback_url on a completed sign-in", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        callbackUrl: "https://demo-store-admin.mark8ly.com/auth/callback?code=c&state=s",
      },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toBe(
      "https://demo-store-admin.mark8ly.com/auth/callback?code=c&state=s",
    );
  });

  it("refuses to redirect to an untrusted callback_url", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        callbackUrl: "https://evil.example.com/steal",
      },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.headers.get("location")).not.toBe("https://evil.example.com/steal");
    expect(res.headers.get("location")).toContain("/dashboard");
  });

  it("falls back to /dashboard when there is no callback_url or handoff_url", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: { tenantId: "tenant-1", multipleTenants: false, mfaRequired: false, emailOtpRequired: false },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.headers.get("location")).toBe("https://demo-store-admin.mark8ly.com/dashboard");
  });

  it("redirects to a trusted Zitadel handoffUrl", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        handoffUrl: "https://auth.tesserix.app/ui/v2/login",
      },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.headers.get("location")).toBe("https://auth.tesserix.app/ui/v2/login");
  });

  it("refuses an untrusted handoffUrl", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        handoffUrl: "https://evil.example.com/steal",
      },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.headers.get("location")).not.toBe("https://evil.example.com/steal");
    expect(res.headers.get("location")).toContain("error=zitadel_unavailable");
  });

  it.each([
    ["no_admin_account"],
    ["unexpected_idp"],
    ["email_not_verified"],
    ["email_ambiguous"],
    ["invalid_return_url"],
    ["zitadel_unavailable"],
  ])("redirects with the %s outcome code on a rejected finish, without setting a cookie", async (code) => {
    finishZitadelGoogleSignInMock.mockResolvedValue({ ok: false, code, message: `http_${code}` });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain(`error=${code}`);
    expect(res.headers.get("set-cookie")).toBeNull();
  });

  it("maps an unrecognised failure code to internal_error rather than echoing it", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: false,
      code: "some_unexpected_backend_detail",
      message: "http_500",
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    const location = res.headers.get("location") ?? "";
    expect(location).not.toContain("some_unexpected_backend_detail");
    expect(location).toContain("error=internal_error");
  });

  it("redirects with step_up_unsupported when a step-up is outstanding, and sets no cookie on this response", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: { tenantId: "tenant-1", multipleTenants: false, mfaRequired: true, emailOtpRequired: false },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain("error=step_up_unsupported");
    expect(res.headers.get("set-cookie")).toBeNull();
  });
});
