// Runtime config sourced from env vars.
//
// Two flavors:
//   - server-only:   PLATFORM_API_URL, MARKETPLACE_API_URL (server actions /
//                    server components)

export const config = {
  /** Server-side platform-api base URL. Only used from server actions / server components. */
  platformApiUrl: process.env.PLATFORM_API_URL ?? "http://localhost:8086",
  /** Server-side marketplace-api base URL. Only used by /api/journal-subscribe
   *  (#153). Deliberately not platformApiUrl above — that points at the
   *  separate platform-api service. Name and default port match
   *  apps/admin/lib/api/marketplace-api.ts, which already talks to
   *  marketplace-api from a Next.js server. */
  marketplaceApiUrl: process.env.MARKETPLACE_API_URL ?? "http://localhost:8088",
} as const;
