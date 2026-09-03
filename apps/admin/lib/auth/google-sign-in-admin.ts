// Pure branching logic behind the admin app's merchant "Continue with
// Google" flow through Zitadel — kept out of app/auth/idp/actions.ts,
// app/auth/idp/finish/route.ts and components/auth/SignInForm.tsx so it
// stays covered by apps/admin's vitest config the same way
// apps/storefront/lib/auth/google-sign-in.ts does for the customer flow.
// (admin's vitest.config.ts `test.include` picks up every
// **/*.{test,spec}.{ts,tsx}` regardless of directory — component-level
// logic IS test-executed here, unlike apps/storefront's `components/**`
// exclusion — but the `coverage.include` allowlist that enforces the
// 80% threshold only lists products/** paths, so nothing under lib/auth/
// or components/auth/ is coverage-gated either way. Extracting here still
// follows the established pattern and keeps this logic unit-testable
// without rendering the form or mocking `fetch`.)

/**
 * buildAdminGoogleReturnUrl builds the `return_url` auth-bff's
 * POST /auth/zitadel/idp/start hands to Zitadel as both successUrl and
 * failureUrl for a merchant Google sign-in.
 *
 * Deliberately built from the CURRENT request's own host (dynamic), not a
 * separately-configured canonical value — unlike apps/storefront, which
 * must resolve a per-tenant store host from a slug before it can build a
 * return URL at all (see startCustomerGoogleSignIn / resolveStore), admin
 * needs no such lookup: whichever admin host the merchant is signing in
 * on IS the host Zitadel should return to, and auth-bff's ADMIN return-url
 * allowlist (host + suffix entries — see returnurl.go and
 * newZitadelHandlers) is what actually enforces which hosts are legitimate,
 * not this function. "fixed", per the phase brief, describes this
 * function's output for a given host: exactly one path
 * (`/auth/idp/finish`), carrying exactly one embedded value
 * (`auth_request_id`) — never a per-tenant slug or store lookup the way
 * the storefront's equivalent needs.
 *
 * `authRequestId` rides in the query string so the finish route can read
 * it back once Zitadel appends its own `id`/`token` (or `error`) params —
 * Zitadel does not know about or forward anything from this app's own
 * OIDC login-client flow, so this is the only channel for it to survive
 * the round trip to Google and back.
 */
export function buildAdminGoogleReturnUrl(
  forwardedHost: string,
  authRequestId: string,
): string {
  const url = new URL(`https://${forwardedHost}/auth/idp/finish`);
  url.searchParams.set("auth_request_id", authRequestId);
  return url.toString();
}

/**
 * AdminGoogleErrorCode enumerates every distinct code the merchant Google
 * flow can redirect back to /login with. Kept as a type (not just `string`)
 * so `messageForAdminGoogleError`'s exhaustiveness is checkable, and so a
 * new code added to the finish route without a matching message here is a
 * compile error, not a silently generic one.
 */
export type AdminGoogleErrorCode =
  | "google_sign_in_unavailable"
  | "invalid_request"
  | "store_not_found"
  | "no_admin_account"
  | "unexpected_idp"
  | "email_not_verified"
  | "email_ambiguous"
  | "invalid_return_url"
  | "invalid_intent"
  | "zitadel_unavailable"
  | "step_up_unsupported"
  | "internal_error";

const KNOWN_CODES: ReadonlySet<string> = new Set<AdminGoogleErrorCode>([
  "google_sign_in_unavailable",
  "invalid_request",
  "store_not_found",
  "no_admin_account",
  "unexpected_idp",
  "email_not_verified",
  "email_ambiguous",
  "invalid_return_url",
  "invalid_intent",
  "zitadel_unavailable",
  "step_up_unsupported",
  "internal_error",
]);

export function isAdminGoogleErrorCode(code: string): code is AdminGoogleErrorCode {
  return KNOWN_CODES.has(code);
}

/**
 * messageForAdminGoogleError maps a finish-flow outcome code to copy the
 * /login page shows the merchant. Every branch is a truthful, DISTINCT
 * message — see the phase brief's constraint that no outcome may imply
 * the Google credential itself was wrong (it never is: every failure
 * here is either a platform-side account/authorization decision or an
 * availability problem, never a bad password) and that an internal error
 * string must never reach the browser.
 *
 * `no_admin_account` gets the most care: the merchant path is link-only
 * (see auth-bff's idpFinish doc) and NEVER creates an account, so this
 * message must say plainly that no admin account exists for this Google
 * identity and point at how one is actually obtained — onboarding or an
 * invite — without suggesting a retry (retrying changes nothing) or
 * implying the sign-in itself failed technically (it didn't: Google and
 * Zitadel both succeeded).
 */
export function messageForAdminGoogleError(code: string): string {
  const known = isAdminGoogleErrorCode(code) ? code : "internal_error";
  switch (known) {
    case "no_admin_account":
      return "There's no admin account for this Google identity yet. Merchant accounts are created during onboarding or by an invite from an existing store owner — not by signing in here.";
    case "email_not_verified":
      return "Google hasn't verified this account's email address, so we can't use it to sign in to a store. Verify your email with Google, or sign in with your email and password instead.";
    case "email_ambiguous":
      return "More than one account matches this email address. Please sign in with your email and password instead so we can be sure which account you mean.";
    case "unexpected_idp":
      return "That sign-in didn't come from Google. Please try again.";
    case "invalid_intent":
      return "That sign-in link expired. Please try Continue with Google again.";
    case "store_not_found":
      return "We couldn't tell which store this is for Google sign-in. Sign in from your store's own admin address, or use your email and password.";
    case "invalid_return_url":
    case "invalid_request":
      return "Something went wrong starting Google sign-in. Please try again.";
    case "step_up_unsupported":
      return "This account needs an extra verification step we can't complete through Google yet. Please sign in with your email and password instead.";
    case "google_sign_in_unavailable":
    case "zitadel_unavailable":
      return "Google sign-in is temporarily unavailable. Please try again shortly, or sign in with your email and password.";
    case "internal_error":
    default:
      return "Something went wrong signing in with Google. Please try again.";
  }
}
