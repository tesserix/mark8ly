import Link from "next/link";
import { notFound } from "next/navigation";
import { ArrowLeft } from "lucide-react";

import { ReviewMediaGallery } from "@/components/customers/reviews/ReviewMediaGallery";
import { ReviewReplyForm } from "@/components/customers/reviews/ReviewReplyForm";
import { ReviewReplyList } from "@/components/customers/reviews/ReviewReplyList";
import { ReviewRowActions } from "@/components/customers/reviews/ReviewRowActions";
import { ReviewStars } from "@/components/customers/reviews/ReviewStars";
import { ReviewStatusBadge } from "@/components/customers/reviews/ReviewStatusBadge";
import { getProduct, getReview } from "@/lib/api/marketplace-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

interface ReviewDetailPageProps {
  params: Promise<{ id: string }>;
}

export default async function ReviewDetailPage({
  params,
}: ReviewDetailPageProps) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = session;

  if (!currentStore) {
    notFound();
  }

  const review = await getReview(currentStore.id, id, {
    userId,
    tenantId,
  });

  if (!review) {
    notFound();
  }

  const product = await getProduct(currentStore.id, review.product_id, {
    userId,
    tenantId,
  });

  return (
          <section className="flex flex-col gap-8">
        <div className="flex flex-col gap-3">
          <Link
            href="/customers/reviews"
            className="inline-flex items-center gap-1 text-xs font-semibold uppercase tracking-wider text-foreground-secondary hover:text-foreground"
          >
            <ArrowLeft className="h-3.5 w-3.5" aria-hidden="true" />
            All reviews
          </Link>
          <div className="flex flex-col gap-2">
            <p className="eyebrow">Review</p>
            <h1 className="font-serif text-3xl font-medium tracking-tight text-foreground">
              {review.title || "Untitled review"}
            </h1>
          </div>
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
              {formatDateTime(review.created_at)}
            </span>
          </div>
        </div>

        <div className="flex flex-col-reverse gap-8 lg:flex-row lg:items-start">
          <div className="flex min-w-0 flex-1 flex-col gap-8">
            <section className="flex flex-col gap-3">
              <h2 className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
                Review
              </h2>
              <p className="whitespace-pre-wrap text-base leading-relaxed text-foreground">
                {review.content}
              </p>
            </section>

            <ReviewMediaGallery media={review.media} />

            <section
              aria-labelledby="replies-heading"
              className="flex flex-col gap-4 border-t border-border-subtle pt-6"
            >
              <h2
                id="replies-heading"
                className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
              >
                Replies
              </h2>
              <ReviewReplyList replies={review.replies} />
              <div className="pt-2">
                <ReviewReplyForm reviewId={review.id} />
              </div>
            </section>
          </div>

          <aside className="flex flex-col gap-6 lg:w-72 lg:shrink-0">
            <div className="flex flex-col gap-3 border border-border-subtle p-4">
              <h2 className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
                Actions
              </h2>
              <ReviewRowActions
                reviewId={review.id}
                status={review.status}
                featured={review.featured}
                showViewLink={false}
              />
            </div>

            <div className="flex flex-col gap-3 border border-border-subtle p-4">
              <h2 className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
                Customer
              </h2>
              <dl className="flex flex-col gap-2 text-sm text-foreground">
                <div className="flex flex-col">
                  <dt className="text-[11px] uppercase tracking-[0.12em] text-foreground-tertiary">
                    Name
                  </dt>
                  <dd>{review.customer_name || "—"}</dd>
                </div>
                <div className="flex flex-col">
                  <dt className="text-[11px] uppercase tracking-[0.12em] text-foreground-tertiary">
                    Email
                  </dt>
                  <dd className="break-all">{review.customer_email || "—"}</dd>
                </div>
              </dl>
            </div>

            <div className="flex flex-col gap-3 border border-border-subtle p-4">
              <h2 className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
                Product
              </h2>
              {product ? (
                <Link
                  href={`/products/${product.id}`}
                  className="font-serif text-base text-foreground hover:text-[color:var(--moss-700)] hover:underline underline-offset-4"
                >
                  {product.title}
                </Link>
              ) : (
                <span className="font-mono text-xs text-foreground-secondary">
                  {review.product_id}
                </span>
              )}
            </div>

            <div className="flex flex-col gap-3 border border-border-subtle p-4">
              <h2 className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
                Engagement
              </h2>
              <dl className="grid grid-cols-2 gap-3 text-sm">
                <div>
                  <dt className="text-[11px] uppercase tracking-[0.12em] text-foreground-tertiary">
                    Helpful
                  </dt>
                  <dd className="font-serif text-xl">
                    {review.helpful_count}
                  </dd>
                </div>
                <div>
                  <dt className="text-[11px] uppercase tracking-[0.12em] text-foreground-tertiary">
                    Not helpful
                  </dt>
                  <dd className="font-serif text-xl">
                    {review.not_helpful_count}
                  </dd>
                </div>
              </dl>
            </div>
          </aside>
        </div>
      </section>
  );
}

function formatDateTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}
