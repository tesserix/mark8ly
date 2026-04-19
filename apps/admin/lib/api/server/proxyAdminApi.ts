import { headers } from "next/headers";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

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
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!userId || !tenantId) {
    return jsonError(401, "unauthorized", "session headers missing");
  }

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
