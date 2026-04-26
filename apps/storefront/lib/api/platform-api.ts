/**
 * Storefront client for platform-api's public routes. Unlike the
 * admin client this file has NO internal endpoint access — the
 * storefront runs entirely on public routes and never forwards a
 * session cookie, because customers don't have sessions in the
 * Phase S slice.
 */

import type { StorefrontTheme } from "@repo/ui/storefront-theme";

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

// Countries / states reference data — shared with the admin app.
// Mirrors the /locations endpoints in platform-api.

export interface Country {
  code: string;
  name: string;
  native_name: string;
  calling_code: string;
  currency_code: string;
  flag_emoji: string;
  region: string;
}

export interface State {
  id: string;
  country_code: string;
  code: string;
  name: string;
  type: string;
}

export async function listCountries(): Promise<Country[]> {
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/api/v1/locations/countries`,
      { cache: "no-store" },
    );
    if (!res.ok) return [];
    const body = (await res.json()) as { data: Country[] };
    return body.data ?? [];
  } catch {
    return [];
  }
}

export async function listStatesByCountry(code: string): Promise<State[]> {
  if (!code) return [];
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/api/v1/locations/countries/${encodeURIComponent(code)}/states`,
      { cache: "no-store" },
    );
    if (!res.ok) return [];
    const body = (await res.json()) as { data: State[] };
    return body.data ?? [];
  } catch {
    return [];
  }
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
