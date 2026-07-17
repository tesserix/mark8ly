import type { createApiClient } from "./client";
import { enveloped } from "./schema-helpers";
import {
  couponSchema,
  couponListSchema,
  couponDetailEnvelopeSchema,
  type Coupon,
  type CouponListResponse,
  type CouponDetailEnvelope,
} from "./schemas/coupons";

export interface ListCouponsParams {
  status?: string;
  search?: string;
  page?: string;
  per_page?: string;
}

/** Body for POST /coupons (CreateCouponRequest, coupons_dto.go:14). */
export interface CreateCouponBody {
  code: string;
  title: string;
  description?: string;
  type: string;
  value: number;
  currency_code?: string;
  min_purchase?: number;
  max_discount?: number;
  usage_limit?: number;
  per_customer?: number;
  target_type?: string;
  target_ids?: string[];
  stackable?: boolean;
  starts_at?: string;
  ends_at?: string;
}

/** Body for PATCH /coupons/:id (PatchCouponRequest, coupons_dto.go:33). */
export interface PatchCouponBody {
  title?: string;
  description?: string;
  min_purchase?: number;
  max_discount?: number;
  usage_limit?: number;
  per_customer?: number;
  stackable?: boolean;
  starts_at?: string;
  ends_at?: string;
  status?: string;
}

/**
 * Admin coupon CRUD. Mirrors web routes.go:449-469, now on the mobile group.
 * Every response is `{data: ...}`-wrapped; list uses `{data,total,page}`, get
 * uses `{data,usage?,usage_total?}`, mutations return `{data: coupon}` which we
 * unwrap. Delete returns `{message}` (no body schema).
 */
export function createCouponsApi(client: ReturnType<typeof createApiClient>) {
  const couponEnvelope = enveloped(couponSchema);
  return {
    list: (params?: ListCouponsParams) =>
      client.get<CouponListResponse>("/coupons", params as Record<string, string>, couponListSchema),
    get: (id: string) =>
      client.get<CouponDetailEnvelope>(`/coupons/${id}`, undefined, couponDetailEnvelopeSchema),
    create: (body: CreateCouponBody) =>
      client.post<{ data: Coupon }>("/coupons", body, couponEnvelope).then((r) => r.data),
    patch: (id: string, body: PatchCouponBody) =>
      client.patch<{ data: Coupon }>(`/coupons/${id}`, body, couponEnvelope).then((r) => r.data),
    remove: (id: string) => client.delete<{ message: string }>(`/coupons/${id}`),
  };
}

export type { Coupon, CouponListResponse };
