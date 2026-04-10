import type { createApiClient } from "./client";
import type { Customer, CustomerDetail, PaginatedResponse } from "./types";

export interface ListCustomersParams {
  search?: string;
  cursor?: string;
  limit?: string;
}

export function createCustomersApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListCustomersParams) =>
      client.get<PaginatedResponse<Customer>>("/customers", params as Record<string, string>),
    get: (id: string) => client.get<CustomerDetail>(`/customers/${id}`),
    block: (id: string) => client.post(`/customers/${id}/block`),
    unblock: (id: string) => client.post(`/customers/${id}/unblock`),
  };
}
