import { useQuery } from "@tanstack/react-query";
import { useApiClient, useStoreSlug } from "./use-api-client";
import { fetchStoreBranding } from "@/lib/storefront-api/store";
import type { StoreBranding } from "@repo/mobile-shared/api/storefront-types";

export function useStoreBranding() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery<StoreBranding>({
    queryKey: ["store", slug, "branding"],
    queryFn: async () => {
      const result = await fetchStoreBranding(client);
      return result.data;
    },
    staleTime: 5 * 60_000, // branding changes rarely
  });
}
