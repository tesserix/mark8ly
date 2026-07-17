import type { createApiClient } from "./client";
import {
  orderDetailSchema,
  orderListSchema,
  type Order,
  type OrderDetail,
  type OrderListResponse,
} from "./schemas/orders";

export interface ListOrdersParams {
  status?: string;
  payment_status?: string;
  search?: string;
  page?: string;
  page_size?: string;
}

export interface ConfirmOrderBody {
  /** Optional payment-status change on confirm (e.g. "authorized" | "paid"). */
  payment_status?: string;
  reason?: string;
}

export interface RefundOrderBody {
  /** Omit for a full remaining-balance refund. */
  amount?: number;
  /**
   * REQUIRED idempotency scope — a stable id per refund attempt, reused on
   * retry (RefundOrderRequest.refund_request_id). Generate with `randomId()`.
   */
  refund_request_id: string;
  reason?: string;
}

export function createOrdersApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListOrdersParams) =>
      client.get<OrderListResponse>("/orders", params as Record<string, string>, orderListSchema),
    // get/confirm/fulfill/cancel all return a BARE AdminOrderResponse
    // (items[]/addresses[], tax_lines only on get). refund returns a different
    // RefundOrderResponse, so it stays unschema'd.
    get: (id: string) =>
      client.get<OrderDetail>(`/orders/${id}`, undefined, orderDetailSchema),
    /** ConfirmOrderRequest: optional payment_status + reason. */
    confirm: (id: string, body?: ConfirmOrderBody) =>
      client.post<OrderDetail>(`/orders/${id}/confirm`, body ?? {}, orderDetailSchema),
    /** MarkFulfilled ignores the body — the old tracking_number was a no-op. */
    fulfill: (id: string) =>
      client.post<OrderDetail>(`/orders/${id}/fulfill`, {}, orderDetailSchema),
    /** CancelOrderRequest.reason is REQUIRED (binding) — omitting it is a 400. */
    cancel: (id: string, reason: string) =>
      client.post<OrderDetail>(`/orders/${id}/cancel`, { reason }, orderDetailSchema),
    /** refund_request_id is REQUIRED; amount omitted ⇒ full remaining balance. */
    refund: (id: string, body: RefundOrderBody) => client.post(`/orders/${id}/refund`, body),
  };
}

export type { Order, OrderDetail };
