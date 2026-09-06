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
    // NOT ar-1 — see the recovery-sentinel describe block below.
    expect(new URL(location).searchParams.get("authRequest")).toBe("recovery");
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

  it("redirects with step_up_unsupported when auth-bff's usermfa gate is outstanding", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: { tenantId: "tenant-1", multipleTenants: false, mfaRequired: true, emailOtpRequired: false },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain("error=step_up_unsupported");
  });

  it("redirects with step_up_unsupported when Zitadel's own TOTP step-up is outstanding", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: {
        tenantId: "tenant-1",
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        totpRequired: true,
        zitadelSessionId: "sess-1",
        zitadelSessionToken: "sess-token-1",
        zitadelTenantCode: "tenant-code-1",
      },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    const location = res.headers.get("location") ?? "";
    expect(location).toContain("error=step_up_unsupported");
    // The whole reason TOTP stays refused: its continuation needs a
    // session id/token, and a redirect-only route has nowhere to put them
    // but the URL.
    expect(location).not.toContain("sess-1");
    expect(location).not.toContain("sess-token-1");
    expect(location).not.toContain("tenant-code-1");
  });
});

// #686 — the last gap in WEB merchant Google sign-in. A browser auth-bff
// has not fingerprinted before always trips the deviceguard/emailotp gate,
// so refusing email OTP here refused Google sign-in outright for every new
// device. This continues into the code screen the password path already
// has, and leaves TOTP/MFA refused.
describe("GET /auth/idp/finish — email-OTP continuation", () => {
  const emailOtpOutcome = (multipleTenants = false) => ({
    ok: true,
    data: {
      tenantId: "tenant-1",
      multipleTenants,
      mfaRequired: false,
      emailOtpRequired: true,
    },
  });

  it("redirects to /login's code step instead of refusing with step_up_unsupported", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue(emailOtpOutcome());

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    const url = new URL(res.headers.get("location") ?? "");
    expect(url.origin + url.pathname).toBe("https://admin.mark8ly.com/login");
    expect(url.searchParams.get("challenge")).toBe("email_otp");
    expect(url.searchParams.get("error")).toBeNull();
  });

  it("keeps the real auth request id so /login renders the form rather than bouncing through /login/authorize", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue(emailOtpOutcome());

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    const url = new URL(res.headers.get("location") ?? "");
    expect(url.searchParams.get("authRequest")).toBe("ar-1");
    expect(wouldMiddleware404(url.toString())).toBe(false);
  });

  it("flags a multi-store account so the post-code landing is /pick-tenant", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue(emailOtpOutcome(true));

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(new URL(res.headers.get("location") ?? "").searchParams.get("multi")).toBe("1");
  });

  it("omits the multi flag for a single-store account", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue(emailOtpOutcome(false));

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(new URL(res.headers.get("location") ?? "").searchParams.get("multi")).toBeNull();
  });

  it("puts no session id, session token, intent id or intent token in the redirect URL", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue(emailOtpOutcome());

    const res = await GET(
      makeRequest("?id=intent-abc&token=intent-token-xyz&auth_request_id=ar-1"),
    );

    const location = res.headers.get("location") ?? "";
    for (const secret of ["intent-abc", "intent-token-xyz", "sess", "token"]) {
      expect(location).not.toContain(secret);
    }
    // Only these three params, and nothing that could be a credential.
    const params = [...new URL(location).searchParams.keys()].sort();
    expect(params).toEqual(["authRequest", "challenge"]);
  });

  it("forwards the pending cookie auth-bff minted onto the redirect", async () => {
    // The pending Set-Cookie is applied by finishZitadelGoogleSignIn ->
    // mapZitadelOutcome -> applySetCookies, which writes through
    // next/headers' cookies(); Next merges that store onto whatever
    // response this handler returns, the same mechanism
    // app/auth/handoff/route.ts relies on to land a session cookie on a
    // 303. That mechanism is action-side, so this route test (which mocks
    // the action wholesale) cannot observe it — the assertion that the
    // email-OTP outcome really does apply its Set-Cookie headers lives in
    // app/login/actions.zitadel.test.ts, "forwards the PENDING cookie on
    // an email_otp_required outcome". This case is here only so the link
    // between the two is not lost: without it the merchant reaches the
    // code screen with no challenge to resume and a correct code fails.
    finishZitadelGoogleSignInMock.mockResolvedValue(emailOtpOutcome());

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
  });
});

// Production report: after a failed Google attempt the merchant followed
// the "sign in with your email and password instead" advice and got a raw
// Zitadel body — {"error": "No valid authentication request found"} —
// because this route handed back the auth request idp/complete had already
// spent. Every error redirect now carries the recovery sentinel instead.
describe("GET /auth/idp/finish — error redirects never reuse a spent auth request", () => {
  it("never carries the original auth_request_id on any error branch", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: false,
      code: "no_admin_account",
      message: "no admin account",
    });

    const scenarios = [
      makeRequest("?id=i1&error=access_denied&auth_request_id=ar-1"),
      makeRequest("?id=i1&token=t1&auth_request_id=ar-1"),
    ];
    for (const req of scenarios) {
      const res = await GET(req);
      const url = new URL(res.headers.get("location") ?? "");
      expect(url.searchParams.get("authRequest")).toBe("recovery");
      expect(url.toString()).not.toContain("ar-1");
    }
  });

  it("uses the sentinel on the step_up_unsupported branch too, where the auth request is definitely spent", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: true,
      data: { tenantId: "tenant-1", multipleTenants: false, mfaRequired: true, emailOtpRequired: false },
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(new URL(res.headers.get("location") ?? "").searchParams.get("authRequest")).toBe(
      "recovery",
    );
  });

  it("still satisfies middleware's canonical-/login gate with the sentinel", async () => {
    finishZitadelGoogleSignInMock.mockResolvedValue({
      ok: false,
      code: "no_admin_account",
      message: "no admin account",
    });

    const res = await GET(makeRequest("?id=i1&token=t1&auth_request_id=ar-1"));

    expect(wouldMiddleware404(res.headers.get("location") ?? "")).toBe(false);
  });
});
