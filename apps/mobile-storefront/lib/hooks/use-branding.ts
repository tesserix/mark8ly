import { useQuery } from "@tanstack/react-query";
import type { StoreBranding } from "@repo/mobile-shared/api/storefront-types";
import { useStorefrontApi } from "@/lib/api-client";

/**
 * Pulls the merchant's runtime branding (logo, banner, palette) from
 * `/storefront/branding`. Lets a merchant tweak colors without a new
 * App Store submission — the build-time palette in app.config.js acts
 * as the offline fallback.
 */
export function useBranding() {
  const api = useStorefrontApi();
  return useQuery<StoreBranding>({
    queryKey: ["branding"],
    queryFn: () => api.get<StoreBranding>("/branding"),
    staleTime: 5 * 60_000,
  });
}
