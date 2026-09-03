/**
 * sanitizeReturnUrl accepts only URLs under mark8ly.com (and its
 * subdomains), plus localhost for local dev. Anything else — an
 * external host, a `javascript:` URL, or a protocol-relative
 * `//evil.com` — gets dropped to prevent open-redirect abuse.
 *
 * Extracted from `app/login/page.tsx` (where it originated) so the
 * Zitadel OIDC routes can apply the exact same policy to the
 * destination they read back from a cookie. A second, slightly
 * different sanitiser is exactly how an open redirect creeps in — see
 * the Task 4 brief under `.superpowers/sdd/`.
 */
export function sanitizeReturnUrl(
  raw: string | null | undefined,
): string | undefined {
  if (!raw) return undefined;
  try {
    const u = new URL(raw);
    if (u.protocol !== "https:" && u.protocol !== "http:") return undefined;
    if (
      u.hostname === "mark8ly.com" ||
      u.hostname.endsWith(".mark8ly.com") ||
      u.hostname === "localhost"
    ) {
      return u.toString();
    }
  } catch {
    // fall through
  }
  return undefined;
}
