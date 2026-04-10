import type { ApiClient } from "./client-provider";
import type { StoreBranding } from "@repo/mobile-shared/api/storefront-types";

interface BrandingResponse {
  data: StoreBranding;
}

export function fetchStoreBranding(
  client: ApiClient,
): Promise<BrandingResponse> {
  return client.get<BrandingResponse>("/branding");
}
