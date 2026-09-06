// Shared plumbing for the Zitadel OIDC authorization-code flow used to
// obtain an `auth_request_id` for the login-client model (Task 4 of
// docs/superpowers/plans/2026-09-03-zitadel-phase3a-admin-frontend.md).
//
// Two routes share this: `app/login/authorize/route.ts` (kicks the
// flow off) and `app/auth/callback/route.ts` (completes it). Keeping
// the cookie names and PKCE/state generation in one place means the
// two routes cannot drift on what they read and write — a drift here
// is exactly how a CSRF check silently stops checking anything.

export const ZITADEL_STATE_COOKIE = "zt_auth_state";
export const ZITADEL_VERIFIER_COOKIE = "zt_pkce_verifier";
export const ZITADEL_RETURN_URL_COOKIE = "zt_return_url";

// Short-lived: the detour through Zitadel's /authorize and back to our
// own /login form is a handful of redirects, not a task that takes
// minutes. Ten minutes is headroom, not an invitation to leave the
// tab open overnight.
export const ZITADEL_FLOW_COOKIE_MAX_AGE_SECONDS = 600;

// Carries a merchant-facing Google sign-in outcome code across the
// /login/authorize -> Zitadel /authorize -> /login?authRequest=… hop.
//
// A query param cannot do this job: Zitadel builds that final /login URL
// itself from the app's configured login base URI and appends only its own
// `authRequest`, so anything this app puts on the way in is dropped. The
// returnUrl already rides a cookie for the same reason
// (ZITADEL_RETURN_URL_COOKIE) — this is that pattern, not a new one.
//
// Only a value that passes `isAdminGoogleErrorCode` is ever written here
// (see app/login/authorize/route.ts), so nothing free-text — least of all
// a provider's own error body — can reach the page through it.
export const ZITADEL_LOGIN_ERROR_COOKIE = "zt_login_error";

// Two minutes: the hop it survives is three redirects long. A Server
// Component render cannot clear a cookie, so this TTL is the only thing
// bounding how long a stale message could reappear on a /login reload —
// short enough that it cannot outlive the attempt it describes.
export const ZITADEL_LOGIN_ERROR_COOKIE_MAX_AGE_SECONDS = 120;

function base64url(bytes: Uint8Array): string {
  return Buffer.from(bytes).toString("base64url");
}

/** Cryptographically random, URL-safe token for the OAuth `state` param. */
export function generateState(): string {
  return base64url(crypto.getRandomValues(new Uint8Array(24)));
}

export interface PkcePair {
  verifier: string;
  challenge: string;
}

/**
 * generatePkcePair produces an RFC 7636 S256 code_verifier/code_challenge
 * pair. The verifier is stored in a cookie so it exists if a later
 * phase performs the token exchange; nothing in this phase reads it
 * back — code exchange is auth-bff's concern, not ours (see the
 * callback route's header comment) — but Zitadel's /authorize still
 * requires the challenge.
 */
export async function generatePkcePair(): Promise<PkcePair> {
  const verifier = base64url(crypto.getRandomValues(new Uint8Array(32)));
  const digest = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(verifier),
  );
  const challenge = base64url(new Uint8Array(digest));
  return { verifier, challenge };
}

export interface BuildAuthorizeUrlInput {
  issuer: string;
  clientId: string;
  redirectUri: string;
  state: string;
  codeChallenge: string;
}

/** Builds Zitadel's `/oauth/v2/authorize` URL for the login-client flow. */
export function buildZitadelAuthorizeUrl(input: BuildAuthorizeUrlInput): string {
  const url = new URL("/oauth/v2/authorize", input.issuer);
  url.searchParams.set("client_id", input.clientId);
  url.searchParams.set("redirect_uri", input.redirectUri);
  url.searchParams.set("response_type", "code");
  url.searchParams.set("scope", "openid");
  url.searchParams.set("state", input.state);
  url.searchParams.set("code_challenge", input.codeChallenge);
  url.searchParams.set("code_challenge_method", "S256");
  return url.toString();
}

/**
 * isTrustedZitadelHostedUrl checks a server-supplied `handoff_url` (see
 * `LoginOutcome`'s "handoff" case) against the one legitimate target for
 * it: Zitadel's own hosted login UI, at the configured issuer's origin.
 *
 * `sanitizeReturnUrl` doesn't fit here — it only allows mark8ly.com (and
 * localhost), and a handoff by definition leaves that origin for
 * Zitatel's own domain. This is a narrower, purpose-built check rather
 * than a second general-purpose sanitiser: exact-origin match against
 * `issuer`, nothing else.
 */
export function isTrustedZitadelHostedUrl(url: string, issuer: string): boolean {
  if (!issuer) return false;
  try {
    const target = new URL(url);
    if (target.protocol !== "https:") return false;
    return target.origin === new URL(issuer).origin;
  } catch {
    return false;
  }
}

/**
 * isTrustedCallbackUrl checks a server-supplied `callback_url` (see
 * `LoginOutcome`'s "complete" case) against the one legitimate target for
 * it: this application's own `/auth/callback` route, on this app's own
 * origin. Unlike `handoffUrl`, which deliberately leaves this origin for
 * Zitadel's hosted UI, `callbackUrl` should never point anywhere else —
 * auth-bff only ever hands this route's own URL back. A sibling check
 * rather than reusing `isTrustedZitadelHostedUrl`, which validates a
 * different destination (Zitadel's origin, not this app's).
 */
export function isTrustedCallbackUrl(url: string, appOrigin: string): boolean {
  if (!appOrigin) return false;
  try {
    const target = new URL(url);
    return target.origin === new URL(appOrigin).origin;
  } catch {
    return false;
  }
}
