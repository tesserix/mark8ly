import { headers } from "next/headers";

import { resolveAuth } from "@/lib/auth/serverSession";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const MARKETPLACE_INTERNAL_AUTH =
  process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";

const HOP_BY_HOP = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
  "content-encoding",
  "content-length",
]);

interface ProxyOptions {
  searchParams?: URLSearchParams;
}

export async function proxyAdminApi(
  request: Request,
  backendPath: string,
  options: ProxyOptions = {},
): Promise<Response> {
  // resolveAuth tries middleware-injected x-session-* headers first
  // (the fast path) and falls back to validating the cookie directly
  // against auth-bff. Closes the surprise-401 gap where headers
  // didn't propagate from middleware to the route handler.
  const auth = await resolveAuth();
  if (!auth) {
    return jsonError(401, "unauthorized", "session headers missing");
  }
  const { userId, tenantId, email } = auth;

  const query = options.searchParams ?? new URL(request.url).searchParams;
  const queryString = query.toString();
  const normalizedPath = backendPath.startsWith("/")
    ? backendPath.slice(1)
    : backendPath;
  const upstreamUrl = `${MARKETPLACE_API_URL}/api/v1/admin/${normalizedPath}${queryString ? `?${queryString}` : ""}`;

  const upstreamHeaders: Record<string, string> = {
    "X-User-Id": userId,
    "X-Tenant-Id": tenantId,
    Accept: "application/json",
  };
  if (email) {
    upstreamHeaders["X-User-Email"] = email;
  }
  // Marketplace-api's HeaderTrustAuth requires X-Internal-Auth when
  // MARKETPLACE_INTERNAL_AUTH_SECRET is set on the Go service. Match
  // the pattern every other admin → marketplace-api caller uses (see
  // apps/admin/lib/api/marketplace-api.ts:300 and shipping-api.ts:86).
  // Earlier this proxy omitted the header — every /subscription,
  // /notifications/unread-count etc. request 401'd, surfaced as a
  // mid-session "Session expired" toast.
  if (MARKETPLACE_INTERNAL_AUTH) {
    upstreamHeaders["X-Internal-Auth"] = MARKETPLACE_INTERNAL_AUTH;
  }
  const contentType = request.headers.get("content-type");
  if (contentType) {
    upstreamHeaders["Content-Type"] = contentType;
  }

  const init: RequestInit = {
    method: request.method,
    headers: upstreamHeaders,
    // Surface backend 3xx responses verbatim (e.g. complete-action returns
    // 302 to the Stripe hosted-invoice URL — the browser reads the Location
    // header directly). Without this, Node's fetch silently follows and the
    // proxy would return the Stripe page instead of the redirect.
    redirect: "manual",
  };
  if (request.method !== "GET" && request.method !== "HEAD") {
    init.body = await request.arrayBuffer();
  }

  let upstream: Response;
  try {
    upstream = await fetch(upstreamUrl, init);
  } catch (err) {
    const message = err instanceof Error ? err.message : "upstream fetch failed";
    return jsonError(502, "bad_gateway", message);
  }

  const responseHeaders = new Headers();
  upstream.headers.forEach((value, key) => {
    if (!HOP_BY_HOP.has(key.toLowerCase())) {
      responseHeaders.set(key, value);
    }
  });

  return new Response(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: responseHeaders,
  });
}

function jsonError(status: number, error: string, message: string): Response {
  return new Response(JSON.stringify({ error, message }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
