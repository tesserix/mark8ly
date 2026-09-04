"use server";

// Phase P — accept-invite server actions.
//
// Two actions, one per identity provider, branched on
// `publicConfig.authProvider` by the form (never here — a server action
// cannot see which provider the browser rendered against any more
// cheaply than the caller can):
//
//   - acceptInvite            GIP.     Unchanged. Still the fallback.
//   - acceptInviteWithZitadel Zitadel. See its own doc; issue #679.
//
// The GIP flow, described below, is left exactly as it was.
//
// The client gets the user to a GIP id_token via one of three paths
// on the AcceptInviteForm:
//
//   a) "I already have a Mark8ly account" → signInWithPassword
//   b) "Continue with Google"              → signInWithGoogle
//   c) "Create a new account"              → signUp (new GIP user)
//
// Whichever path is picked hands us {idToken, uid, verifiedEmail}.
// We then:
//   1. Call platform-api accept — writes the FGA role tuple, marks
//      the invitation accepted, returns the tenant_id.
//   2. Auto-login against auth-bff so a session cookie exists.
//   3. Switch that session to the invitation's tenant.
//   4. Return the tenant_id + role so the client can redirect.
//
// Errors from any step map to a discriminated-union result that the
// client can surface inline.

import { cookies } from "next/headers";

import {
  acceptInvitation,
  PlatformApiError,
} from "@/lib/api/platform-api";
import { autoLogin, AuthBffError } from "@/lib/auth/auth-bff";
import { publicConfig } from "@/lib/config";

export interface AcceptInviteInput {
  token: string;
  idToken: string;
  uid: string;
  verifiedEmail: string;
}

export type AcceptInviteResult =
  | { ok: true; tenantId: string }
  | { ok: false; code: string; message: string };

/**
 * Where the Zitadel path sends the invitee once the accept succeeded.
 *
 * Not a shortcut: /login/authorize is the ONLY way to obtain a Zitadel
 * `auth_request_id`, and every Zitadel sign-in call in this app
 * (`signInWithZitadel`, `confirmZitadelTotp`, `finishZitadelGoogleSignIn`)
 * requires one. Zitadel's login-client model redirects the browser to a
 * fixed, instance-configured login URI (`/login`), so the accept page
 * cannot be handed an auth request of its own — it has to hand the
 * browser to the real login flow. The invitee re-enters the password
 * they just chose; the alternative is stashing a password somewhere to
 * replay it, which is not a trade worth making.
 */
const ZITADEL_SIGN_IN_URL = `/login/authorize?returnUrl=${encodeURIComponent("/dashboard")}`;

/**
 * The message shown when platform-api could not finish provisioning the
 * invitee (Zitadel user, admin project grant, or the FGA tuples).
 *
 * Only used when platform-api sent no message of its own. Its message is
 * preferred because it is written against the failure that actually
 * happened; this is the floor, not the ceiling. What must never happen
 * is `provisioning_failed` collapsing into "Something went wrong" —
 * a half-provisioned teammate is invisible until they try to sign in,
 * and the invitation is still pending, so "open the link again" is
 * genuinely the fix.
 */
const PROVISIONING_FAILED_FALLBACK =
  "We couldn't finish setting up your account. Your invite is still valid — open the invitation link again to retry, and contact the person who invited you if it keeps failing.";

export interface AcceptInviteWithZitadelInput {
  token: string;
  /** The invitation's email. Normalised to lowercase before it leaves
   *  this action: every email-keyed FGA tuple is lowercased, and the
   *  Zitadel login path resolves membership by email, so a
   *  `Staff@Example.com` sent verbatim would later miss its own
   *  membership and produce the misleading "no store found". */
  email: string;
  password: string;
}

export type AcceptInviteWithZitadelResult =
  | { ok: true; tenantId: string; signInUrl: string }
  | { ok: false; code: string; message: string };

function fail(err: unknown): {
  ok: false;
  code: string;
  message: string;
} {
  if (err instanceof PlatformApiError || err instanceof AuthBffError) {
    if (err.code === "provisioning_failed") {
      // `HTTP 500` is what PlatformApiError synthesizes when the
      // response carried no `message` — it is a stand-in for the
      // absence of one, not copy, and must not reach the invitee.
      const supplied = err.message?.trim() ?? "";
      const usable = supplied && !/^HTTP \d+$/.test(supplied);
      return {
        ok: false,
        code: err.code,
        message: usable ? supplied : PROVISIONING_FAILED_FALLBACK,
      };
    }
    return { ok: false, code: err.code, message: err.message };
  }
  return {
    ok: false,
    code: "unknown",
    message: err instanceof Error ? err.message : String(err),
  };
}

/**
 * acceptInviteWithZitadel is the Zitadel-path counterpart of
 * `acceptInvite`. Three things it deliberately does NOT do:
 *
 *   1. No GIP sign-up. Under Zitadel, platform-api's accept endpoint
 *      creates the account — that is the whole point of #679. Creating
 *      a GIP user here is the bug, not the flow.
 *   2. No `uid`. The invitee has no provider account at this point, so
 *      there is no id to send; platform-api's Accept no longer requires
 *      one when a provisioner is wired.
 *   3. No `autoLogin` / no id_token. auth-bff's /auth/auto-login verifies
 *      a GIP id_token; there is none on this path. The invitee is sent
 *      to the real Zitadel login flow instead (see ZITADEL_SIGN_IN_URL).
 */
export async function acceptInviteWithZitadel(
  input: AcceptInviteWithZitadelInput,
): Promise<AcceptInviteWithZitadelResult> {
  try {
    const accepted = await acceptInvitation({
      token: input.token,
      verified_email: input.email.trim().toLowerCase(),
      password: input.password,
    });
    return {
      ok: true,
      tenantId: accepted.tenant_id,
      signInUrl: ZITADEL_SIGN_IN_URL,
    };
  } catch (err) {
    return fail(err);
  }
}

export async function acceptInvite(
  input: AcceptInviteInput,
): Promise<AcceptInviteResult> {
  try {
    // 1. Write the FGA tuple + mark the invitation accepted. Returns
    // the tenant id we need for steps 2 and 3.
    const accepted = await acceptInvitation({
      token: input.token,
      uid: input.uid,
      verified_email: input.verifiedEmail,
    });

    // 2. Mint an initial session cookie on the invitee's NEW tenant.
    // If this is a brand-new GIP user, auto-login will retry against
    // FGA until the owner/member tuple propagates — but we just wrote
    // it synchronously in step 1, so it's already there.
    const login = await autoLogin({
      idToken: input.idToken,
      expectedTenantId: publicConfig.gipTenantId,
      workspaceTenant: accepted.tenant_id,
    });
    if (login.setCookies.length) {
      const c = await cookies();
      for (const raw of login.setCookies) {
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

    // 3. The auto-login already set the session to accepted.tenant_id
    // (we passed workspace_tenant). No need to call switch-tenant.
    return { ok: true, tenantId: accepted.tenant_id };
  } catch (err) {
    return fail(err);
  }
}

// Local duplicate of the parseSetCookie helper shared across login
// and pick-tenant. Will hoist to lib/ once a fourth caller shows up.
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
    else if (lower.startsWith("max-age="))
      out.maxAge = parseInt(attr.slice(8), 10);
  }
  return out;
}
