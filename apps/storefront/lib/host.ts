// apps/storefront/lib/host.ts
//
// Validates the inbound Host header for use as a cookie Domain. Strips
// :port. Rejects anything that is not a plain hostname (no path chars,
// no consecutive dots, no leading/trailing dot, no IP literal brackets).
//
// The output is fed verbatim into Set-Cookie Domain=, so an unsafe host
// MUST return null and the caller MUST refuse to mint.

const HOSTNAME_RE = /^[a-zA-Z0-9.-]+$/;

export function sanitizeHost(raw: string | null | undefined): string | null {
  if (!raw) return null;
  const noPort = raw.split(":")[0] ?? "";
  if (!noPort) return null;
  if (noPort.startsWith(".") || noPort.endsWith(".")) return null;
  if (noPort.includes("..")) return null;
  if (!HOSTNAME_RE.test(noPort)) return null;
  return noPort;
}
