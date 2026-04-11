// Server-side proxy helper for client-originated checkout calls.
//
// The client cannot hit marketplace-api directly: the edge enforces
// `X-Storefront-Key` (a per-env secret that must never ship in the
// client bundle) and the `MARKETPLACE_API_URL` env var is server-only.
// Every client fetch therefore lands on a same-origin `/api/checkout/*`
// route handler that re-issues the call with the right headers.

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

function storefrontHeaders(extra?: HeadersInit): HeadersInit {
  const headers: Record<string, string> = {
    Accept: "application/json",
    "Content-Type": "application/json",
  };
  if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;
  if (extra) {
    const parsed = new Headers(extra);
    parsed.forEach((value, key) => {
      headers[key] = value;
    });
  }
  return headers;
}

export function marketplaceStoreUrl(storeSlug: string): string {
  return `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}`;
}

export async function proxyJson(
  url: string,
  init: RequestInit = {},
): Promise<Response> {
  // Wrap network errors so the client always gets a structured JSON
  // response, not an opaque 500.
  try {
    const upstream = await fetch(url, {
      ...init,
      headers: storefrontHeaders(init.headers),
      // Route handlers run per-request; no RSC cache games wanted here.
      cache: "no-store",
    });
    const bodyText = await upstream.text();
    return new Response(bodyText, {
      status: upstream.status,
      headers: {
        "Content-Type":
          upstream.headers.get("Content-Type") ?? "application/json",
      },
    });
  } catch (err) {
    return Response.json(
      {
        error: "upstream_unreachable",
        message: err instanceof Error ? err.message : "marketplace-api unreachable",
      },
      { status: 502 },
    );
  }
}

export function requireStoreSlug(
  req: Request,
): { slug: string } | Response {
  const url = new URL(req.url);
  const slug = url.searchParams.get("store");
  if (!slug) {
    return Response.json(
      { error: "missing_store", message: "store query param is required" },
      { status: 400 },
    );
  }
  return { slug };
}
