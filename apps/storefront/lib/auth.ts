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

export interface CustomerSession {
  uid: string;
  email: string;
  store_slug: string;
  store_id: string;
  tenant_id: string;
}

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

/** Decode the `mp_customer_session` cookie value. Returns null on any
 *  parse failure so the layout can fall back to the anonymous state. */
export function decodeSession(cookieValue: string): CustomerSession | null {
  try {
    const json = Buffer.from(cookieValue, "base64").toString();
    const parsed = JSON.parse(json) as Partial<CustomerSession>;
    if (!parsed.uid || !parsed.email) return null;
    return {
      uid: parsed.uid,
      email: parsed.email,
      store_slug: parsed.store_slug ?? "",
      store_id: parsed.store_id ?? "",
      tenant_id: parsed.tenant_id ?? "",
    };
  } catch {
    return null;
  }
}
