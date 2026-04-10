import { useQuery } from "@tanstack/react-query";
import { useApiClient, useStoreSlug } from "./use-api-client";
import { fetchProfile } from "@/lib/storefront-api/customer";

export function useCustomerProfile() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery({
    queryKey: ["store", slug, "customer-profile"],
    queryFn: () => fetchProfile(client),
    select: (res) => res.data,
  });
}
