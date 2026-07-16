import type { createApiClient } from "./client";
import {
  customerDetailSchema,
  customerListSchema,
  type Customer,
  type CustomerDetail,
  type CustomerListResponse,
} from "./schemas/customers";

export interface ListCustomersParams {
  search?: string;
  status?: string;
  page?: string;
  page_size?: string;
}

export function createCustomersApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListCustomersParams) =>
      client.get<CustomerListResponse>(
        "/customers",
        params as Record<string, string>,
        customerListSchema,
      ),
    get: (id: string) =>
      client.get<CustomerDetail>(`/customers/${id}`, undefined, customerDetailSchema),
    /**
     * `reason` is REQUIRED by the backend (BlockCustomerRequest,
     * customers_dto.go:72-74 — `binding:"required"`). Omitting it, as this
     * client used to, is an unconditional 400.
     */
    block: (id: string, reason: string) => client.post(`/customers/${id}/block`, { reason }),
    unblock: (id: string) => client.post(`/customers/${id}/unblock`),
  };
}

export type { Customer, CustomerDetail };
