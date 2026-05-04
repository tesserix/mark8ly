// Shared helper for server-side calls into platform-api's /internal/* surface.
// Carries the X-Internal-Auth shared secret so platform-api's
// RequireInternalAuth gate can reject anything not coming from a trusted
// in-cluster caller. The secret is read once at module load and reused.
//
// An empty PLATFORM_INTERNAL_AUTH_SECRET no-ops the header — that mirrors
// platform-api's permissive-empty middleware and keeps local dev working
// before the secret is provisioned in the cluster.

const PLATFORM_API_URL =
  process.env.PLATFORM_API_URL ?? "http://localhost:8086";

// Strip surrounding whitespace / trailing newlines that GCP Secret Manager
// sometimes persists on random-base64 secrets — HTTP would silently drop
// the trailing LF and the constant-time compare on platform-api's side
// would 401 every request. Same defensive trim used in otto's _proxy.ts.
const PLATFORM_INTERNAL_AUTH = (
  process.env.PLATFORM_INTERNAL_AUTH_SECRET ?? ""
).trim();

/** Headers to add to every server-side fetch into platform-api /internal/*. */
export function platformInternalHeaders(
  extra: Record<string, string> = {},
): Record<string, string> {
  const out: Record<string, string> = { ...extra };
  if (PLATFORM_INTERNAL_AUTH) {
    out["X-Internal-Auth"] = PLATFORM_INTERNAL_AUTH;
  }
  return out;
}

/** Convenience wrapper — same shape as fetch, plus the internal-auth header. */
export function platformInternalFetch(
  path: string,
  init: RequestInit = {},
): Promise<Response> {
  const url = path.startsWith("http")
    ? path
    : `${PLATFORM_API_URL}${path.startsWith("/") ? path : `/${path}`}`;
  const headers = new Headers(init.headers);
  if (PLATFORM_INTERNAL_AUTH && !headers.has("X-Internal-Auth")) {
    headers.set("X-Internal-Auth", PLATFORM_INTERNAL_AUTH);
  }
  return fetch(url, { ...init, headers });
}

export const platformInternalAuthEnabled = PLATFORM_INTERNAL_AUTH !== "";
