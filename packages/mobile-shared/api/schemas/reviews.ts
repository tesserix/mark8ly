import { z } from "zod";
import { paginated } from "../schema-helpers";

/**
 * Wire truth for the admin review endpoints, from reviews_dto.go
 * (AdminReviewResponse / AdminReviewMediaDTO / AdminReviewReplyDTO). Every
 * optional field is a Go `omitempty` — ABSENT from the JSON, not null — so
 * `.optional()`, never `.nullable()` (same rule as customers.ts).
 *
 * Response envelopes differ by endpoint (matching reviews.go):
 *   - LIST → `{data, meta}` (use `reviewListSchema`)
 *   - get / approve / reject / featured / reply → a BARE review object
 *     (validate `reviewSchema` directly; do NOT wrap in `{data}`).
 */
export const reviewStatusSchema = z.enum(["pending", "approved", "rejected"]);
export type ReviewStatus = z.infer<typeof reviewStatusSchema>;

/** AdminReviewMediaDTO. media_type is "image" | "video". */
export const reviewMediaSchema = z.object({
  id: z.string(),
  url: z.string(),
  alt: z.string().optional(),
  position: z.number(),
  media_type: z.string(),
  width: z.number().optional(),
  height: z.number().optional(),
  file_size: z.number().optional(),
});
export type ReviewMedia = z.infer<typeof reviewMediaSchema>;

/**
 * AdminReviewReplyDTO. author_type is "merchant" | "customer". The backend
 * does NOT emit `parent_reply_id` (the field the web tree reads is dormant),
 * so replies render flat here — do not invent it.
 */
export const reviewReplySchema = z.object({
  id: z.string(),
  author_type: z.string(),
  author_name: z.string(),
  author_email: z.string().optional(),
  content: z.string(),
  created_at: z.string(),
});
export type ReviewReply = z.infer<typeof reviewReplySchema>;

/**
 * AdminReviewResponse. `media` and `replies` are always emitted arrays
 * (the handler builds them with make([]..., 0, ...)), so they are required,
 * not optional.
 */
export const reviewSchema = z.object({
  id: z.string(),
  product_id: z.string(),
  customer_profile_id: z.string().optional(),
  customer_name: z.string(),
  customer_email: z.string(),
  rating: z.number(),
  title: z.string().optional(),
  content: z.string(),
  status: reviewStatusSchema,
  verified_purchase: z.boolean(),
  featured: z.boolean(),
  helpful_count: z.number(),
  not_helpful_count: z.number(),
  published_at: z.string().optional(),
  created_at: z.string(),
  updated_at: z.string(),
  media: z.array(reviewMediaSchema),
  replies: z.array(reviewReplySchema),
});
export type Review = z.infer<typeof reviewSchema>;

export const reviewListSchema = paginated(reviewSchema);
export type ReviewListResponse = z.infer<typeof reviewListSchema>;

/** POST /reviews/:id/reply body — ReplyRequest (required, max 5000). */
export const replyBodySchema = z.object({
  content: z.string().trim().min(1).max(5000),
});
export type ReplyBody = z.infer<typeof replyBodySchema>;
