import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const configMock = vi.hoisted(() => ({
  authProvider: "zitadel" as "gip" | "zitadel",
  zitadelIssuer: "https://auth.tesserix.app",
}));
vi.mock("@/lib/config", () => ({ publicConfig: configMock }));

const finishZitadelGoogleSignInMock = vi.fn();
vi.mock("@/app/login/actions", () => ({
  finishZitadelGoogleSignIn: (...args: unknown[]) => finishZitadelGoogleSignInMock(...args),
}));

import { GET } from "./route";
import { isValidSlugReturnUrl } from "@/lib/auth/host-policy";

// The REAL canonical host — the admin /login page (and so this route) is
// reachable ONLY here, never on a per-tenant `{slug}-admin.mark8ly.com`
// subdomain. A previous version of this flow was tested against
// `demo-store-admin.mark8ly.com`, a host the real flow can never present —
// every test below is pinned to the host that actually reaches this code.
const CANONICAL_HOST = "admin.mark8ly.com";

function makeRequest(search: string, host = CANONICAL_HOST): Request {
  return new Request(`https://${host}/auth/idp/finish${search}`, {
    headers: { host, "x-forwarded-proto": "https" },
  });
}

// Mirrors middleware.ts's EXACT canonical-/login 404 gate (see its own
// comment above that check): a request to canonical /login 404s unless it
// carries either a valid slug returnUrl or a non-empty authRequest. This
// route's error redirects always land on canonical /login (see
// errorRedirect), so any redirect Location this route produces must never
// satisfy this predicate — a blank 404 would swallow the truthful error
// message this route worked to produce.
function wouldMiddleware404(location: string): boolean {
  const url = new URL(location);
  const returnUrl = url.searchParams.get("returnUrl");
  const authRequest = url.searchParams.get("authRequest");
  return !isValidSlugReturnUrl(returnUrl) && !authRequest;
}

beforeEach(() => {
  configMock.authProvider = "zitadel";
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
  });

  it("redirects with invalid_request when id/token/auth_request_id is missing", async () => {
    const res = await GET(makeRequest("?id=i1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain("error=invalid_request");
    expect(finishZitadelGoogleSignInMock).not.toHaveBeenCalled();
  });

  it("includes a sentinel authRequest when auth_request_id itself is missing, so canonical /login does not 404 through middleware", async () => {
    // `?id=i1` alone: no token, no auth_request_id at all — the exact
    // scenario where the old code omitted `authRequest` entirely and left
    // the merchant looking at a blank 404 instead of the truthful message.
    const res = await GET(makeRequest("?id=i1"));

    const location = res.headers.get("location") ?? "";
    const authRequest = new URL(location).searchParams.get("authRequest");
    expect(authRequest).toBeTruthy();
    expect(wouldMiddleware404(location)).toBe(false);
  });

  it("never 404s through middleware on ANY error redirect this route can produce", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: false,
      code: "no_admin_account",
      message: "no admin account",
    });

    const scenarios = [
      makeRequest("?id=i1&error=access_denied&auth_request_id=ar-1"),
      makeRequest("?id=i1"),
      makeRequest("?id=i1&token=t1&auth_request_id=ar-1"),
    ];
    for (const req of scenarios) {
      const res = await GET(req);
      const location = res.headers.get("location") ?? "";
      expect(wouldMiddleware404(location)).toBe(false);
    }
  });

  it("calls finishZitadelGoogleSignIn with ONLY id/token/auth_request_id — never a host-derived tenant, never `user`", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        callbackUrl: "https://admin.mark8ly.com/auth/callback?code=c",
      },
    });

    await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1&user=attacker-supplied-uid"));

    expect(finishZitadelGoogleSignInMock).toHaveBeenCalledTimes(1);
    const call = finishZitadelGoogleSignInMock.mock.calls[0]![0];
    expect(call).toEqual({
      authRequestId: "ar-1",
      intentId: "i1",
      intentToken: "t1",
    });
    expect(call).not.toHaveProperty("user");
    expect(call).not.toHaveProperty("workspaceTenant");
  });

  it("a tampered `user` param changes nothing about the outcome", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        callbackUrl: "https://admin.mark8ly.com/auth/callback?code=c",
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
        callbackUrl: "https://admin.mark8ly.com/auth/callback?code=c&state=s",
      },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toBe(
      "https://admin.mark8ly.com/auth/callback?code=c&state=s",
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

    expect(res.headers.get("location")).toBe("https://admin.mark8ly.com/dashboard");
  });

  it("redirects to /pick-tenant instead of /dashboard when the account has more than one store", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: { tenantId: "tenant-1", multipleTenants: true, mfaRequired: false, emailOtpRequired: false },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.headers.get("location")).toBe("https://admin.mark8ly.com/pick-tenant");
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
    ["zitadel_unavailable"],
  ])("redirects with the %s outcome code on a rejected finish", async (code) => {
    finishZitadelGoogleSignInMock.mockResolvedValue({ ok: false, code, message: `http_${code}` });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain(`error=${code}`);
  });

  it("maps tenant_not_found (no store membership for a verified identity) onto no_admin_account", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: false,
      code: "tenant_not_found",
      message: "no store found for this account",
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.headers.get("location")).toContain("error=no_admin_account");
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

  it("redirects with step_up_unsupported when a step-up is outstanding", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: { tenantId: "tenant-1", multipleTenants: false, mfaRequired: true, emailOtpRequired: false },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain("error=step_up_unsupported");
  });
});
