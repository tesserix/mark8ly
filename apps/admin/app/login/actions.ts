"use server";

// Server actions for the admin /login page.
//
// The client gathers GIP credentials (via signInWithPassword or
// signInWithGoogle), then hands the resulting id_token + uid to signIn.
// We look up the workspace tenant by uid, call auth-bff /auth/auto-login,
// and forward the resulting Set-Cookie back to the browser response.

import { cookies, headers } from "next/headers";

import {
  autoLogin,
  completeEmailOTPChallenge,
  resendEmailOTP,
  completeMFAChallenge,
  zitadelLogin,
  zitadelTotp,
  zitadelIdpFinish,
  AuthBffError,
} from "@/lib/auth/auth-bff";
import type { LoginOutcome } from "@/lib/auth/login-response";
import {
  listMemberTenants,
  PlatformApiError,
  type Membership,
} from "@/lib/api/platform-api";
import { publicConfig } from "@/lib/config";
import { tenantIdForHostSlug } from "@/lib/auth/tenant-host";
import {
  mintZitadelTotpCode,
  verifyZitadelTotpCode,
  ZitadelTotpCodeError,
} from "@repo/ui/auth/zitadel-totp-code";

// Same HMAC key every other short-lived admin handoff/exchange code in
// this app signs with (see cross-domain-handoff.ts, auth/handoff/route.ts).
const SESSION_ENCRYPT_KEY = process.env.SESSION_ENCRYPT_KEY ?? "";

// Long enough for a user to fetch a code from their authenticator app,
// short enough to be useless if it leaked anywhere afterward.
const ZITADEL_TOTP_CODE_TTL_SECONDS = 300;

type Result<T> =
  | { ok: true; data: T }
  | { ok: false; code: string; message: string };

function fail(err: unknown): { ok: false; code: string; message: string } {
  if (err instanceof PlatformApiError || err instanceof AuthBffError) {
    return { ok: false, code: err.code, message: err.message };
  }
  // An error we didn't anticipate (e.g. a thrown LoginResponseError from
  // parseLoginResponse) must never surface as user-facing copy — that
  // leaks internal error shapes/messages to the browser. Log the detail
  // server-side and hand the client a generic message instead.
  console.error("login action failed with an unexpected error", err);
  return {
    ok: false,
    code: "unknown",
    message: "Something went wrong. Please try again.",
  };
}

interface SignInInput {
  idToken: string;
  uid: string;
}

interface SignInSuccess {
  tenantId: string;
  // true when the user has more than one tenant and the UI should
  // redirect them to /pick-tenant after the initial dashboard land.
  // The initial tenant is the first one returned by platform-api's
  // /users/me/tenants (priority: owner first, then by created_at).
  multipleTenants: boolean;
  // true when the user has MFA enabled — the signIn flow wrote the
  // short-lived m8_mfa_pending cookie instead of a real session.
  // The UI must collect a 6-digit code and call confirmMFALogin
  // before treating the sign-in as complete.
  mfaRequired: boolean;
  // true when the sign-in came from an unrecognised device and auth-bff
  // emailed a one-time code, writing only a PENDING cookie. Treating
  // this as success redirects into a middleware bounce back to /login
  // with no error — the UI must collect the code and call
  // confirmEmailOTPLogin.
  emailOtpRequired: boolean;
  // Present when the sign-in came back with a Zitadel-issued
  // callback_url on completion. The GIP path never sets this.
  callbackUrl?: string;
  // true when Zitadel itself (not auth-bff's usermfa gate) demands a
  // TOTP code before the auth request can complete. zitadelSessionId /
  // zitadelSessionToken / zitadelTenantCode must be carried by the UI
  // into confirmZitadelTotp unchanged — no session cookie is minted at
  // this point.
  totpRequired?: boolean;
  zitadelSessionId?: string;
  zitadelSessionToken?: string;
  // Opaque HMAC-signed code carrying the server-resolved tenantId /
  // multipleTenants (see zitadel-totp-code.ts). confirmZitadelTotp
  // verifies this instead of trusting client-supplied tenant fields —
  // the tenant used for workspace_tenant must always come from the
  // server's own resolution, the same posture the GIP path has via
  // resolveWorkspaceTenant.
  zitadelTenantCode?: string;
  // Present when auth-bff handed back a handoff to Zitadel's hosted UI
  // instead of completing the login inline.
  handoffUrl?: string;
}

