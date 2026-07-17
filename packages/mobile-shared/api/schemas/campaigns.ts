import { z } from "zod";
import { money, paginated } from "../schema-helpers";

/**
 * Wire truth for campaigns — CampaignResponse (campaigns_dto.go:44-68).
 * subject/content/segment_id/coupon_id/scheduled_at/sent_at are pointers
 * WITHOUT omitempty → JSON `null` → `.nullable()`. storefront_label IS
 * omitempty → absent → `.optional()`. `revenue` is a StringFixed(2) money
 * string → `money`.
 */
export const campaignSchema = z.object({
  id: z.string(),
  name: z.string(),
  // "email" (default)
  type: z.string(),
  // "draft" | "scheduled" | "sending" | "sent" | "paused" | ...
  status: z.string(),
  subject: z.string().nullable(),
  content: z.string().nullable(),
  segment_id: z.string().nullable(),
  coupon_id: z.string().nullable(),
  scheduled_at: z.string().nullable(),
  sent_at: z.string().nullable(),
  total_recipients: z.number(),
  delivered: z.number(),
  opened: z.number(),
  clicked: z.number(),
  converted: z.number(),
  unsubscribed: z.number(),
  failed: z.number(),
  revenue: money,
  show_on_storefront: z.boolean(),
  storefront_label: z.string().optional(),
  storefront_priority: z.number(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Campaign = z.infer<typeof campaignSchema>;

/** LIST envelope: standard `{data, meta}` (campaigns.go:71). */
export const campaignListSchema = paginated(campaignSchema);
export type CampaignListResponse = z.infer<typeof campaignListSchema>;
