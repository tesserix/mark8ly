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

// Google-through-Zitadel is phase 3c-2 and does not exist yet. Until it
// does, the storefront must not offer any Google sign-in/sign-up/link
// control while running against Zitadel — that control would still send
// the customer's credential through GIP, the identity store we are
// migrating off.
export function isGoogleSignInOffered(): boolean {
  return getAuthProvider() === "gip";
}