/**
 * resolveWorkspaceTenant picks the tenant a sign-in should land in.
 *
 * Phase P: multi-tenant membership. A user can own one tenant and be
 * staff on another, so "find my workspace tenant" is no longer a
 * single-row lookup. We list every tenant the identity key has any
 * role on, pick the first, and flag the caller if there's more than
 * one so the UI can offer a picker.
 *
 * Subdomain-aware primary selection: when the user signs in on
 * `{slug}-admin.mark8ly.com` AND the slug resolves to a tenant the
 * user belongs to, use that tenant instead of `tenants[0]`. Without
 * this, a founder who owns `india-store` but also has a staff role on
 * `demo-store` lands on the demo-store picker even though the
 * india-store subdomain is unambiguous.
 *
 * Shared by both the GIP (`signIn`) and Zitadel (`signInWithZitadel`)
 * paths so they cannot diverge on which tenant they pick. `identityKey`
 * is the GIP uid on the GIP path; on the Zitadel path there is no GIP
 * uid yet, so the caller passes the Zitadel login name (email), which
 * platform-api's membership lookup accepts identically.
 */
async function resolveWorkspaceTenant(
  identityKey: string,
): Promise<
  | { ok: true; primary: Membership; multipleTenants: boolean }
  | { ok: false; code: string; message: string }
> {
  const tenants = await listMemberTenants(identityKey);
  if (tenants.length === 0) {
    return {
      ok: false,
      code: "tenant_not_found",
      message: "no store found for this account",
    };
  }

  const h = await headers();
  const hostTenantId = await tenantIdForHostSlug(h.get("host"));
  const hostMatched = hostTenantId
    ? tenants.find((t) => t.tenant_id === hostTenantId) ?? null
    : null;
  const primary = hostMatched ?? tenants[0];
  if (!primary) {
    return {
      ok: false,
      code: "tenant_not_found",
      message: "no store found for this account",
    };
  }

  // When the host uniquely selects a tenant, skip the picker even if
  // the user belongs to multiple stores — we've already committed to
  // the one their subdomain points at.
  const multipleTenants = tenants.length > 1 && !hostMatched;

  return { ok: true, primary, multipleTenants };
}

export async function signIn(
  input: SignInInput,
): Promise<Result<SignInSuccess>> {
  try {
    const resolution = await resolveWorkspaceTenant(input.uid);
    if (!resolution.ok) return resolution;
    const { primary, multipleTenants } = resolution;

    const result = await autoLogin({
      idToken: input.idToken,
      expectedTenantId: publicConfig.gipTenantId,
      workspaceTenant: primary.tenant_id,
    });

    await applySetCookies(result.setCookies);

    return {
      ok: true,
      data: {
        tenantId: primary.tenant_id,
        multipleTenants,
        mfaRequired: result.mfaRequired,
        emailOtpRequired: result.emailOtpRequired,
      },
    };
  } catch (err) {
    return fail(err);
  }
}

/**
 * signInWithZitadel is the Zitadel-path counterpart of `signIn`: it
 * resolves the workspace tenant with the exact same helper (so the two
 * paths can never pick different tenants for the same account), submits
 * the login-name + password pair to auth-bff's Zitadel endpoint, and
 * maps whatever `LoginOutcome` comes back onto the same `SignInSuccess`
 * shape the login form already reads.
 *
 * User-Agent and X-Forwarded-For are read from the incoming request and
 * forwarded to auth-bff. This is load-bearing, not hygiene: auth-bff
 * fingerprints the device from the user agent and rate-limits email OTP
 * by client IP. Phase 2 fixed exactly this defect one layer down
 * (server-to-server); omitting it here would silently re-create it one
 * layer up (browser-to-server) — every user would collapse onto one
 * device fingerprint with no failing test and no error log.
 */
export interface SignInWithZitadelInput {
  email: string;
  password: string;
  authRequestId: string;
}

