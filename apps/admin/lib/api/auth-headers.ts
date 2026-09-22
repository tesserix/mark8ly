/**
 * The one place admin API clients build request headers for marketplace-api.
 *
 * # Why this module exists
 *
 * marketplace-api authenticates admin calls with the session identity
 * (`X-User-Id` / `X-Tenant-Id`) AND the platform's shared internal secret
 * (`X-Internal-Auth`). Miss the secret and every request 401s.
 *
 * Eleven clients under lib/api each built that header set by hand. Seven
 * got it right. Four — campaigns, coupons, csvImports, loyalty — omitted
 * `X-Internal-Auth`, so Campaigns, Segments, Coupons, Loyalty and CSV
 * import had never worked in production (#890). Gift Cards worked only
 * because it happens to route through marketplace-api.ts.
 *
 * Nothing surfaced it: the BFF turns those 401s into 200s with empty
 * payloads, so the features render as "nothing here yet" rather than as
 * an error, and a coupon write reported "Please check your input".
 *
 * The duplication was the defect. Every client now imports from here, and
 * `auth-headers.test.ts` fails if a client under lib/api builds a
 * `X-User-Id` header without going through this module.
 */

const MARKETPLACE_INTERNAL_AUTH =
  process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";

/** Session identity as it arrives from Next middleware. */
export interface SessionHeaders {
  userId: string;
  tenantId: string;
  /**
   * User's email for audit-log attribution. Optional for backward
   * compatibility — when present it is forwarded as X-User-Email and
   * recorded against any audit event the request emits.
   */
  email?: string;
}

function base(session: SessionHeaders): Record<string, string> {
  const headers: Record<string, string> = {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    Accept: "application/json",
  };
  if (session.email) {
    headers["X-User-Email"] = session.email;
  }
  // Empty in local dev where marketplace-api runs without the secret.
  // Sending an empty header would be worse than omitting it: the server
  // treats presence as an assertion.
  if (MARKETPLACE_INTERNAL_AUTH) {
    headers["X-Internal-Auth"] = MARKETPLACE_INTERNAL_AUTH;
  }
  return headers;
}

/** Headers for a GET. No Content-Type — there is no body. */
export function readHeaders(session: SessionHeaders): Record<string, string> {
  return base(session);
}

/** Headers for a request carrying a JSON body. */
export function writeHeaders(session: SessionHeaders): Record<string, string> {
  return { ...base(session), "Content-Type": "application/json" };
}
