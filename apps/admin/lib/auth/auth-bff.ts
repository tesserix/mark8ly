// Server-side typed client for auth-bff. Used from server actions only.
//
// /auth/auto-login mints a session cookie which the caller is responsible
// for forwarding to the browser via Next's response headers.

import { config } from "@/lib/config";
import { parseLoginResponse, type LoginOutcome } from "./login-response";

const base = config.authBffUrl;

export class AuthBffError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

interface AutoLoginRequest {
  idToken: string;
  expectedTenantId: string;
  workspaceTenant: string;
}

interface AutoLoginResult {
  uid: string;
  email: string;
  tenant_id: string;
  /** Every Set-Cookie header auth-bff emitted, as an array. auto-login
   *  may send more than one (e.g. the m8_mfa_pending cookie plus a
   *  clear for a stale session), and mfa-challenge always sends two
   *  (the new m8_session + a clear for m8_mfa_pending). The caller
   *  must forward ALL of them to the browser — `headers.get()` joins
   *  them with commas and breaks the parser. */
  setCookies: string[];
  /** True when the user has MFA enrolled. The caller should NOT treat
   *  this as a successful sign-in; it must collect the 6-digit code and
   *  call completeMFAChallenge before authenticated requests work. */
  mfaRequired: boolean;
  /** True when the sign-in came from an unrecognised device and auth-bff
   *  emailed a one-time code. Like mfaRequired this is NOT a completed
   *  sign-in — auto-login minted only a PENDING cookie, so redirecting
   *  now bounces the user straight back to /login with no error shown.
   *  The caller must collect the code and call completeEmailOTPChallenge. */
  emailOtpRequired: boolean;
}

interface MFAChallengeResult {
  uid: string;
  email: string;
  tenant_id: string;
  setCookies: string[];
}

interface SwitchTenantResult {
  /** Set-Cookie header value from auth-bff. The caller forwards this
   *  to the browser response. */
  setCookie: string;
}

/**
 * Switches the current session's tenant id. Phase P: called from
 * the tenant switcher dropdown and from the accept-invite server
 * action after a successful role grant.
 *
 * The caller MUST forward its own session cookies via cookieHeader
 * so auth-bff can read the existing session and verify membership
 * against the target tenant.
 */
/**
 * Switches the current session's store id under the same tenant.
 * Phase Q.2 — used by the store switcher and by the "Add store"
 * flow after creating a new store so the user lands in the newly
 * created store immediately.
 */
export async function switchStore(
  storeId: string,
  cookieHeader: string,
): Promise<SwitchTenantResult> {
  const res = await fetch(`${base}/auth/switch-store`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Cookie: cookieHeader,
    },
    body: JSON.stringify({ store_id: storeId }),
    cache: "no-store",
  });

  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? "auth_bff_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const setCookie = res.headers.get("set-cookie") ?? "";
  return { setCookie };
}

export async function switchTenant(
  tenantId: string,
  cookieHeader: string,
): Promise<SwitchTenantResult> {
  const res = await fetch(`${base}/auth/switch-tenant`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Cookie: cookieHeader,
    },
    body: JSON.stringify({ tenant_id: tenantId }),
    cache: "no-store",
  });

  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? "auth_bff_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const setCookie = res.headers.get("set-cookie") ?? "";
  return { setCookie };
}

