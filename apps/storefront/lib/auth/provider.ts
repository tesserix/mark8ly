// Single source of truth for the storefront's identity-provider flag,
// shared by CustomerSignInForm, CreateAccountForm, and SecurityClient so
// the decision is testable in one place (this file lives under lib/**,
// which apps/storefront's vitest config covers — components/** is not).
//
// Matches apps/storefront/app/sign-in/actions.ts's AUTH_PROVIDER rule
// exactly: only the literal string "zitadel" switches behavior off the
// GIP/Identity Toolkit path. Unset, empty, or any other value or case
// (including "Zitadel" or "true") stays on GIP. Do not loosen this
// comparison — see provider.test.ts, which pins the wrong-case guard.
export function getAuthProvider(): "gip" | "zitadel" {
  return process.env.NEXT_PUBLIC_AUTH_PROVIDER === "zitadel"
    ? "zitadel"
    : "gip";
}

// Google-through-Zitadel (phase 3c-2) now exists on both providers for
// SIGN-IN/SIGN-UP: under GIP the control still goes through the
// mark8ly.com/auth/google trampoline exactly as before, and under
// Zitadel it drives auth-bff's own Google IDP intent instead (see
// @/lib/auth/google-sign-in). Kept as its own function, rather than
// inlining `true` at each call site, so a future gap in
// Google-through-Zitadel coverage (e.g. a third provider) has one place
// to express "is Google offered at all" again.
export function isGoogleSignInOffered(): boolean {
  return true;
}

// Whether SecurityClient's "Add Google" (account-linking) control may be
// offered. This is deliberately NOT the same question as
// isGoogleSignInOffered above: linking an existing, authenticated
// shopper's account to a Google identity needs an authenticated
// "link this provider to my existing account" backend endpoint, which
// does not exist under Zitadel today — auth-bff's customer IDP endpoint
// only self-registers/signs in. Offering the control under Zitadel would
// let "Add Google" silently switch a signed-in shopper to a brand-new
// account instead of linking one, so it must not be offered at all under
// that flag. Under GIP the existing link flow is unaffected.
export function isGoogleLinkOffered(): boolean {
  return getAuthProvider() !== "zitadel";
}
