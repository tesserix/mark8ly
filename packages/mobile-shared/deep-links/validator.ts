const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const HANDLE_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const SLUG_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

interface RouteMatch {
  screen: string;
  params: Record<string, string>;
}

type RouteDefinition = (segments: string[]) => RouteMatch | null;

const ROUTE_DEFINITIONS: RouteDefinition[] = [
  // account/orders/{uuid}
  (segments) => {
    if (
      segments.length === 3 &&
      segments[0] === "account" &&
      segments[1] === "orders"
    ) {
      const id = segments[2];
      if (!UUID_PATTERN.test(id)) return null;
      return { screen: "account/orders/[id]", params: { id } };
    }
    return null;
  },

  // account/loyalty
  (segments) => {
    if (segments.length === 2 && segments[0] === "account" && segments[1] === "loyalty") {
      return { screen: "account/loyalty", params: {} };
    }
    return null;
  },

  // account/wishlist
  (segments) => {
    if (segments.length === 2 && segments[0] === "account" && segments[1] === "wishlist") {
      return { screen: "account/wishlist", params: {} };
    }
    return null;
  },

  // account/reviews
  (segments) => {
    if (segments.length === 2 && segments[0] === "account" && segments[1] === "reviews") {
      return { screen: "account/reviews", params: {} };
    }
    return null;
  },

  // browse/product/{handle}
  (segments) => {
    if (
      segments.length === 3 &&
      segments[0] === "browse" &&
      segments[1] === "product"
    ) {
      const handle = segments[2];
      if (!HANDLE_PATTERN.test(handle)) return null;
      return { screen: "browse/product/[handle]", params: { handle } };
    }
    return null;
  },

  // browse/category/{slug}
  (segments) => {
    if (
      segments.length === 3 &&
      segments[0] === "browse" &&
      segments[1] === "category"
    ) {
      const slug = segments[2];
      if (!SLUG_PATTERN.test(slug)) return null;
      return { screen: "browse/category/[slug]", params: { slug } };
    }
    return null;
  },
];

function parsePath(path: string): string[] | null {
  if (!path || path.trim() === "") return null;

  // Reject path traversal
  if (path.includes("..")) return null;

  // Strip leading slash if present
  const normalized = path.startsWith("/") ? path.slice(1) : path;

  if (normalized === "") return null;

  const segments = normalized.split("/");

  // Reject empty segments (e.g. double slashes)
  if (segments.some((s) => s === "")) return null;

  return segments;
}

export function validateDeepLink(path: string): boolean {
  return extractDeepLinkRoute(path) !== null;
}

export function extractDeepLinkRoute(path: string): RouteMatch | null {
  const segments = parsePath(path);
  if (!segments) return null;

  for (const matcher of ROUTE_DEFINITIONS) {
    const result = matcher(segments);
    if (result !== null) return result;
  }

  return null;
}
