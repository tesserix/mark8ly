/**
 * Auth helpers for customer login/logout URL construction and session
 * cookie detection. Used by the root layout to hydrate CustomerAuthProvider.
 */

const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8085";

export function buildLoginUrl(redirectUri: string): string {
  const params = new URLSearchParams({
    product: "mp-customer",
    redirect_uri: redirectUri,
  });
  return `${AUTH_BFF_URL}/login?${params.toString()}`;
}

export function buildLogoutUrl(redirectUri: string): string {
  const params = new URLSearchParams({
    product: "mp-customer",
    redirect_uri: redirectUri,
  });
  return `${AUTH_BFF_URL}/logout?${params.toString()}`;
}

export function hasSessionCookie(cookieHeader: string | null): boolean {
  if (!cookieHeader) return false;
  return cookieHeader.includes("mp_customer_session=");
}
