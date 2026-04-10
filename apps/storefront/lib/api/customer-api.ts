/**
 * Server-side API client for customer account endpoints in
 * marketplace-api. Forwards session cookies so the backend can
 * authenticate the request via auth-bff session validation.
 *
 * Follows the same patterns as marketplace-api.ts — returns null /
 * empty array on failure so callers render gracefully without
 * try/catch.
 */

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

function accountHeaders(cookieHeader: string): HeadersInit {
  const headers: Record<string, string> = {
    Accept: "application/json",
    Cookie: cookieHeader,
  };
  if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;
  return headers;
}

export interface CustomerProfile {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  phone: string;
  avatar_url: string;
  marketing_opt_in: boolean;
  created_at: string;
}

export interface CustomerAddress {
  id: string;
  label: string;
  is_default: boolean;
  name: string;
  line1: string;
  line2: string;
  city: string;
  region: string;
  postal_code: string;
  country_code: string;
  phone: string;
}

export async function fetchProfile(
  storeSlug: string,
  cookieHeader: string,
): Promise<CustomerProfile | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/account`;
  try {
    const res = await fetch(url, {
      headers: accountHeaders(cookieHeader),
      cache: "no-store",
    });
    if (!res.ok) return null;
    const body = (await res.json()) as { data: CustomerProfile };
    return body.data ?? null;
  } catch {
    return null;
  }
}

export async function fetchAddresses(
  storeSlug: string,
  cookieHeader: string,
): Promise<CustomerAddress[]> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/account/addresses`;
  try {
    const res = await fetch(url, {
      headers: accountHeaders(cookieHeader),
      cache: "no-store",
    });
    if (!res.ok) return [];
    const body = (await res.json()) as { data: CustomerAddress[] };
    return body.data ?? [];
  } catch {
    return [];
  }
}
