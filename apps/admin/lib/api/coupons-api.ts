// apps/admin/lib/api/coupons-api.ts
//
// Admin coupon API client. Follows the same calling convention as
// marketplace-api.ts: server components pass SessionHeaders, the client
// does the header rename dance.

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface SessionHeaders {
  userId: string;
  tenantId: string;
}

function authHeaders(session: SessionHeaders): Record<string, string> {
  return {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    "Content-Type": "application/json",
    Accept: "application/json",
  };
}

// ---------- Types ----------

export interface AdminCoupon {
  id: string;
  code: string;
  title: string;
  description: string | null;
  type: "percentage" | "fixed_amount" | "free_shipping";
  value: string;
  currency_code: string | null;
  min_purchase: string | null;
  max_discount: string | null;
  usage_limit: number | null;
  per_customer: number;
  target_type: "all" | "products" | "categories";
  target_ids: string[];
  stackable: boolean;
  starts_at: string;
  ends_at: string | null;
  status: "active" | "disabled" | "expired";
  usage_count: number;
  created_at: string;
  updated_at: string;
}

export interface CouponUsageRow {
  id: string;
  order_id: string;
  customer_email: string;
  discount_amount: string;
  currency_code: string;
  created_at: string;
}

export interface ListCouponsQuery {
  status?: string;
  search?: string;
  page?: number;
  per_page?: number;
}

export interface ListCouponsResponse {
  data: AdminCoupon[];
  total: number;
  page: number;
}

export interface GetCouponResponse {
  data: AdminCoupon;
  usage: CouponUsageRow[];
  usage_total: number;
}

export interface CreateCouponBody {
  code: string;
  title: string;
  description?: string;
  type: "percentage" | "fixed_amount" | "free_shipping";
  value: string;
  currency_code?: string;
  min_purchase?: string;
  max_discount?: string;
  usage_limit?: number;
  per_customer?: number;
  target_type?: string;
  target_ids?: string[];
  stackable?: boolean;
  starts_at?: string;
  ends_at?: string;
}

export interface PatchCouponBody {
  title?: string;
  description?: string;
  min_purchase?: string;
  max_discount?: string;
  usage_limit?: number;
  per_customer?: number;
  stackable?: boolean;
  starts_at?: string;
  ends_at?: string;
  status?: string;
}

// ---------- API functions ----------

export async function listCoupons(
  storeId: string,
  query: ListCouponsQuery,
  session: SessionHeaders,
): Promise<ListCouponsResponse | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.search) params.set("search", query.search);
  if (query.page) params.set("page", String(query.page));
  if (query.per_page) params.set("per_page", String(query.per_page));

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/coupons?${params}`;
  try {
    const res = await fetch(url, { headers: authHeaders(session), cache: "no-store" });
    if (!res.ok) return null;
    return (await res.json()) as ListCouponsResponse;
  } catch {
    return null;
  }
}

export async function getCoupon(
  storeId: string,
  couponId: string,
  session: SessionHeaders,
): Promise<GetCouponResponse | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/coupons/${couponId}`;
  try {
    const res = await fetch(url, { headers: authHeaders(session), cache: "no-store" });
    if (!res.ok) return null;
    return (await res.json()) as GetCouponResponse;
  } catch {
    return null;
  }
}

export async function createCoupon(
  storeId: string,
  body: CreateCouponBody,
  session: SessionHeaders,
): Promise<{ data: AdminCoupon } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/coupons`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: authHeaders(session),
      body: JSON.stringify(body),
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminCoupon };
  } catch {
    return null;
  }
}

export async function patchCoupon(
  storeId: string,
  couponId: string,
  body: PatchCouponBody,
  session: SessionHeaders,
): Promise<{ data: AdminCoupon } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/coupons/${couponId}`;
  try {
    const res = await fetch(url, {
      method: "PATCH",
      headers: authHeaders(session),
      body: JSON.stringify(body),
    });
    if (!res.ok) return null;
    return (await res.json()) as { data: AdminCoupon };
  } catch {
    return null;
  }
}

export async function deleteCoupon(
  storeId: string,
  couponId: string,
  session: SessionHeaders,
): Promise<boolean> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/coupons/${couponId}`;
  try {
    const res = await fetch(url, {
      method: "DELETE",
      headers: authHeaders(session),
    });
    return res.ok;
  } catch {
    return false;
  }
}
