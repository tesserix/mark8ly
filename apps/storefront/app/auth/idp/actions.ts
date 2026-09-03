"use server";

// Server action that starts the storefront's Zitadel Google sign-in flow.
//
// Called from the client (CustomerSignInForm, CreateAccountForm,
// SecurityClient) instead of those components calling auth-bff directly:
// AUTH_BFF_URL and MARKETPLACE_INTERNAL_AUTH_SECRET are server-only config
// (see @/lib/auth/auth-bff-customer's file header) that must never reach
// the browser bundle.
//
// This module exports exactly one function, and it is async — Next.js
// strips any non-async runtime export from a "use server" module (see the
// comment on @/lib/auth/customer-sign-in-result for the build break this
// caused in an earlier phase), so any future addition here must stay
// async or move to lib/.

import { headers } from "next/headers";
import {
  AuthBffCustomerError,
  startCustomerIDPIntent,
} from "@/lib/auth/auth-bff-customer";
import {
  isGoogleSignInDest,
  type GoogleSignInDest,
} from "@/lib/auth/google-sign-in-dest";

export type StartGoogleSignInResult =
  | { ok: true; authUrl: string }
  | { ok: false; message: string };

/**
 * startCustomerGoogleSignIn resolves this request's own host, builds the
 * /auth/idp/finish return URL (carrying `dest` so the finish route knows
 * where to land the browser afterwards), and starts a Zitadel IDP intent
 * for Google via auth-bff.
 *
 * Deliberately does NOT accept an arbitrary `dest` — only a value from
 * the fixed GOOGLE_SIGNIN_DESTS allowlist is embedded in the return URL,
 * so a caller cannot turn this into an open redirect by passing an
 * attacker-controlled destination through it.
 *
 * Returns a result rather than throwing so callers can render a
 * truthful, generic message instead of an internal error string — see
 * the phase brief's "never render an internal error string to a
 * shopper" constraint. The detail is logged server-side only.
 */
export async function startCustomerGoogleSignIn(
  dest: GoogleSignInDest,
): Promise<StartGoogleSignInResult> {
  const safeDest: GoogleSignInDest = isGoogleSignInDest(dest) ? dest : "/account";

  const h = await headers();
  const forwardedHost = h.get("x-forwarded-host") ?? h.get("host") ?? "";
  if (!forwardedHost) {
    console.error("startCustomerGoogleSignIn: no request host available");
    return {
      ok: false,
      message: "Could not start Google sign-in. Please try again.",
    };
  }

  const returnUrl = `https://${forwardedHost}/auth/idp/finish?dest=${encodeURIComponent(safeDest)}`;

  try {
    const authUrl = await startCustomerIDPIntent(returnUrl);
    return { ok: true, authUrl };
  } catch (err) {
    if (err instanceof AuthBffCustomerError) {
      console.error("startCustomerGoogleSignIn: auth-bff rejected the request", err.code);
    } else {
      console.error("startCustomerGoogleSignIn: unexpected error", err);
    }
    return {
      ok: false,
      message: "Google sign-in is temporarily unavailable. Please try again shortly.",
    };
  }
}
