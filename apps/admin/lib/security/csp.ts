/**
 * Admin Content-Security-Policy.
 *
 * script-src is nonce-based rather than 'unsafe-inline': the nonce is
 * minted per request in middleware, Next stamps it onto its own script
 * tags, and 'strict-dynamic' extends that trust to the sign-in SDKs the
 * app injects via document.createElement (lib/gip/google-gsi.ts,
 * lib/gip/apple-js.ts). CSP3 browsers ignore the host list below once
 * 'strict-dynamic' is present; it stays as the CSP2 fallback.
 */
export function buildCsp(nonce: string, env = process.env.NODE_ENV): string {
  // Next's dev server compiles with eval for HMR; a production build
  // never needs it, so the relaxation stays out of prod.
  const devEval = env === "development" ? " 'unsafe-eval'" : "";
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${devEval} https://accounts.google.com/gsi/client https://appleid.cdn-apple.com https://analytics.tesserix.app`,
    "style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style",
    "img-src 'self' data: blob: https:",
    "font-src 'self' data:",
    "connect-src 'self' https: wss:",
    "frame-ancestors 'none'",
    // GSI renders the button + One-Tap UI inside an iframe served from
    // accounts.google.com/gsi/. Apple's JS SDK injects an iframe from
    // appleid.apple.com to orchestrate the sign-in popup.
    "frame-src 'self' https://accounts.google.com/gsi/ https://appleid.apple.com",
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
