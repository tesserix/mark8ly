/**
 * Storefront Content-Security-Policy.
 *
 * script-src is nonce-based rather than 'unsafe-inline': the nonce is
 * minted per request in middleware, Next stamps it onto its own script
 * tags, and StructuredData passes it to the JSON-LD block.
 *
 * Deliberately no 'strict-dynamic'. It would make CSP3 browsers ignore
 * the host list below, and the payment SDKs are the one thing here we
 * cannot afford to get wrong — they pull further scripts at runtime
 * (cdn.razorpay.com, lumberjack.razorpay.com)
 * that the wildcard entries already cover and that lib/csp/allowlist
 * .test.ts keeps honest. Nonces close the inline hole; the allowlist
 * stays the authority on external scripts.
 */
export function buildCsp(nonce: string, env = process.env.NODE_ENV): string {
  // Next's dev server compiles with eval for HMR; a production build
  // never needs it, so the relaxation stays out of prod.
  const devEval = env === "development" ? " 'unsafe-eval'" : "";
  return [
    "default-src 'self'",
    // Storefronts trampoline to mark8ly.com/auth/google for customer
    // Google sign-in, so GSI is not loaded here directly. The allowlist
    // is kept in sync with admin + onboarding so any future inline use
    // (e.g. one-tap on storefront) is unblocked. Razorpay
    // are allowlisted by wildcard, per their own documented CSPs:
    // enumerating hosts does not work because the SDKs pull further
    // scripts at runtime that reading our source never reveals, and each
    // missed host is a silent prod-only breakage.
    `script-src 'self' 'nonce-${nonce}'${devEval} https://accounts.google.com/gsi/client https://*.razorpay.com https://analytics.tesserix.app`,
    // Merchants inject branded CSS via <style> (sanitizeCss in app/layout.tsx).
    "style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style",
    "img-src 'self' data: blob: https:",
    "font-src 'self' data:",
    "connect-src 'self' https: wss:",
    "frame-ancestors 'none'",
    // Razorpay renders its checkout modal (and the bank/UPI redirect
    // flows) in iframes from api.razorpay.com — allowing only the script
    "frame-src 'self' https://accounts.google.com/gsi/ https://*.razorpay.com",
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
