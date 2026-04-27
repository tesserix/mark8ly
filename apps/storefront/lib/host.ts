//
// Validates the inbound Host header for use as a cookie Domain. Strips
// :port. Rejects anything that is not a plain hostname (no path chars,
// no consecutive dots, no leading/trailing dot, no IP literal brackets,
// no IPv4 literals, no labels starting or ending with a hyphen).
//
// The output is fed verbatim into Set-Cookie Domain=, so an unsafe host
// MUST return null and the caller MUST refuse to mint.

// Each label must start and end with alphanumeric; hyphens only in the middle.
const HOSTNAME_RE =
  /^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?)*$/;

// Reject 4-dotted-decimal IPv4 literals — cookie Domain is for hostnames only.
const IPV4_RE = /^\d+\.\d+\.\d+\.\d+$/;

export function sanitizeHost(raw: string | null | undefined): string | null {
  if (!raw) return null;
  const noPort = raw.split(":")[0] ?? "";
  if (!noPort) return null;
  if (IPV4_RE.test(noPort)) return null;
  if (!HOSTNAME_RE.test(noPort)) return null;
  return noPort;
}