export async function signInWithZitadel(
  input: SignInWithZitadelInput,
): Promise<Result<SignInSuccess>> {
  try {
    const resolution = await resolveWorkspaceTenant(input.email);
    if (!resolution.ok) return resolution;
    const { primary, multipleTenants } = resolution;

    const h = await headers();
    const outcome = await zitadelLogin({
      authRequestId: input.authRequestId,
      loginName: input.email,
      password: input.password,
      workspaceTenant: primary.tenant_id,
      userAgent: h.get("user-agent") ?? undefined,
      forwardedFor: h.get("x-forwarded-for") ?? undefined,
    });

    return await mapZitadelOutcome(outcome, primary.tenant_id, multipleTenants);
  } catch (err) {
    return fail(err);
  }
}

/**
 * confirmZitadelTotp finishes a Zitadel sign-in that Zitadel itself
 * stepped up with a TOTP challenge (distinct from auth-bff's own
 * usermfa gate, which `confirmMFALogin` handles). `zitadelTotp` needs
 * `workspace_tenant`, and there is no pending-cookie mechanism yet on
 * the Zitadel path to recover it server-side the way `confirmMFALogin`
 * recovers auth-bff's own m8_mfa_pending cookie.
 *
 * The tenant therefore travels as `zitadelTenantCode` — an opaque,
 * HMAC-signed code `signInWithZitadel` minted from its own
 * `resolveWorkspaceTenant` result — rather than as raw `tenantId` /
 * `multipleTenants` fields the client could set at will. auth-bff
 * re-checks membership independently on every call, so a forged tenant
 * id could never reach a tenant the caller doesn't belong to either
 * way; this is about keeping this path's posture identical to the GIP
 * path, where the tenant is always server-derived and never
 * client-supplied, especially once GIP is retired and this is the only
 * login path left.
 */
export interface ConfirmZitadelTotpInput {
  authRequestId: string;
  sessionId: string;
  sessionToken: string;
  code: string;
  zitadelTenantCode: string;
}

export async function confirmZitadelTotp(
  input: ConfirmZitadelTotpInput,
): Promise<Result<SignInSuccess>> {
  try {
    const trimmed = input.code.trim();
    if (!trimmed) {
      return {
        ok: false,
        code: "invalid_code",
        message: "Enter the 6-digit code from your authenticator app.",
      };
    }

    let claims: { tenant_id: string; multiple_tenants: boolean };
    try {
      claims = verifyZitadelTotpCode(input.zitadelTenantCode, SESSION_ENCRYPT_KEY);
    } catch (err) {
      const code = err instanceof ZitadelTotpCodeError ? err.code : "invalid_tenant_code";
      return {
        ok: false,
        code,
        message: "Your sign-in session expired. Please sign in again.",
      };
    }

    const h = await headers();
    const outcome = await zitadelTotp({
      authRequestId: input.authRequestId,
      sessionId: input.sessionId,
      sessionToken: input.sessionToken,
      code: trimmed,
      workspaceTenant: claims.tenant_id,
      userAgent: h.get("user-agent") ?? undefined,
      forwardedFor: h.get("x-forwarded-for") ?? undefined,
    });

    return await mapZitadelOutcome(outcome, claims.tenant_id, claims.multiple_tenants);
  } catch (err) {
    return fail(err);
  }
}

/**
 * finishZitadelGoogleSignIn is the Google-through-Zitadel counterpart of
 * `signInWithZitadel`: instead of a login-name/password pair, the caller
 * (app/auth/idp/finish/route.ts) hands over the intent id/token Zitadel
 * appended to its redirect back to the browser, plus the `workspaceTenant`
 * it resolved from the request host (see lib/auth/tenant-host.ts) — the
 * merchant Google identity is not known until auth-bff resolves the
 * intent, so unlike the password path there is no email to run
 * `resolveWorkspaceTenant` against ahead of the call.
 *
 * Reuses `mapZitadelOutcome`, the exact same session-minting path
 * `signInWithZitadel`/`confirmZitadelTotp` use — a completed sign-in here
 * mints `m8_session` (or a callback_url to auth-bff's own OIDC finalize)
 * exactly the way a password sign-in does, never a second cookie-minting
 * path. `multipleTenants` is always false: this path never offers a
 * tenant picker — the host already picked one tenant to try, and
 * auth-bff's FGA membership check independently confirms or rejects it.
 *
 * `intentId`/`intentToken` are the ONLY inputs identity is derived from —
 * see zitadelIdpFinish's doc and idpFinishRequest.User's doc on
 * auth-bff's side for why the `user` query param Zitadel's redirect can
 * carry is never read, here or anywhere upstream of this call.
 */
