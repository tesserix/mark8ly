const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const HANDLE_SLUG_PATTERN = /^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$/i;

type RouteMatch = {
  route: string;
  params: Record<string, string>;
};

/**
 * Ordered allowlist of valid deep link patterns.
 * Each entry has a regex to match the full path and an extractor
 * that returns the structured route + params.
 */
const ROUTE_ALLOWLIST: Array<{
  pattern: RegExp;
  extract: (match: RegExpMatchArray) => RouteMatch | null;
}> = [
  {
    // account/orders/[uuid]
    pattern: /^account\/orders\/(.+)$/,
    extract: (match) => {
      const id = match[1];
      if (!id || !UUID_PATTERN.test(id)) return null;
      return { route: "account/orders/[id]", params: { id } };
    },
  },
  {
    // account/loyalty
    pattern: /^account\/loyalty$/,
    extract: () => ({ route: "account/loyalty", params: {} }),
  },
  {
    // account/wishlist
    pattern: /^account\/wishlist$/,
    extract: () => ({ route: "account/wishlist", params: {} }),
  },
  {
    // account/reviews
    pattern: /^account\/reviews$/,
    extract: () => ({ route: "account/reviews", params: {} }),
  },
  {
    // browse/product/[handle] — no slashes in handle
    pattern: /^browse\/product\/([^/]+)$/,
    extract: (match) => {
      const handle = match[1];
      if (!handle || !HANDLE_SLUG_PATTERN.test(handle)) return null;
      return { route: "browse/product/[handle]", params: { handle } };
    },
  },
  {
    // browse/category/[slug] — no slashes in slug
    pattern: /^browse\/category\/([^/]+)$/,
    extract: (match) => {
      const slug = match[1];
      if (!slug || !HANDLE_SLUG_PATTERN.test(slug)) return null;
      return { route: "browse/category/[slug]", params: { slug } };
    },
  },
];

function hasPathTraversal(path: string): boolean {
  return path.includes("..");
}

export function validateDeepLink(path: string): boolean {
  if (!path || hasPathTraversal(path)) return false;

  for (const entry of ROUTE_ALLOWLIST) {
    const match = path.match(entry.pattern);
    if (match) {
      return entry.extract(match) !== null;
    }
  }

  return false;
}

export function extractDeepLinkRoute(path: string): RouteMatch | null {
  if (!path || hasPathTraversal(path)) return null;

  for (const entry of ROUTE_ALLOWLIST) {
    const match = path.match(entry.pattern);
    if (match) {
      return entry.extract(match);
    }
  }

  return null;
}
