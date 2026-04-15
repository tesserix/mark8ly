"use client";

// apps/storefront/components/reviews/ProductReviews.tsx
//
// Fetches and displays product reviews. Renders the ReviewForm for
// authenticated customers or a sign-in prompt for guests.

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { ReviewForm } from "./ReviewForm";

interface Review {
  id: string;
  rating: number;
  title: string;
  content: string;
  customer_name: string;
  created_at: string;
}

interface ReviewsResponse {
  data: Review[];
  average_rating: number;
}

interface ProductReviewsProps {
  productHandle: string;
  isAuthenticated: boolean;
}

function StarDisplay({ rating }: { rating: number }) {
  return (
    <span className="inline-flex gap-0.5" aria-label={`${rating} out of 5 stars`}>
      {[1, 2, 3, 4, 5].map((star) => (
        <svg
          key={star}
          viewBox="0 0 20 20"
          className="h-4 w-4"
          fill={star <= rating ? "currentColor" : "none"}
          stroke="currentColor"
          strokeWidth={1.2}
          aria-hidden="true"
        >
          <path d="M10 1.5l2.47 5.01 5.53.8-4 3.9.94 5.49L10 14.26 4.06 16.7l.94-5.49-4-3.9 5.53-.8L10 1.5z" />
        </svg>
      ))}
    </span>
  );
}

function formatDate(iso: string): string {
  try {
    return new Intl.DateTimeFormat(undefined, {
      year: "numeric",
      month: "long",
      day: "numeric",
    }).format(new Date(iso));
  } catch {
    return iso;
  }
}

export function ProductReviews({
  productHandle,
  isAuthenticated,
}: ProductReviewsProps) {
  const [reviews, setReviews] = useState<Review[]>([]);
  const [averageRating, setAverageRating] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchReviews = useCallback(async () => {
    try {
      const res = await fetch(
        `/api/reviews/${encodeURIComponent(productHandle)}`,
      );
      if (!res.ok) {
        setError("Unable to load reviews.");
        return;
      }
      const body: ReviewsResponse = await res.json();
      setReviews(body.data ?? []);
      setAverageRating(body.average_rating ?? 0);
    } catch {
      setError("Unable to load reviews.");
    } finally {
      setLoading(false);
    }
  }, [productHandle]);

  useEffect(() => {
    fetchReviews();
  }, [fetchReviews]);

  return (
    <section
      className="border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-12"
      aria-labelledby="reviews-heading"
    >
      <h2
        id="reviews-heading"
        className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl text-[color:var(--storefront-text,var(--ink-900))]"
      >
        Reviews
      </h2>

      {/* Average rating summary */}
      {!loading && !error && reviews.length > 0 && (
        <div className="mt-4 flex items-center gap-3">
          <span
            className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl text-[color:var(--storefront-text,var(--ink-900))]"
            style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
          >
            {averageRating.toFixed(1)}
          </span>
          <StarDisplay rating={Math.round(averageRating)} />
          <span className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
            {reviews.length} review{reviews.length !== 1 ? "s" : ""}
          </span>
        </div>
      )}

      {/* Loading state */}
      {loading && (
        <p className="mt-6 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Loading reviews...
        </p>
      )}

      {/* Error state */}
      {error && (
        <p className="mt-6 text-sm text-[color:var(--signal,#C23B22)]" role="alert">
          {error}
        </p>
      )}

      {/* Empty state */}
      {!loading && !error && reviews.length === 0 && (
        <p className="mt-6 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          No reviews yet. Be the first to share your experience.
        </p>
      )}

      {/* Review list */}
      {!loading && reviews.length > 0 && (
        <ul className="mt-8 flex flex-col gap-8">
          {reviews.map((review) => (
            <li
              key={review.id}
              className="border-b border-[color:var(--storefront-text,var(--ink-900))]/5 pb-8 last:border-b-0"
            >
              <div className="flex items-center gap-3">
                <StarDisplay rating={review.rating} />
                {review.title && (
                  <span className="text-sm font-semibold text-[color:var(--storefront-text,var(--ink-900))]">
                    {review.title}
                  </span>
                )}
              </div>
              <p className="mt-2 text-sm leading-6 text-[color:var(--storefront-text,var(--ink-900))] opacity-80">
                {review.content}
              </p>
              <p className="mt-3 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-40">
                {review.customer_name}
                {review.created_at && (
                  <> &middot; {formatDate(review.created_at)}</>
                )}
              </p>
            </li>
          ))}
        </ul>
      )}

      {/* Review form or sign-in prompt */}
      <div className="mt-10">
        {isAuthenticated ? (
          <ReviewForm productHandle={productHandle} />
        ) : (
          <div className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/10 px-5 py-4">
            <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
              <Link
                href="/sign-in"
                className="font-semibold text-[color:var(--storefront-accent,var(--moss-700))] underline underline-offset-2 transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
              >
                Sign in
              </Link>{" "}
              to write a review.
            </p>
          </div>
        )}
      </div>
    </section>
  );
}
