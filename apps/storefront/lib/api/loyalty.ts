// apps/storefront/lib/api/loyalty.ts
//
// Client-side API helpers for storefront loyalty endpoints.

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "";

interface LoyaltyProgramPublic {
  is_active: boolean;
  points_per_dollar: string;
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
    const res = await fetch(
      `${API_BASE}/api/v1/storefront/stores/${storeSlug}/loyalty/enroll`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Storefront-Key": storefrontKey,
        },
        body: JSON.stringify({
          email,
          name,
          referral_code: referralCode,
        }),
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
