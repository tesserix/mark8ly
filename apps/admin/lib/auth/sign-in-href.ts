import { isValidSlugReturnUrl } from "./host-policy";

/**
 * signInHref builds the "back to sign in" target for the password-reset
 * and forgot-password pages.
 *
 * Those pages render on the CANONICAL admin host (platform-api mails
 * `{admin}/reset-password?oobCode=…`), and middleware hard-404s canonical
 * `/login` unless it carries a returnUrl pointing at a real slug-admin
 * host — that gate is deliberate anti-phishing and stays. A bare
 * `href="/login"` therefore dead-ends every merchant who clicks it.
 *
 * We cannot synthesize a returnUrl here: password reset is cross-tenant
 * (see RequestPasswordReset — "no tenant_id to forward"), and a merchant
 * may belong to several stores, so there is no single slug to guess.
 *
 * So: link only when the page was reached with a returnUrl we can hand
 * straight back to middleware. Otherwise return null and let the caller
 * render guidance rather than a link that 404s.
 */
export function signInHref(returnUrl: string | null | undefined): string | null {
  if (!isValidSlugReturnUrl(returnUrl)) return null;
  return `/login?returnUrl=${encodeURIComponent(returnUrl as string)}`;
}
