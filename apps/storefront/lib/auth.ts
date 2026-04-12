/**
 * Auth helpers for customer login/logout URL construction and session
 * cookie detection. Used by the root layout to hydrate CustomerAuthProvider.
 *
 * Sign-in and sign-out are now hosted on the storefront itself:
 *   /sign-in  — client-side GIP form → server action → auth-bff
 *   /sign-out — server action clears the session cookie
 *
 * The URLs produced here are same-origin relative paths, so they
 * always work regardless of hostname or deployment config.
 */

const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8085";

export function buildLoginUrl(_redirectUri: string): string {
  return "/sign-in";
}

export function buildLogoutUrl(_redirectUri: string): string {
  // POST /auth/logout on auth-bff clears the session cookie. For now
  // we do a client-side redirect to a server action that does the
  // same. A dedicated /sign-out page is stubbed in — for now the
  // logout link calls the API directly via the CustomerAccountMenu.
  return "/sign-out";
}

/** AUTH_BFF_URL for server-side calls (server actions, middleware). */
export function getAuthBffUrl(): string {
  return AUTH_BFF_URL;
}

export function hasSessionCookie(cookieHeader: string | null): boolean {
  if (!cookieHeader) return false;
  return cookieHeader.includes("mp_customer_session=");
}
