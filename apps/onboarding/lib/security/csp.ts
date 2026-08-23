/**
 * Onboarding Content-Security-Policy.
 *
 * This app is mostly a statically prerendered marketing site, and
 * prerendered HTML cannot carry a per-request nonce — its script tags
 * would be blocked. So there are two policies:
 *
 *   - buildCsp: nonce-based, no 'unsafe-inline'. Applied to the routes
 *     that handle credentials (signup, set-password, the Google
 *     trampoline), which are rendered per request.
 *   - buildStaticCsp: the marketing pages, which keep 'unsafe-inline'
 *     so they keep static generation.
 *
 * Neither uses 'strict-dynamic': the host allowlist has to stay
 * authoritative for the sign-in SDKs, which are injected at runtime by
 * lib/gip/google-gsi.ts.
 */

/** Route prefixes served with the nonce policy. */
export const NONCE_PATH_PREFIXES = ["onboarding", "auth/google"] as const;

export function usesNonce(pathname: string): boolean {
  const p = pathname.replace(/^\/+/, "");
  return NONCE_PATH_PREFIXES.some(
    (prefix) => p === prefix || p.startsWith(`${prefix}/`),
  );
}

// Next's dev server compiles with eval for HMR; a production build never
// needs it, so the relaxation stays out of prod.
function devEval(env: string | undefined): string {
  return env === "development" ? " 'unsafe-eval'" : "";
}

// accounts.google.com hosts the GSI client script used by /auth/google
// (the customer Google sign-in trampoline). No Apple SDK here — this app
// only trampolines Google.
const SCRIPT_HOSTS =
  "https://accounts.google.com/gsi/client https://analytics.tesserix.app";

function policy(scriptSrc: string): string {
  return [
    "default-src 'self'",
    scriptSrc,
    "style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style",
    "img-src 'self' data: blob: https:",
    "font-src 'self' data:",
    "connect-src 'self' https: wss:",
    "frame-ancestors 'none'",
    // GSI renders the button + One-Tap UI inside an iframe served from
    // accounts.google.com/gsi/.
    "frame-src 'self' https://accounts.google.com/gsi/",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
  ].join("; ");
}

/**
 * jsonLdHash covers the constant JSON-LD block the root layout renders
 * into <head>. It is shared with the static pages, so it cannot take a
 * nonce; being a constant, a hash pins it exactly.
 */
export function buildCsp(
  nonce: string,
  jsonLdHash: string,
  env = process.env.NODE_ENV,
): string {
  return policy(
    `script-src 'self' 'nonce-${nonce}' '${jsonLdHash}'${devEval(env)} ${SCRIPT_HOSTS}`,
  );
}

export function buildStaticCsp(env = process.env.NODE_ENV): string {
  return policy(`script-src 'self' 'unsafe-inline'${devEval(env)} ${SCRIPT_HOSTS}`);
}

/** Per-request nonce. crypto.getRandomValues is available on the Edge runtime. */
export function newNonce(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return btoa(String.fromCharCode(...bytes));
}

/** CSP hash-source for an inline script body. */
export async function sha256Source(text: string): Promise<string> {
  const digest = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(text),
  );
  return `sha256-${btoa(String.fromCharCode(...new Uint8Array(digest)))}`;
}
