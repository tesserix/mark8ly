import type { createApiClient } from "./client";
import type { Order, OrderDetail, PaginatedResponse } from "./types";

export interface ListOrdersParams {
  status?: string;
  search?: string;
  cursor?: string;
  limit?: string;
}

export function createOrdersApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListOrdersParams) =>
      client.get<PaginatedResponse<Order>>("/orders", params as Record<string, string>),
    get: (id: string) => client.get<OrderDetail>(`/orders/${id}`),
    confirm: (id: string) => client.post(`/orders/${id}/confirm`),
    fulfill: (id: string, trackingNumber: string) =>
      client.post(`/orders/${id}/fulfill`, { tracking_number: trackingNumber }),
    cancel: (id: string, reason?: string) =>
      client.post(`/orders/${id}/cancel`, { reason }),
    refund: (id: string, amount: number) =>
      client.post(`/orders/${id}/refund`, { amount }),
  };
}
