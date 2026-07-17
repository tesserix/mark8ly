import type { createApiClient } from "./client";
import { brandingSchema, type Branding } from "./schemas/branding";

/**
 * Partial branding update (UpdateBrandingRequest — all fields optional, sent
 * only when changed). The mobile app edits only the text basics; colours,
 * fonts, SEO, logo upload and homepage content stay on the web dashboard.
 */
export interface BrandingUpdateBody {
  tagline?: string;
  announcement_text?: string;
  announcement_link?: string;
  announcement_active?: boolean;
  footer_tagline?: string;
  footer_copyright?: string;
  social_instagram?: string;
  social_twitter?: string;
  social_facebook?: string;
  social_tiktok?: string;
  social_youtube?: string;
  return_policy?: string;
  show_powered_by?: boolean;
}

/**
 * Store branding. Mirrors web routes.go:849-862 (GET + PUT; the upload-url
 * route is intentionally not exposed on mobile). Both endpoints return the
 * BARE BrandingResponse — no `{data}` wrapper.
 */
export function createBrandingApi(client: ReturnType<typeof createApiClient>) {
  return {
    get: () => client.get<Branding>("/branding", undefined, brandingSchema),
    update: (body: BrandingUpdateBody) => client.put<Branding>("/branding", body, brandingSchema),
  };
}

export type { Branding };
