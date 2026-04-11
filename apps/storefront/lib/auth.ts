/**
 * Auth helpers for customer login/logout URL construction and session
 * cookie detection. Used by the root layout to hydrate CustomerAuthProvider.
 *
 * IMPORTANT: The URLs built here are serialized into React props by the
 * root layout and rendered as anchor hrefs in the customer nav. That
 * means they must be publicly reachable from the customer's browser —
 * a local-dev fallback like `http://localhost:8085` will visibly ship
 * into production and break the Sign in / Sign out links.
 *
 * In production we therefore refuse to return a localhost URL and fall
 * back to `"#"` instead (the same no-op value `CustomerAuthProvider`
 * uses for its default). The Sign in button becomes inert, but the
 * storefront stays up — and the server logs carry a loud warning so
 * ops can find and set `AUTH_BFF_URL` on the deployment.
 */

const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8085";
const IS_PROD = process.env.NODE_ENV === "production";

function isLocalhostUrl(url: string): boolean {
  return /^https?:\/\/(localhost|127\.0\.0\.1|0\.0\.0\.0)(:|\/|$)/.test(url);
}

let warnedOnceAboutAuthBff = false;
function warnAuthBffMisconfigured(): void {
  if (warnedOnceAboutAuthBff) return;
  warnedOnceAboutAuthBff = true;
  // eslint-disable-next-line no-console
  console.error(
    "[storefront] AUTH_BFF_URL points at a localhost address in " +
      "production. Sign in / sign out links have been disabled. " +
      "Set AUTH_BFF_URL on the storefront deployment to a publicly " +
      "reachable auth-bff origin (e.g. https://auth.mark8ly.com).",
  );
}

function safeAuthOrigin(): string | null {
  if (IS_PROD && isLocalhostUrl(AUTH_BFF_URL)) {
    warnAuthBffMisconfigured();
    return null;
  }
  return AUTH_BFF_URL;
}

export function buildLoginUrl(redirectUri: string): string {
  const origin = safeAuthOrigin();
  if (!origin) return "#";
  const params = new URLSearchParams({
    product: "mp-customer",
    redirect_uri: redirectUri,
  });
  return `${origin}/login?${params.toString()}`;
}

export function buildLogoutUrl(redirectUri: string): string {
  const origin = safeAuthOrigin();
  if (!origin) return "#";
  const params = new URLSearchParams({
    product: "mp-customer",
    redirect_uri: redirectUri,
  });
  return `${origin}/logout?${params.toString()}`;
}

export function hasSessionCookie(cookieHeader: string | null): boolean {
  if (!cookieHeader) return false;
  return cookieHeader.includes("mp_customer_session=");
}
