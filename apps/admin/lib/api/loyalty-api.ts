// apps/admin/lib/api/loyalty-api.ts
//
// Server-side API client for loyalty endpoints. Follows the same
// calling convention as coupons-api.ts: server components pass
// SessionHeaders, the client does the header rename dance.

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface SessionHeaders {
  userId: string;
  tenantId: string;
}

// ---------- Types ----------

export interface LoyaltyProgram {
  id: string;
  is_active: boolean;
  points_per_dollar: string;
  points_currency: string;
  signup_bonus: number;
  referral_bonus: number;
  referee_bonus: number;
  point_expiry_days: number | null;
  min_redeem_points: number;
  points_value: string;
  tiers: LoyaltyTier[];
  created_at: string;
  updated_at: string;
}

export interface LoyaltyTier {
  name: string;
  min_points: number;
  multiplier: string;
}

export interface LoyaltyMember {
  id: string;
  customer_email: string;
  customer_name: string | null;
  points_balance: number;
  lifetime_points: number;
  tier: string;
  referral_code: string;
  enrolled_at: string;
}

export interface LoyaltyTransaction {
  id: string;
  type: string;
  points: number;
  balance_after: number;
  description: string | null;
  adjusted_by: string | null;
  order_id: string | null;
  created_at: string;
}

export interface LoyaltyReferral {
  id: string;
  referrer_id: string;
  referee_id: string;
  status: string;
  referrer_bonus: number;
  referee_bonus: number;
  completed_at: string | null;
  created_at: string;
}

// ---------- Helpers ----------

function authHeaders(session: SessionHeaders): Record<string, string> {
  return {
    "Content-Type": "application/json",
    Accept: "application/json",
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
  };
}

// ---------- API functions ----------

export async function getLoyaltyProgram(
  storeId: string,
  session: SessionHeaders,
): Promise<LoyaltyProgram | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/program`;
  try {
    const res = await fetch(url, {
      headers: authHeaders(session),
      cache: "no-store",
    });
    if (!res.ok) return null;
    const json = await res.json();
    return json.data ?? null;
  } catch {
    return null;
  }
}

export async function updateLoyaltyProgram(
  storeId: string,
  body: Record<string, unknown>,
  session: SessionHeaders,
): Promise<LoyaltyProgram | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/program`;
  try {
    const res = await fetch(url, {
      method: "PUT",
      headers: authHeaders(session),
      body: JSON.stringify(body),
    });
    if (!res.ok) return null;
    const json = await res.json();
    return json.data ?? null;
  } catch {
    return null;
  }
}

export async function getLoyaltyMembers(
  storeId: string,
  session: SessionHeaders,
  query: { page?: number; per_page?: number; tier?: string } = {},
): Promise<{ data: LoyaltyMember[]; total: number; page: number }> {
  const params = new URLSearchParams();
  if (query.page) params.set("page", String(query.page));
  if (query.per_page) params.set("per_page", String(query.per_page));
  if (query.tier) params.set("tier", query.tier);

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/members?${params}`;
  try {
    const res = await fetch(url, {
      headers: authHeaders(session),
      cache: "no-store",
    });
    if (!res.ok) return { data: [], total: 0, page: 1 };
    return await res.json();
  } catch {
    return { data: [], total: 0, page: 1 };
  }
}

export async function getLoyaltyMember(
  storeId: string,
  memberId: string,
  session: SessionHeaders,
): Promise<{
  data: LoyaltyMember;
  transactions: LoyaltyTransaction[];
} | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/members/${memberId}`;
  try {
    const res = await fetch(url, {
      headers: authHeaders(session),
      cache: "no-store",
    });
    if (!res.ok) return null;
    return await res.json();
  } catch {
    return null;
  }
}

export async function adjustPoints(
  storeId: string,
  memberId: string,
  body: { points: number; description: string },
  session: SessionHeaders,
): Promise<boolean> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/members/${memberId}/adjust`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: authHeaders(session),
      body: JSON.stringify(body),
    });
    return res.ok;
  } catch {
    return false;
  }
}

export async function getLoyaltyReferrals(
  storeId: string,
  session: SessionHeaders,
  query: { page?: number; per_page?: number } = {},
): Promise<{ data: LoyaltyReferral[]; total: number; page: number }> {
  const params = new URLSearchParams();
  if (query.page) params.set("page", String(query.page));
  if (query.per_page) params.set("per_page", String(query.per_page));

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/loyalty/referrals?${params}`;
  try {
    const res = await fetch(url, {
      headers: authHeaders(session),
      cache: "no-store",
    });
    if (!res.ok) return { data: [], total: 0, page: 1 };
    return await res.json();
  } catch {
    return { data: [], total: 0, page: 1 };
  }
}
