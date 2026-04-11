import Link from "next/link";

import type { AdminReview } from "@/lib/api/marketplace-api";

import { ReviewRowActions } from "./ReviewRowActions";
import { ReviewStars } from "./ReviewStars";
import { ReviewStatusBadge } from "./ReviewStatusBadge";

interface ReviewsListProps {
  reviews: AdminReview[];
  productTitles?: Record<string, string>;
}

export function ReviewsList({
  reviews,
  productTitles = {},
}: ReviewsListProps) {
  return (
    <ul role="list" className="flex flex-col">
      {reviews.map((review) => {
        const productTitle = productTitles[review.product_id];
        return (
          <li
            key={review.id}
            className="border-b border-border-subtle py-6 first:border-t"
          >
            <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div className="flex min-w-0 flex-1 flex-col gap-3">
                <div className="flex flex-wrap items-center gap-3">
                  <ReviewStars rating={review.rating} />
                  <ReviewStatusBadge status={review.status} />
                  {review.featured && (
                    <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-[color:var(--moss-700)]">
                      Featured
                    </span>
                  )}
                  {review.verified_purchase && (
                    <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-foreground-tertiary">
                      Verified purchase
                    </span>
                  )}
                  <span className="text-xs text-foreground-tertiary">
                    {formatDate(review.created_at)}
                  </span>
                </div>

                {review.title && (
                  <Link
                    href={`/customers/reviews/${review.id}`}
                    className="font-serif text-lg text-foreground hover:underline underline-offset-4"
                  >
                    {review.title}
                  </Link>
                )}

                <p className="line-clamp-3 text-sm leading-relaxed text-foreground-secondary">
                  {review.content}
                </p>

                <dl className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-foreground-tertiary">
                  <div className="flex gap-1">
                    <dt className="font-semibold uppercase tracking-[0.12em]">
                      Customer
                    </dt>
                    <dd>
                      {review.customer_name ||
                        review.customer_email ||
                        "—"}
                    </dd>
                  </div>
                  <div className="flex gap-1">
                    <dt className="font-semibold uppercase tracking-[0.12em]">
                      Product
                    </dt>
                    <dd>
                      <Link
                        href={`/products/${review.product_id}`}
                        className="hover:text-foreground hover:underline underline-offset-2"
                      >
                        {productTitle ?? shortId(review.product_id)}
                      </Link>
                    </dd>
                  </div>
                  {review.replies.length > 0 && (
                    <div className="flex gap-1">
                      <dt className="font-semibold uppercase tracking-[0.12em]">
                        Replies
                      </dt>
                      <dd>{review.replies.length}</dd>
                    </div>
                  )}
                </dl>
              </div>

              <div className="lg:ml-6 lg:shrink-0">
                <ReviewRowActions
                  reviewId={review.id}
                  status={review.status}
                  featured={review.featured}
                />
              </div>
            </div>
          </li>
        );
      })}
    </ul>
  );
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
    });
  } catch {
    return iso;
  }
}

function shortId(id: string): string {
  return id.length > 8 ? `${id.slice(0, 8)}…` : id;
}
