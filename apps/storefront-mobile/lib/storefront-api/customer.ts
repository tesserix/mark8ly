import type { ApiClient } from "./client-provider";

export interface CustomerProfile {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  phone?: string;
  avatar_url?: string;
  created_at: string;
}

interface CustomerProfileResponse {
  data: CustomerProfile;
}

export function fetchProfile(
  client: ApiClient,
): Promise<CustomerProfileResponse> {
  return client.get<CustomerProfileResponse>("/account");
}
