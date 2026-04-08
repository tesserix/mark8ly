/**
 * Storefront client for platform-api's public routes. Unlike the
 * admin client this file has NO internal endpoint access — the
 * storefront runs entirely on public routes and never forwards a
 * session cookie, because customers don't have sessions in the
 * Phase S slice.
 */

import type { StorefrontTheme } from "@/lib/storefront-theme";

const PLATFORM_API_URL =
  process.env.PLATFORM_API_URL ?? "http://localhost:8086";

export interface PublicStore {
  id: string;
  slug: string;
  name: string;
  country_code: string;
  currency_code: string;
  timezone: string;
  logo_url?: string;
  storefront_theme: StorefrontTheme;
}

/**
 * Resolves a store by its public slug. Returns null for any
 * non-200 response so the caller can render a "store not found"
 * view without try/catching.
 */
export async function fetchStoreBySlug(
  slug: string,
): Promise<PublicStore | null> {
  if (!slug) return null;
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/api/v1/stores/by-slug/${encodeURIComponent(slug)}`,
      { cache: "no-store" },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { data: PublicStore };
    return body.data;
  } catch {
    return null;
  }
}
