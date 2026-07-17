import { z } from "zod";

/**
 * Store branding — BrandingResponse (branding.go:55). The full DTO is a large
 * storefront-theming object (colours, fonts, SEO, homepage JSON); the mobile
 * app deliberately models + edits only the safe text basics. Zod drops the
 * unmodelled keys on parse, which is fine — the app never reads them.
 *
 * GET and PUT both return this BARE object (NOT `{data}`-wrapped). The nullable
 * fields are pointers WITH omitempty → ABSENT when unset → `.optional()`.
 * `announcement_active` / `show_powered_by` are non-pointer bools → always present.
 */
export const brandingSchema = z.object({
  id: z.string(),
  store_id: z.string(),
  logo_url: z.string().optional(),
  tagline: z.string().optional(),
  announcement_text: z.string().optional(),
  announcement_link: z.string().optional(),
  announcement_active: z.boolean(),
  footer_tagline: z.string().optional(),
  footer_copyright: z.string().optional(),
  social_instagram: z.string().optional(),
  social_twitter: z.string().optional(),
  social_facebook: z.string().optional(),
  social_tiktok: z.string().optional(),
  social_youtube: z.string().optional(),
  return_policy: z.string().optional(),
  show_powered_by: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Branding = z.infer<typeof brandingSchema>;
