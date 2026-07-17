import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createBrandingApi, type BrandingUpdateBody } from "@repo/mobile-shared/api/branding";
import type { Branding } from "@repo/mobile-shared/api/types";
import { useApiClient } from "@/lib/api-client";

export function useBranding() {
  const client = useApiClient();
  const api = createBrandingApi(client);

  return useQuery<Branding>({
    queryKey: ["branding"],
    queryFn: () => api.get(),
    refetchOnWindowFocus: true,
  });
}

export function useUpdateBranding() {
  const client = useApiClient();
  const api = createBrandingApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: BrandingUpdateBody) => api.update(body),
    onSuccess: (data) => {
      // The PUT returns the full updated object — seed the cache directly.
      queryClient.setQueryData(["branding"], data);
    },
  });
}
