"use server";

// Server action that starts the admin app's merchant Google sign-in flow
// through Zitadel. Called from SignInForm instead of it calling auth-bff
// directly: AUTH_BFF_URL and MARKETPLACE_INTERNAL_AUTH_SECRET are
// server-only config (see @/lib/auth/auth-bff's file header) that must
// never reach the browser bundle.
//
// This module exports exactly one function, and it is async — Next.js
// strips any non-async runtime export from a "use server" module (see
// apps/storefront/app/auth/idp/actions.ts's file header for the build
// break this caused in an earlier phase), so any future addition here
// must stay async or move to lib/.

import { headers } from "next/headers";
import { AuthBffError, startZitadelIDPIntent } from "@/lib/auth/auth-bff";
import { buildAdminGoogleReturnUrl } from "@/lib/auth/google-sign-in-admin";

export type StartAdminGoogleSignInResult =
  | { ok: true; authUrl: string }
  | { ok: false; message: string };

/**
 * startAdminGoogleSignIn resolves this request's own host, builds the
 * /auth/idp/finish return URL (carrying `auth_request_id` so the finish
 * route can hand it back to auth-bff's idp/finish — see
 * buildAdminGoogleReturnUrl's doc), and starts a Zitadel IDP intent for
 * Google via auth-bff.
 *
 * `authRequestId` must already exist — the caller only reaches this once
 * /login/authorize has round-tripped through Zitadel's own /authorize
 * (see app/login/authorize/route.ts) — so a missing value here means the
 * page state is stale rather than anything auth-bff can help with; this
 * function refuses rather than sending an empty auth_request_id.
 *
 * Returns a result rather than throwing so the caller can render a
 * truthful, generic message instead of an internal error string — see
 * the phase brief's "never render an internal error string to a user"
 * constraint. The detail is logged server-side only.
 */
export async function startAdminGoogleSignIn(
  authRequestId: string,
): Promise<StartAdminGoogleSignInResult> {
  if (!authRequestId) {
    console.error("startAdminGoogleSignIn: called with no auth_request_id");
    return {
      ok: false,
      message: "Please refresh the page and try again.",
    };
  }

  const h = await headers();
  const forwardedHost = h.get("x-forwarded-host") ?? h.get("host") ?? "";
  if (!forwardedHost) {
    console.error("startAdminGoogleSignIn: no request host available");
    return {
      ok: false,
      message: "Could not start Google sign-in. Please try again.",
    };
  }

  const returnUrl = buildAdminGoogleReturnUrl(forwardedHost, authRequestId);

  try {
    const authUrl = await startZitadelIDPIntent(returnUrl);
    return { ok: true, authUrl };
  } catch (err) {
    if (err instanceof AuthBffError) {
      console.error("startAdminGoogleSignIn: auth-bff rejected the request", err.code);
    } else {
      console.error("startAdminGoogleSignIn: unexpected error", err);
    }
    return {
      ok: false,
      message: "Google sign-in is temporarily unavailable. Please try again shortly.",
    };
  }
}
