// Origin-based CSRF guard for cookie-authenticated state changes.
//
// The session cookie is SameSite=Lax on .mark8ly.com, which stops a
// third-party site from posting with it but does NOT stop one tenant's
// host from posting to another's — every merchant gets a subdomain of
// the cookie's domain, so *.mark8ly.com is all "same site".
//
// The guard rejects only on positive evidence of a cross-origin request.
// A browser always sends Origin on a cross-origin state change, so an
// absent Origin means a non-browser caller (mobile app, service-to-
// service) rather than an attack, and blocking it would break them.

const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);

export interface CsrfCheckable {
  method: string;
  headers: { get(name: string): string | null };
}

export function isCrossOriginStateChange(req: CsrfCheckable): boolean {
  if (SAFE_METHODS.has(req.method.toUpperCase())) return false;

  const origin = req.headers.get("origin");
  if (!origin) {
    const site = req.headers.get("sec-fetch-site");
    return site !== null && site !== "same-origin";
  }

  // x-forwarded-host wins: behind the gateway the Host header carries
  // the pod address, not the name the browser used.
  const target = (
    req.headers.get("x-forwarded-host") ??
    req.headers.get("host") ??
    ""
  ).toLowerCase();

  let originHost: string;
  try {
    originHost = new URL(origin).host.toLowerCase();
  } catch {
    // Includes the literal "null" origin a sandboxed iframe sends.
    return true;
  }
  return originHost === "" || originHost !== target;
}
