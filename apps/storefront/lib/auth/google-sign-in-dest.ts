// The small, fixed set of post-sign-in destinations the storefront's
// Zitadel Google flow is allowed to land on.
//
// Zitadel's IDP-intent redirect carries only what we hand it as
// successUrl/failureUrl (see idpintent.go's StartIDPIntent) plus whatever
// query params Zitadel itself appends (id/token, or id/error/
// error_description). To get back to the RIGHT page after finishing —
// /account for sign-in/sign-up, /account/security for the "link Google"
// control — apps/storefront/app/auth/idp/actions.ts embeds a `dest` query
// param in the return_url it sends to idp/start, and the finish route
// reads it back.
//
// That `dest` value round-trips through a URL the browser follows, so it
// must be constrained to a fixed allowlist rather than trusted as an
// arbitrary path — this is the SAME open-redirect discipline
// ValidateReturnURL applies on the auth-bff side, just for the one hop
// auth-bff's allowlist doesn't cover (the query param riding inside the
// URL it already validated).
export const GOOGLE_SIGNIN_DESTS = ["/account", "/account/security"] as const;

export type GoogleSignInDest = (typeof GOOGLE_SIGNIN_DESTS)[number];

export function isGoogleSignInDest(value: unknown): value is GoogleSignInDest {
  return (
    typeof value === "string" &&
    (GOOGLE_SIGNIN_DESTS as readonly string[]).includes(value)
  );
}
