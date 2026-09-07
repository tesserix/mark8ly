"use server";

// Phase P — accept-invite server action.
//
// The invitee chooses a password on AcceptInviteForm; platform-api's
// accept endpoint creates the Zitadel account, writes the FGA role
// tuple, marks the invitation accepted and returns the tenant_id. The
// invitee is then sent to the real Zitadel login flow.
//
// Errors map to a discriminated-union result the client surfaces inline.

import {
  acceptInvitation,
  PlatformApiError,
} from "@/lib/api/platform-api";
import { AuthBffError } from "@/lib/auth/auth-bff";

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
 * acceptInviteWithZitadel. Two things it deliberately does NOT do:
 *
 *   1. No `uid`. The invitee has no provider account at this point, so
 *      there is no id to send; platform-api's Accept no longer requires
 *      one when a provisioner is wired.
 *   2. No session mint. The invitee is sent to the real Zitadel login
 *      flow instead (see ZITADEL_SIGN_IN_URL).
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
