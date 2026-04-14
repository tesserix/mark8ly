// apps/storefront/lib/api/loyalty.ts
//
// Server-side helpers for the storefront loyalty endpoints. All callers
// are Server Components (/account/* pages, layout, sign-in actions), so
// we hit marketplace-api directly with the server-only MARKETPLACE_API_URL
// and attach the X-Storefront-Key. Never import this from a client bundle
// — the storefront key must not ship to the browser.

const API_BASE =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

interface LoyaltyProgramPublic {
  is_active: boolean;
  points_per_unit: string;
  points_currency: string;
  signup_bonus: number;
  referral_bonus: number;
  referee_bonus: number;
  min_redeem_points: number;
  points_value: string;
  tiers: { name: string; min_points: number; multiplier: string }[];
}

interface CustomerLoyalty {
  points_balance: number;
  lifetime_points: number;
  tier: string;
  referral_code: string;
}

interface RedeemResult {
  points_redeemed: number;
  value: string;
}

export async function getProgram(
  storeSlug: string,
  storefrontKey: string,
): Promise<LoyaltyProgramPublic | null> {
  try {
    const res = await fetch(
      `${API_BASE}/api/v1/storefront/stores/${storeSlug}/loyalty/program`,
      {
        headers: { "X-Storefront-Key": storefrontKey },
        cache: "no-store",
      },
    );
    if (!res.ok) return null;
    const json = await res.json();
    return json.data ?? null;
  } catch {
    return null;
  }
}

export async function enrollCustomer(
  storeSlug: string,
  storefrontKey: string,
  email: string,
  name?: string,
  referralCode?: string,
): Promise<CustomerLoyalty | null> {
  try {
    const body: Record<string, unknown> = { email };
    if (name) body.name = name;
    if (referralCode) body.referral_code = referralCode;
    const res = await fetch(
      `${API_BASE}/api/v1/storefront/stores/${storeSlug}/loyalty/enroll`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Storefront-Key": storefrontKey,
        },
        body: JSON.stringify(body),
      },
    );
    if (!res.ok) return null;
    const json = await res.json();
    return json.data ?? null;
  } catch {
    return null;
  }
}

export async function getMe(
  storeSlug: string,
  storefrontKey: string,
  email: string,
): Promise<CustomerLoyalty | null> {
  try {
    const res = await fetch(
      `${API_BASE}/api/v1/storefront/stores/${storeSlug}/loyalty/me?email=${encodeURIComponent(email)}`,
      {
        headers: { "X-Storefront-Key": storefrontKey },
        cache: "no-store",
      },
    );
    if (!res.ok) return null;
    const json = await res.json();
    return json.data ?? null;
  } catch {
    return null;
  }
}

export async function redeemPoints(
  storeSlug: string,
  storefrontKey: string,
  email: string,
  points: number,
): Promise<RedeemResult | null> {
  try {
    const res = await fetch(
      `${API_BASE}/api/v1/storefront/stores/${storeSlug}/loyalty/redeem`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Storefront-Key": storefrontKey,
        },
        body: JSON.stringify({ email, points }),
      },
    );
    if (!res.ok) return null;
    const json = await res.json();
    return json.data ?? null;
  } catch {
    return null;
  }
}

export type { LoyaltyProgramPublic, CustomerLoyalty, RedeemResult };
