// Maps the error codes apps/storefront/app/auth/idp/finish/route.ts can
// redirect to /sign-in?error=<code> with, to a truthful, shopper-facing
// message. Each outcome gets its own copy — the phase brief is explicit
// that these must never be worded like a wrong-credential retry loop,
// and that email_taken in particular (a permanent lockout — the email
// already belongs to an account that was never verified) must not read
// like email_not_verified (a fixable, retry-after-verifying state) or a
// generic transient failure.
//
// `code` here is always one of a small, fixed set of strings the route
// itself put in the URL (never raw text from auth-bff or Zitadel) — see
// the route's file header — so looking it up here never risks rendering
// an internal error string to the shopper.
const MESSAGES: Record<string, string> = {
  email_not_verified:
    "Google hasn't verified this account's email address yet. Verify it with Google, then try Continue with Google again.",
  email_taken:
    "This email already belongs to an account that hasn't been verified yet. Continuing with Google won't fix that — please contact support to finish setting up that account.",
  email_ambiguous:
    "We found more than one account for this email and couldn't tell which one to sign you into. Please try again in a moment, or contact support if this keeps happening.",
  unexpected_idp:
    "Something went wrong verifying your Google sign-in. Please try again.",
  invalid_intent:
    "That Google sign-in link expired or was already used. Please try Continue with Google again.",
  invalid_return_url:
    "Something went wrong starting Google sign-in. Please try again.",
  zitadel_unavailable:
    "Sign-in is temporarily unavailable. Please try again shortly.",
  store_not_found: "Could not resolve this store. Please try again.",
  invalid_host: "Could not validate the host for sign-in. Please try again.",
  invalid_request: "Something went wrong with that Google sign-in link. Please try again.",
  google_sign_in_unavailable:
    "Google sign-in didn't complete. Please try again.",
};

const DEFAULT_MESSAGE =
  "Something went wrong signing in with Google. Please try again.";

/**
 * googleIdpErrorMessage returns the truthful, distinct message for a
 * known /auth/idp/finish error code, or a generic (but still honest)
 * fallback for anything else. Returns null for no code at all, so
 * callers can tell "no error to show" from "show the generic message".
 */
export function googleIdpErrorMessage(code: string | undefined | null): string | null {
  if (!code) return null;
  return MESSAGES[code] ?? DEFAULT_MESSAGE;
}