export interface FinishZitadelGoogleSignInInput {
  authRequestId: string;
  intentId: string;
  intentToken: string;
  workspaceTenant: string;
}

export async function finishZitadelGoogleSignIn(
  input: FinishZitadelGoogleSignInInput,
): Promise<Result<SignInSuccess>> {
  try {
    const h = await headers();
    const outcome = await zitadelIdpFinish({
      authRequestId: input.authRequestId,
      intentId: input.intentId,
      intentToken: input.intentToken,
      workspaceTenant: input.workspaceTenant,
      userAgent: h.get("user-agent") ?? undefined,
      forwardedFor: h.get("x-forwarded-for") ?? undefined,
    });

    return await mapZitadelOutcome(outcome, input.workspaceTenant, false);
  } catch (err) {
    return fail(err);
  }
}

/**
 * mapZitadelOutcome forwards whatever cookies auth-bff minted (none, for
 * a step-up outcome) and maps a `LoginOutcome` onto the existing
 * `Result<SignInSuccess>` shape, shared by `signInWithZitadel` and
 * `confirmZitadelTotp`.
 */
async function mapZitadelOutcome(
  outcome: LoginOutcome & { setCookies: string[] },
  tenantId: string,
  multipleTenants: boolean,
): Promise<Result<SignInSuccess>> {
  await applySetCookies(outcome.setCookies);

  switch (outcome.kind) {
    case "complete":
      return {
        ok: true,
        data: {
          tenantId,
          multipleTenants,
          mfaRequired: false,
          emailOtpRequired: false,
          ...(outcome.callbackUrl ? { callbackUrl: outcome.callbackUrl } : {}),
        },
      };
    case "mfa_required":
      return {
        ok: true,
        data: { tenantId, multipleTenants, mfaRequired: true, emailOtpRequired: false },
      };
    case "email_otp_required":
      return {
        ok: true,
        data: { tenantId, multipleTenants, mfaRequired: false, emailOtpRequired: true },
      };
    case "totp_required": {
      const zitadelTenantCode = mintZitadelTotpCode(
        { tenant_id: tenantId, multiple_tenants: multipleTenants },
        SESSION_ENCRYPT_KEY,
        ZITADEL_TOTP_CODE_TTL_SECONDS,
      );
      return {
        ok: true,
        data: {
          tenantId,
          multipleTenants,
          mfaRequired: false,
          emailOtpRequired: false,
          totpRequired: true,
          zitadelSessionId: outcome.sessionId,
          zitadelSessionToken: outcome.sessionToken,
          zitadelTenantCode,
        },
      };
    }
    case "handoff":
      return {
        ok: true,
        data: {
          tenantId,
          multipleTenants,
          mfaRequired: false,
          emailOtpRequired: false,
          handoffUrl: outcome.handoffUrl,
        },
      };
    default: {
      const _exhaustive: never = outcome;
      throw new Error(`unhandled LoginOutcome kind: ${JSON.stringify(_exhaustive)}`);
    }
  }
}

/**
 * confirmMFALogin finishes a two-factor sign-in. The client already
 * has the m8_mfa_pending cookie from the preceding signIn call; we
 * forward it to auth-bff /auth/mfa-challenge along with the 6-digit
 * code. On success auth-bff mints the real session cookie which we
 * set on the response.
 */
export async function confirmMFALogin(
  code: string,
): Promise<Result<{ tenantId: string }>> {
  try {
    if (!code.trim()) {
      return {
        ok: false,
        code: "invalid_code",
        message: "Enter the 6-digit code from your authenticator app.",
      };
    }
    const c = await cookies();
    const cookieHeader = c
      .getAll()
      .map((x) => `${x.name}=${x.value}`)
      .join("; ");
    const result = await completeMFAChallenge(code.trim(), cookieHeader);
    for (const raw of result.setCookies) {
      const parsed = parseSetCookie(raw);
      if (parsed) {
        c.set({
          name: parsed.name,
          value: parsed.value,
          path: parsed.path ?? "/",
          domain: parsed.domain,
          httpOnly: parsed.httpOnly,
          secure: parsed.secure,
          sameSite: "lax",
          maxAge: parsed.maxAge,
        });
      }
    }
    return { ok: true, data: { tenantId: result.tenant_id } };
  } catch (err) {
    return fail(err);
  }
}

