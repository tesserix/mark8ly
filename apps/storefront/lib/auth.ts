/**
 * Auth helpers for customer login/logout URL construction and session
 * cookie detection. Used by the root layout to hydrate CustomerAuthProvider.
 *
 * Sign-in and sign-out are hosted on the storefront itself:
 *   /sign-in       — client-side GIP form → server action sets cookie
 *   /create-account — same flow but with GIP signUp
 *   /sign-out      — clears the session cookie
 *
 * The session cookie (`mp_customer_session`) is a base64-encoded JSON
 * payload: `{ uid, email, store_slug, store_id, tenant_id }`. Set by
 * the sign-in server action, read by `decodeSession` below.
 */

// Re-export the session type + decode from the shared session module.
export { decodeSession, type CustomerSession } from "@/lib/session";

export function buildLoginUrl(_redirectUri: string): string {
  return "/sign-in";
}

export function buildLogoutUrl(_redirectUri: string): string {
  return "/sign-out";
}

export function hasSessionCookie(cookieHeader: string | null): boolean {
  if (!cookieHeader) return false;
  return cookieHeader.includes("mp_customer_session=");
}
