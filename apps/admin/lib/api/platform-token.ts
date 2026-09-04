// The machine token mark8ly presents to tesserix-home's platform API.
//
// # Why this exists at all
//
// The admin server used to authenticate to tesserix-home with a SHARED SECRET
// (`X-Internal-Token: $INTERNAL_API_TOKEN`), one bearer for every caller. The
// platform API verifies Zitadel machine tokens instead, and scopes what a
// caller may reach from the identity in that token — so the shared secret has
// no equivalent there and this is what replaces it (tesserix-home#152).
//
// # Authorization: Bearer, not a custom header
//
// tesserix.ts uses `X-Internal-Token` because the istio-ingress in front of
// the PUBLIC tesserix.app parses `Authorization: Bearer` as a JWT and rejects
// an opaque token. That constraint does not apply here: the platform API has
// no public path, and this calls it over the in-cluster ClusterIP, which never
// passes through that ingress. Verified from a pod in this namespace during
// the #152 rollout — Bearer answers 200, no token answers 401.
//
// Do not "restore" the custom header on the strength of the comment in
// tesserix.ts. It is true of a different URL.

const TOKEN_PATH = "/oauth/v2/token";

// Re-mint this many seconds BEFORE the token actually expires.
//
// A token that is valid when checked can still be expired when it arrives.
// Without the skew a merchant's support page fails once an hour, at a
// different minute each time, which is the shape of bug nobody reproduces.
const REFRESH_SKEW_SECONDS = 60;

interface CachedToken {
  token: string;
  /** Epoch ms at which this token should stop being handed out. */
  expiresAt: number;
}

let cached: CachedToken | null = null;
// The in-flight mint, shared by concurrent callers.
//
// The support page fetches a listing and the announcements banner at once. On
// a cold cache both would mint, and the second would race the first into the
// cache for no gain. One promise, awaited by everyone.
let inFlight: Promise<string> | null = null;

/** Test seam. Not exported from the module's public surface by convention. */
export function __resetPlatformTokenCache(): void {
  cached = null;
  inFlight = null;
}

function required(name: string): string {
  const value = (process.env[name] ?? "").trim();
  if (!value) {
    // Named, because the alternative failure is a 401 from the API and an hour
    // spent looking at the credential rather than at the missing variable.
    throw new Error(`platform token: ${name} is not set`);
  }
  return value;
}

/**
 * A valid access token for the platform API, minted or reused.
 *
 * Throws rather than returning null: every caller needs a token to do anything
 * at all, and a null would be threaded through four call sites as a second
 * failure mode that means the same thing.
 */
export async function getPlatformToken(): Promise<string> {
  if (cached && Date.now() < cached.expiresAt) return cached.token;
  if (inFlight) return inFlight;

  inFlight = mint()
    .then((minted) => {
      cached = minted;
      return minted.token;
    })
    .finally(() => {
      // Cleared on BOTH paths. Leaving a rejected promise here would serve the
      // same failure to every later caller, turning one upstream blip into a
      // permanently broken support surface.
      inFlight = null;
    });

  return inFlight;
}

async function mint(): Promise<CachedToken> {
  const issuer = required("TESSERIX_PLATFORM_OIDC_ISSUER").replace(/\/+$/, "");
  const projectId = required("TESSERIX_PLATFORM_OIDC_PROJECT_ID");
  const clientId = required("TESSERIX_PLATFORM_OIDC_CLIENT_ID");
  const clientSecret = required("TESSERIX_PLATFORM_OIDC_CLIENT_SECRET");

  const body = new URLSearchParams({
    grant_type: "client_credentials",
    // BOTH scopes are load-bearing, measured against production Zitadel.
    //
    // The audience scope alone mints a token with NO roles claim, which the
    // platform API refuses with 401 — indistinguishable from a bad credential,
    // so it reads as "my secret is wrong" rather than "my scope is wrong".
    // The roles scope alone gets the wrong audience. Only the pair works.
    scope: [
      "openid",
      `urn:zitadel:iam:org:project:id:${projectId}:aud`,
      "urn:zitadel:iam:org:projects:roles",
    ].join(" "),
  });

  const response = await fetch(`${issuer}${TOKEN_PATH}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      // Client authentication in the header rather than the body, so the
      // secret does not end up in a logged request body.
      Authorization: `Basic ${Buffer.from(`${clientId}:${clientSecret}`).toString("base64")}`,
    },
    body,
    cache: "no-store",
  });

  if (!response.ok) {
    // The status and nothing from the body: an error body from a token
    // endpoint can echo request parameters back, and this is logged.
    throw new Error(`platform token: mint failed with HTTP ${response.status}`);
  }

  const payload = (await response.json()) as { access_token?: string; expires_in?: number };
  if (!payload.access_token) {
    throw new Error("platform token: response carried no access_token");
  }

  // Default to a short life when the server does not say. Erring short costs
  // an extra mint; erring long serves an expired token.
  const lifetime = typeof payload.expires_in === "number" ? payload.expires_in : 300;

  return {
    token: payload.access_token,
    expiresAt: Date.now() + Math.max(lifetime - REFRESH_SKEW_SECONDS, 0) * 1000,
  };
}