/**
 * confirmEmailOTPLogin finishes a sign-in that auth-bff challenged with
 * an emailed code. Mirrors confirmMFALogin: the browser already holds
 * the pending cookie from signIn, we forward it with the code, and set
 * whatever cookies auth-bff mints in return.
 */
export async function confirmEmailOTPLogin(
  code: string,
): Promise<Result<{ tenantId: string }>> {
  try {
    const trimmed = code.trim();
    if (!trimmed) {
      return {
        ok: false,
        code: "invalid_code",
        message: "Enter the 6-digit code we emailed you.",
      };
    }
    const c = await cookies();
    const cookieHeader = c
      .getAll()
      .map((x) => `${x.name}=${x.value}`)
      .join("; ");
    const result = await completeEmailOTPChallenge(trimmed, cookieHeader);
    for (const raw of result.setCookies) {
      const parsed = parseSetCookie(raw);
      if (parsed) {
        c.set({
          name: parsed.name,
          value: parsed.value,
          path: parsed.path ?? "/",
          domain: parsed.domain,
          httpOnly: parsed.httpOnly,
          secure: parsed.secure,
          sameSite: "lax",
          maxAge: parsed.maxAge,
        });
      }
    }
    return { ok: true, data: { tenantId: result.tenant_id } };
  } catch (err) {
    return fail(err);
  }
}

/** resendEmailOTPCode asks for a fresh emailed code on the same pending
 *  session. A 429 here means the rate limit tripped — waiting is the
 *  only way out, so the message must not tell the user to retry. */
export async function resendEmailOTPCode(): Promise<Result<null>> {
  try {
    const c = await cookies();
    const cookieHeader = c
      .getAll()
      .map((x) => `${x.name}=${x.value}`)
      .join("; ");
    await resendEmailOTP(cookieHeader);
    return { ok: true, data: null };
  } catch (err) {
    return fail(err);
  }
}

// applySetCookies forwards every Set-Cookie header auth-bff emitted to the
// browser response, exactly the way `signIn` has always done inline. Used
// by the Zitadel path (signInWithZitadel / confirmZitadelTotp via
// mapZitadelOutcome) — left as a standalone helper rather than folded into
// the GIP path's existing inline blocks so confirmMFALogin and
// confirmEmailOTPLogin are untouched.
async function applySetCookies(setCookies: string[]): Promise<void> {
  if (!setCookies.length) return;
  const c = await cookies();
  for (const raw of setCookies) {
    const parsed = parseSetCookie(raw);
    if (parsed) {
      c.set({
        name: parsed.name,
        value: parsed.value,
        path: parsed.path ?? "/",
        domain: parsed.domain,
        httpOnly: parsed.httpOnly,
        secure: parsed.secure,
        sameSite: "lax",
        maxAge: parsed.maxAge,
      });
    }
  }
}

// parseSetCookie pulls the bits next/headers cookies().set() needs out of
// a Set-Cookie header. Minimal parser — only the attributes auth-bff emits.
function parseSetCookie(raw: string): {
  name: string;
  value: string;
  path?: string;
  domain?: string;
  httpOnly: boolean;
  secure: boolean;
  maxAge?: number;
} | null {
  const parts = raw.split(";").map((p) => p.trim());
  const [first, ...attrs] = parts;
  if (!first || !first.includes("=")) return null;
  const eq = first.indexOf("=");
  const name = first.slice(0, eq);
  const value = first.slice(eq + 1);

  const out = {
    name,
    value,
    httpOnly: false,
    secure: false,
  } as {
    name: string;
    value: string;
    path?: string;
    domain?: string;
    httpOnly: boolean;
    secure: boolean;
    maxAge?: number;
  };
  for (const attr of attrs) {
    const lower = attr.toLowerCase();
    if (lower === "httponly") out.httpOnly = true;
    else if (lower === "secure") out.secure = true;
    else if (lower.startsWith("path=")) out.path = attr.slice(5);
    else if (lower.startsWith("domain=")) out.domain = attr.slice(7);
    else if (lower.startsWith("max-age=")) out.maxAge = parseInt(attr.slice(8), 10);
  }
  return out;
}
