import type { ApiClient } from "./client-provider";

export interface Address {
  id: string;
  first_name: string;
  last_name: string;
  line1: string;
  line2?: string;
  city: string;
  state: string;
  postal_code: string;
  country: string;
  phone?: string;
  is_default: boolean;
}

export type AddressInput = Omit<Address, "id" | "is_default"> & {
  is_default?: boolean;
};

interface AddressListResponse {
  data: Address[];
}

interface AddressResponse {
  data: Address;
}

export function listAddresses(
  client: ApiClient,
): Promise<AddressListResponse> {
  return client.get<AddressListResponse>("/account/addresses");
}

export function createAddress(
  client: ApiClient,
  body: AddressInput,
): Promise<AddressResponse> {
  return client.post<AddressResponse>("/account/addresses", body);
}

export function updateAddress(
  client: ApiClient,
  id: string,
  body: Partial<AddressInput>,
): Promise<AddressResponse> {
  return client.patch<AddressResponse>(`/account/addresses/${id}`, body);
}

export function deleteAddress(
  client: ApiClient,
  id: string,
): Promise<void> {
  return client.delete(`/account/addresses/${id}`);
}