export async function autoLogin(
  req: AutoLoginRequest,
): Promise<AutoLoginResult> {
  const res = await fetch(`${base}/auth/auto-login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      id_token: req.idToken,
      expected_tenant_id: req.expectedTenantId,
      workspace_tenant: req.workspaceTenant,
    }),
    cache: "no-store",
  });

  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? "auth_bff_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }

  const body = (await res.json()) as {
    data: {
      uid: string;
      email: string;
      tenant_id: string;
      mfa_required?: boolean;
      email_otp_required?: boolean;
    };
  };
  const setCookies = readAllSetCookies(res);

  return {
    uid: body.data.uid,
    email: body.data.email,
    tenant_id: body.data.tenant_id,
    setCookies,
    mfaRequired: body.data.mfa_required === true,
    emailOtpRequired: body.data.email_otp_required === true,
  };
}

/**
 * completeEmailOTPChallenge finishes a sign-in that auth-bff challenged
 * with an emailed one-time code. The caller forwards the pending cookie
 * auto-login set, plus the 6-digit code from the email. On success
 * auth-bff mints the real session cookie and clears the pending one.
 */
export async function completeEmailOTPChallenge(
  code: string,
  cookieHeader: string,
): Promise<{ uid: string; tenant_id: string; setCookies: string[] }> {
  const res = await fetch(`${base}/auth/otp/verify`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Cookie: cookieHeader },
    body: JSON.stringify({ code }),
    cache: "no-store",
  });
  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? `http_${res.status}`,
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const body = (await res.json()) as { uid: string; tenant_id: string };
  return {
    uid: body.uid,
    tenant_id: body.tenant_id,
    setCookies: readAllSetCookies(res),
  };
}

/** resendEmailOTP asks auth-bff for a fresh code on the same pending
 *  session. Rate limited upstream (429), which the caller surfaces. */
export async function resendEmailOTP(cookieHeader: string): Promise<void> {
  const res = await fetch(`${base}/auth/otp/resend`, {
    method: "POST",
    headers: { Cookie: cookieHeader },
    cache: "no-store",
  });
  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? `http_${res.status}`,
      body.message ?? `HTTP ${res.status}`,
    );
  }
}

// readAllSetCookies returns every Set-Cookie header auth-bff sent.
// Uses the Undici-specific `getSetCookie()` when present (Node 18+
// stable fetch), falling back to parsing the joined `.get()` value
// on older environments.
function readAllSetCookies(res: Response): string[] {
  const h = res.headers as unknown as {
    getSetCookie?: () => string[];
  };
  if (typeof h.getSetCookie === "function") {
    return h.getSetCookie();
  }
  const raw = res.headers.get("set-cookie");
  return raw ? [raw] : [];
}

/**
 * completeMFAChallenge finishes a sign-in that required a second
 * factor. The caller must forward the m8_mfa_pending cookie the
 * browser received from auto-login via cookieHeader, plus the
 * 6-digit TOTP code the user typed. On success auth-bff returns the
 * full session cookie, which the caller forwards to the browser.
 */
export interface LinkedProvider {
  providerId: string;
  email?: string;
}

export async function getMyProviders(
  cookieHeader: string,
): Promise<{ providers: LinkedProvider[] }> {
  const res = await fetch(`${base}/auth/me/providers`, {
    method: "GET",
    headers: {
      Cookie: cookieHeader,
    },
    cache: "no-store",
  });
  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? `http_${res.status}`,
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const wrapper = (await res.json()) as {
    data: { providers: { provider_id: string; email?: string }[] };
  };
  return {
    providers: wrapper.data.providers.map((p) => ({
      providerId: p.provider_id,
      email: p.email,
    })),
  };
}

export async function completeMFAChallenge(
  code: string,
  cookieHeader: string,
): Promise<MFAChallengeResult> {
  const res = await fetch(`${base}/auth/mfa-challenge`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Cookie: cookieHeader,
    },
    body: JSON.stringify({ code }),
    cache: "no-store",
  });

  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? "auth_bff_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }

  const body = (await res.json()) as {
    data: { user_id: string; email: string; tenant_id: string };
  };
  return {
    uid: body.data.user_id,
    email: body.data.email,
    tenant_id: body.data.tenant_id,
    setCookies: readAllSetCookies(res),
  };
}

interface ZitadelLoginRequest {
  authRequestId: string;
  loginName: string;
  password: string;
  workspaceTenant: string;
  /** Forwarded to auth-bff for device fingerprinting and the email-OTP
   *  rate limiter. auth-bff previously fingerprinted every user
   *  identically when this arrived empty from the server side (phase 2
   *  fix) — dropping it here from the browser side recreates the exact
   *  same silent failure, so it MUST be forwarded whenever the caller
   *  has it. */
  userAgent?: string;
  forwardedFor?: string;
}

interface ZitadelTotpRequest {
  authRequestId: string;
  sessionId: string;
  sessionToken: string;
  code: string;
  workspaceTenant: string;
  /** See ZitadelLoginRequest.userAgent. */
  userAgent?: string;
  forwardedFor?: string;
}

/**
 * zitadelLogin submits a Zitadel login-name + password pair against an
 * existing Zitadel auth request. The response can be a completed
 * sign-in, a TOTP step-up, auth-bff's own MFA/email-OTP step-up, or a
 * handoff to Zitadel's hosted UI — parseLoginResponse normalises all of
 * that, so this function does no parsing of its own beyond handing the
 * 2xx body to it.
 */
export async function zitadelLogin(
  req: ZitadelLoginRequest,
): Promise<LoginOutcome & { setCookies: string[] }> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (req.userAgent) headers["User-Agent"] = req.userAgent;
  if (req.forwardedFor) headers["X-Forwarded-For"] = req.forwardedFor;

  const res = await fetch(`${base}/auth/zitadel/login`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      auth_request_id: req.authRequestId,
      login_name: req.loginName,
      password: req.password,
      workspace_tenant: req.workspaceTenant,
    }),
    cache: "no-store",
  });

  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? "auth_bff_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }

  const body = await res.json();
  const outcome = parseLoginResponse(body);
  return { ...outcome, setCookies: readAllSetCookies(res) };
}

/**
 * zitadelTotp completes a Zitadel login that required a TOTP step-up.
 * The caller passes the session_id/session_token parseLoginResponse
 * returned from zitadelLogin's totp_required outcome, plus the 6-digit
 * code the user typed. Like zitadelLogin, the 2xx body is handed to
 * parseLoginResponse rather than parsed here.
 */
export async function zitadelTotp(
  req: ZitadelTotpRequest,
): Promise<LoginOutcome & { setCookies: string[] }> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (req.userAgent) headers["User-Agent"] = req.userAgent;
  if (req.forwardedFor) headers["X-Forwarded-For"] = req.forwardedFor;

  const res = await fetch(`${base}/auth/zitadel/totp`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      auth_request_id: req.authRequestId,
      session_id: req.sessionId,
      session_token: req.sessionToken,
      code: req.code,
      workspace_tenant: req.workspaceTenant,
    }),
    cache: "no-store",
  });

  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? "auth_bff_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }

  const body = await res.json();
  const outcome = parseLoginResponse(body);
  return { ...outcome, setCookies: readAllSetCookies(res) };
}
