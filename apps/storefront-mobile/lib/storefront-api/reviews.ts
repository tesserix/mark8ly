import type { ApiClient } from "./client-provider";

export interface CustomerReview {
  id: string;
  product_id: string;
  product_handle: string;
  product_title: string;
  product_thumbnail?: string;
  rating: number;
  title: string;
  body: string;
  created_at: string;
}

export interface SubmitReviewBody {
  rating: number;
  title: string;
  body: string;
}

interface ReviewListResponse {
  data: CustomerReview[];
}

interface ReviewResponse {
  data: CustomerReview;
}

export function listMyReviews(
  client: ApiClient,
): Promise<ReviewListResponse> {
  return client.get<ReviewListResponse>("/my-reviews");
}

export function submitReview(
  client: ApiClient,
  handle: string,
  body: SubmitReviewBody,
): Promise<ReviewResponse> {
  return client.post<ReviewResponse>(`/products/${handle}/reviews`, body);
}
