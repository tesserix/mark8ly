import type { createApiClient } from "./client";
import {
  reviewListSchema,
  reviewSchema,
  type Review,
  type ReviewListResponse,
} from "./schemas/reviews";

export interface ListReviewsParams {
  /** "pending" | "approved" | "rejected"; omitted means all. */
  status?: string;
  page?: string;
  page_size?: string;
}

/**
 * Admin review moderation. Mirrors the WEB routes (routes.go:590-611), now
 * exposed on the mobile group too. `list` returns the `{data, meta}` envelope;
 * every mutation returns a BARE review object (validate reviewSchema directly).
 */
export function createReviewsApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListReviewsParams) =>
      client.get<ReviewListResponse>(
        "/reviews",
        params as Record<string, string>,
        reviewListSchema,
      ),
    get: (id: string) => client.get<Review>(`/reviews/${id}`, undefined, reviewSchema),
    approve: (id: string) => client.post<Review>(`/reviews/${id}/approve`, undefined, reviewSchema),
    reject: (id: string) => client.post<Review>(`/reviews/${id}/reject`, undefined, reviewSchema),
    setFeatured: (id: string, featured: boolean) =>
      client.post<Review>(`/reviews/${id}/featured`, { featured }, reviewSchema),
    reply: (id: string, content: string) =>
      client.post<Review>(`/reviews/${id}/reply`, { content }, reviewSchema),
  };
}

export type { Review, ReviewListResponse };
