// Shared decision logic behind the storefront's three "Continue with
// Google" controls (CustomerSignInForm, CreateAccountForm,
// SecurityClient's "Add Google"). Lives under lib/** rather than inline
// in each component so it stays covered by apps/storefront's vitest
// config — components/** is not.
//
// Zitadel takes the return URL per request, so the browser goes straight
// from auth-bff's own Google IDP intent back to this store's own host
// (apps/storefront/app/auth/idp/finish/route.ts). resolveGoogleSignInUrl
// asks the startCustomerGoogleSignIn server action for that authUrl.

import { startCustomerGoogleSignIn } from "@/app/auth/idp/actions";
import type { GoogleSignInDest } from "@/lib/auth/google-sign-in-dest";

export type GoogleSignInIntent = "signin" | "signup" | "link";

export interface GoogleSignInArgs {
  storeSlug: string;
  intent: GoogleSignInIntent;
  dest: GoogleSignInDest;
  /** window.location.origin — passed in so this stays testable without a DOM. */
  origin: string;
}

export type ResolveGoogleSignInUrlResult =
  | { ok: true; url: string }
  | { ok: false; message: string };

/**
 * resolveGoogleSignInUrl mints the Zitadel authUrl a "Continue with
 * Google" click should send the browser to. Returns a result instead of
 * throwing so the caller can render a truthful message rather than an
 * unhandled rejection.
 *
 * `args.storeSlug` and `args.intent` are deliberately NOT passed to
 * startCustomerGoogleSignIn: Zitadel resolves the store from the
 * request's own host server-side (see app/auth/idp/actions.ts), so
 * storeSlug travels nowhere, and auth-bff's customer IDP-start endpoint
 * self-registers or signs in identically regardless of signin/signup
 * intent. The "link" intent value was only ever meaningful to the old
 * GIP trampoline shape — see SecurityClient, which does not offer
 * "Add Google" for exactly that reason.
 */
export async function resolveGoogleSignInUrl(
  args: GoogleSignInArgs,
): Promise<ResolveGoogleSignInUrlResult> {
  const result = await startCustomerGoogleSignIn(args.dest);
  if (!result.ok) return result;
  return { ok: true, url: result.authUrl };
}
