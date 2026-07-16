import type { createApiClient } from "./client";
import type { OrderDetail } from "./types";
import { orderListSchema, type Order, type OrderListResponse } from "./schemas/orders";

export interface ListOrdersParams {
  status?: string;
  payment_status?: string;
  search?: string;
  page?: string;
  page_size?: string;
}

export function createOrdersApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListOrdersParams) =>
      client.get<OrderListResponse>("/orders", params as Record<string, string>, orderListSchema),
    /**
     * NO schema, deliberately. OrderDetail is still the hand-written
     * (and largely fictional) type: `line_items`, `shipping_address`,
     * `timeline`, `tracking_number`, `payment_method` and
     * `payment_transaction_id` do not exist on the wire. Attaching a schema
     * here would turn a broken screen into a thrown contract_mismatch
     * without fixing anything. The detail rewrite is its own sub-project —
     * see docs/superpowers/specs/2026-07-16-mobile-admin-lists-bcd-design.md.
     */
    get: (id: string) => client.get<OrderDetail>(`/orders/${id}`),
    confirm: (id: string) => client.post(`/orders/${id}/confirm`),
    fulfill: (id: string, trackingNumber: string) =>
      client.post(`/orders/${id}/fulfill`, { tracking_number: trackingNumber }),
    cancel: (id: string, reason?: string) => client.post(`/orders/${id}/cancel`, { reason }),
    refund: (id: string, amount: number) => client.post(`/orders/${id}/refund`, { amount }),
  };
}

export type { Order };
