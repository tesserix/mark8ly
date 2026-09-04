// Maps the outcome codes apps/storefront/app/create-account/actions.ts's
// registerCustomer and verifyCustomerEmail can return, to a truthful,
// shopper-facing message. Mirrors lib/auth/google-idp-error-messages.ts's
// shape and discipline exactly — see that file's header for the general
// rationale — but this is a SEPARATE mapping, not a shared one: the
// sign-up flow's `email_taken` means something different from the Google
// finish route's (a VERIFIED existing account here, vs. an unverified one
// there), so the two must not share copy.
//
// Two codes carry the phase brief's sharpest constraint:
//   - email_taken is PERMANENT for that address (a verified account
//     already owns it) — the message must be actionable (sign in instead,
//     or contact support) and must never suggest retrying registration.
//   - verification_email_failed is the OPPOSITE: register rolled the new
//     account back, so the address is free again and retrying genuinely
//     works — the message must say so, not read like a dead end.
//
// `code` here is always one of a small, fixed set of strings
// registerCustomerAccount/verifyCustomerEmailCode put in an outcome value
// (see lib/auth/auth-bff-customer.ts) — never raw text from auth-bff or
// Zitadel — so looking it up here never risks rendering an internal error
// string to the shopper.
const MESSAGES: Record<string, string> = {
  email_taken:
    "An account with this email address already exists. Sign in instead, or contact support if you believe this is a mistake.",
  email_ambiguous:
    "We found more than one account for this email and can't tell which one to use. Please contact support for help.",
  weak_password:
    "That password doesn't meet our security requirements. Try a longer password with a mix of letters, numbers, and symbols.",
  invalid_verification_code:
    "That code is incorrect or has expired. Check your email and try entering it again.",
  email_not_verified:
    "This account's email address hasn't been verified yet. Check your email for the verification code.",
  verification_email_failed:
    "We couldn't send your verification email, so your account wasn't created. Please try creating your account again.",
  zitadel_unavailable:
    "Account creation is temporarily unavailable. Please try again shortly.",
  invalid_request: "Something went wrong with that request. Please try again.",
};

const DEFAULT_MESSAGE = "Something went wrong creating your account. Please try again.";

/**
 * customerSignupErrorMessage returns the truthful, distinct message for a
 * known register/verify-email outcome code, or a generic (but still
 * honest) fallback for anything else. Never echoes the raw code.
 */
export function customerSignupErrorMessage(code: string): string {
  return MESSAGES[code] ?? DEFAULT_MESSAGE;
}
