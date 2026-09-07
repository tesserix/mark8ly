/**
 * Admin Content-Security-Policy.
 *
 * script-src is nonce-based rather than 'unsafe-inline': the nonce is
 * minted per request in middleware and Next stamps it onto its own
 * script tags. CSP3 browsers ignore the host list below once
 * 'strict-dynamic' is present; it stays as the CSP2 fallback.
 *
 * Google Identity Services and Apple's JS SDK are no longer allowlisted:
 * Google sign-in is a full-page redirect through Zitadel's IDP intent,
 * and nothing in this app injects a third-party auth script any more.
 */
export function buildCsp(nonce: string, env = process.env.NODE_ENV): string {
  // Next's dev server compiles with eval for HMR; a production build
  // never needs it, so the relaxation stays out of prod.
  const devEval = env === "development" ? " 'unsafe-eval'" : "";
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${devEval} https://analytics.tesserix.app`,
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: blob: https:",
    "font-src 'self' data:",
    "connect-src 'self' https: wss:",
    "frame-ancestors 'none'",
    "frame-src 'self'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
  ].join("; ");
}

/** Per-request nonce. crypto.getRandomValues is available on the Edge runtime. */
export function newNonce(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return btoa(String.fromCharCode(...bytes));
}
