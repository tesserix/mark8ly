"use client";

// apps/storefront/components/reviews/ProductReviews.tsx
//
// Renders approved product reviews with three-way reactions (helpful /
// not helpful / useful, one active per user) and a nested comments
// thread under each review. Anonymous visitors see counts and can read
// the thread; authenticated customers can react + post comments +
// reply to other comments.

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { ReviewForm } from "./ReviewForm";

type ReactionKey = "helpful" | "not_helpful" | "useful";

interface ReviewReply {
  id: string;
  parent_reply_id?: string | null;
  author_type: "customer" | "merchant";
  author_name: string;
  content: string;
  created_at: string;
}

interface Review {
  id: string;
  rating: number;
  title: string;
  content: string;
  customer_name: string;
  created_at: string;
  helpful_count: number;
  not_helpful_count: number;
  useful_count: number;
  viewer_reaction?: ReactionKey | "";
  replies?: ReviewReply[];
}

interface ReviewsResponse {
  data: Review[];
  meta?: { average_rating?: number };
  average_rating?: number;
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

const REACTION_META: Record<ReactionKey, { label: string; emoji: string }> = {
  helpful: { label: "Helpful", emoji: "👍" },
  not_helpful: { label: "Not helpful", emoji: "👎" },
  useful: { label: "Useful", emoji: "💡" },
};

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
      setAverageRating(body.meta?.average_rating ?? body.average_rating ?? 0);
    } catch {
      setError("Unable to load reviews.");
    } finally {
      setLoading(false);
    }
  }, [productHandle]);

  useEffect(() => {
    fetchReviews();
  }, [fetchReviews]);

  // React (or switch reaction). Optimistic update with rollback on
  // server error so the count increments feel immediate. The backend
  // uses UPSERT under a UNIQUE(review_id, customer_profile_id) so
  // switching from 👍 to 👎 replaces the row; the count math mirrors
  // that: decrement the old bucket, increment the new one.
  const onReact = useCallback(
    async (reviewId: string, next: ReactionKey) => {
      if (!isAuthenticated) return;
      setReviews((prev) => prev.map((r) => applyReaction(r, reviewId, next)));
      try {
        const res = await fetch(`/api/reviews/${encodeURIComponent(productHandle)}/reactions/${encodeURIComponent(reviewId)}`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ reaction: next }),
        });
        if (!res.ok) throw new Error(`status ${res.status}`);
      } catch {
        // Rollback by refetching the authoritative state.
        await fetchReviews();
      }
    },
    [fetchReviews, isAuthenticated, productHandle],
  );

  const onComment = useCallback(
    async (reviewId: string, content: string, parentReplyId?: string) => {
      if (!isAuthenticated || !content.trim()) return;
      const res = await fetch(`/api/reviews/${encodeURIComponent(productHandle)}/replies/${encodeURIComponent(reviewId)}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          content: content.trim(),
          parent_reply_id: parentReplyId ?? null,
        }),
      });
      if (!res.ok) {
        return { ok: false as const, error: "Couldn't post your comment." };
      }
      const body = (await res.json()) as { data: ReviewReply };
      setReviews((prev) =>
        prev.map((r) =>
          r.id === reviewId
            ? { ...r, replies: [...(r.replies ?? []), body.data] }
            : r,
        ),
      );
      return { ok: true as const };
    },
    [isAuthenticated, productHandle],
  );

  return (
    <section
      className="border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-12"
      aria-labelledby="reviews-heading"
    >
      <h2
        id="reviews-heading"
        className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-3xl text-[color:var(--storefront-text,var(--ink-900))]"
      >
        Reviews
      </h2>

      {!loading && !error && reviews.length > 0 && (
        <div className="mt-4 flex items-center gap-3">
          <span
            className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl text-[color:var(--storefront-text,var(--ink-900))]"
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

      {loading && (
        <p className="mt-6 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Loading reviews...
        </p>
      )}

      {error && (
        <p className="mt-6 text-sm text-[color:var(--storefront-danger)]" role="alert">
          {error}
        </p>
      )}

      {!loading && !error && reviews.length === 0 && (
        <p className="mt-6 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          No reviews yet. Be the first to share your experience.
        </p>
      )}

      {!loading && reviews.length > 0 && (
        <ul className="mt-8 flex flex-col gap-10">
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

              <ReactionButtons
                review={review}
                disabled={!isAuthenticated}
                onReact={onReact}
              />

              <CommentsThread
                review={review}
                isAuthenticated={isAuthenticated}
                onComment={onComment}
              />
            </li>
          ))}
        </ul>
      )}

      <div className="mt-10">
        {/* Anonymous and authenticated visitors both see the form.
            ReviewForm branches internally on isAuthenticated to ask for
            name + email when there's no customer session. */}
        <ReviewForm
          productHandle={productHandle}
          isAuthenticated={isAuthenticated}
        />
        {!isAuthenticated && (
          <p className="mt-3 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
            Have an account?{" "}
            <Link
              href="/sign-in"
              className="font-semibold underline underline-offset-2 transition-opacity hover:opacity-80"
            >
              Sign in
            </Link>{" "}
            to skip entering your details.
          </p>
        )}
      </div>
    </section>
  );
}

// ─── Reaction row ──────────────────────────────────────────────────────

function ReactionButtons({
  review,
  disabled,
  onReact,
}: {
  review: Review;
  disabled: boolean;
  onReact: (reviewId: string, k: ReactionKey) => void | Promise<void>;
}) {
  const keys: ReactionKey[] = ["helpful", "not_helpful", "useful"];
  const counts: Record<ReactionKey, number> = {
    helpful: review.helpful_count,
    not_helpful: review.not_helpful_count,
    useful: review.useful_count,
  };
  return (
    <div className="mt-4 flex flex-wrap items-center gap-2" role="group" aria-label="React to this review">
      {keys.map((k) => {
        const active = review.viewer_reaction === k;
        return (
          <button
            key={k}
            type="button"
            disabled={disabled}
            onClick={() => onReact(review.id, k)}
            aria-pressed={active}
            data-reaction={k}
            className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs transition-colors ${
              active
                ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]/10 text-[color:var(--storefront-text,var(--ink-900))]"
                : "border-[color:var(--storefront-text,var(--ink-900))]/15 text-[color:var(--storefront-text,var(--ink-900))]/70 hover:border-[color:var(--storefront-text,var(--ink-900))]/30"
            } ${disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer"}`}
            title={disabled ? "Sign in to react" : REACTION_META[k].label}
          >
            <span aria-hidden="true">{REACTION_META[k].emoji}</span>
            <span>{REACTION_META[k].label}</span>
            <span className="tabular-nums opacity-60">{counts[k] ?? 0}</span>
          </button>
        );
      })}
    </div>
  );
}

// ─── Comments thread ───────────────────────────────────────────────────

function CommentsThread({
  review,
  isAuthenticated,
  onComment,
}: {
  review: Review;
  isAuthenticated: boolean;
  onComment: (
    reviewId: string,
    content: string,
    parentReplyId?: string,
  ) => Promise<{ ok: boolean; error?: string } | void>;
}) {
  const replies = review.replies ?? [];
  const [open, setOpen] = useState<boolean>(replies.length > 0);
  const [composer, setComposer] = useState("");
  const [replyingTo, setReplyingTo] = useState<string | null>(null);
  const [replyBody, setReplyBody] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Build a top-level → children map keyed by parent_reply_id so the
  // render loop stays O(n) rather than nested filter calls.
  const { topLevel, childrenByParent } = useMemo(() => {
    const top: ReviewReply[] = [];
    const byParent = new Map<string, ReviewReply[]>();
    for (const r of replies) {
      const parent = r.parent_reply_id ?? null;
      if (!parent) {
        top.push(r);
      } else {
        const arr = byParent.get(parent) ?? [];
        arr.push(r);
        byParent.set(parent, arr);
      }
    }
    top.sort((a, b) => a.created_at.localeCompare(b.created_at));
    byParent.forEach((arr) =>
      arr.sort((a, b) => a.created_at.localeCompare(b.created_at)),
    );
    return { topLevel: top, childrenByParent: byParent };
  }, [replies]);

  async function submitTopLevel() {
    const text = composer.trim();
    if (!text) return;
    setSubmitting(true);
    setError(null);
    const r = await onComment(review.id, text);
    setSubmitting(false);
    if (r && r.ok === false) {
      setError(r.error ?? "Couldn't post.");
      return;
    }
    setComposer("");
  }

  async function submitNested(parentReplyId: string) {
    const text = replyBody.trim();
    if (!text) return;
    setSubmitting(true);
    setError(null);
    const r = await onComment(review.id, text, parentReplyId);
    setSubmitting(false);
    if (r && r.ok === false) {
      setError(r.error ?? "Couldn't post.");
      return;
    }
    setReplyBody("");
    setReplyingTo(null);
  }

  return (
    <div className="mt-4">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="text-xs font-medium uppercase tracking-[0.12em] text-[color:var(--storefront-accent,var(--moss-700))] hover:underline"
        aria-expanded={open}
        aria-controls={`thread-${review.id}`}
      >
        {open ? "Hide comments" : replies.length > 0 ? `Show ${replies.length} comment${replies.length === 1 ? "" : "s"}` : "Add a comment"}
      </button>
      {open && (
        <div
          id={`thread-${review.id}`}
          className="mt-3 space-y-4 border-l border-[color:var(--storefront-text,var(--ink-900))]/10 pl-4"
        >
          {topLevel.length === 0 && replies.length === 0 && (
            <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
              No comments yet.
            </p>
          )}

          {topLevel.map((reply) => (
            <CommentNode
              key={reply.id}
              reply={reply}
              children={childrenByParent.get(reply.id) ?? []}
              isAuthenticated={isAuthenticated}
              onStartReply={() => {
                setReplyingTo(reply.id);
                setReplyBody("");
                setError(null);
              }}
              isReplying={replyingTo === reply.id}
              replyBody={replyBody}
              onReplyChange={setReplyBody}
              onReplySubmit={() => submitNested(reply.id)}
              onReplyCancel={() => {
                setReplyingTo(null);
                setReplyBody("");
                setError(null);
              }}
              submitting={submitting}
            />
          ))}

          {isAuthenticated && replyingTo === null && (
            <form
              className="flex flex-col gap-2 pt-2"
              onSubmit={(e) => {
                e.preventDefault();
                void submitTopLevel();
              }}
            >
              <textarea
                value={composer}
                onChange={(e) => setComposer(e.target.value)}
                rows={2}
                maxLength={5000}
                placeholder="Add a comment..."
                className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-transparent px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-40 focus:outline-2 focus:outline-offset-2 focus:outline-[color:var(--storefront-accent,var(--moss-700))]"
                disabled={submitting}
              />
              {error && (
                <p className="text-xs text-[color:var(--storefront-danger)]" role="alert">
                  {error}
                </p>
              )}
              <div>
                <button
                  type="submit"
                  disabled={submitting || !composer.trim()}
                  className="inline-flex items-center rounded-md bg-[color:var(--storefront-text,var(--ink-900))] px-3 py-1.5 text-xs font-medium text-[color:var(--storefront-on-accent,var(--paper-200))] hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {submitting ? "Posting..." : "Post comment"}
                </button>
              </div>
            </form>
          )}

          {!isAuthenticated && (
            <p className="pt-2 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
              <Link
                href="/sign-in"
                className="font-semibold text-[color:var(--storefront-accent,var(--moss-700))] underline underline-offset-2"
              >
                Sign in
              </Link>{" "}
              to join the discussion.
            </p>
          )}
        </div>
      )}
    </div>
  );
}

// A single comment node (optionally with nested children indented one
// level deeper). Deeper nesting is supported by the data model but
// intentionally flattened in the UI to keep threads readable.
function CommentNode({
  reply,
  children,
  isAuthenticated,
  onStartReply,
  isReplying,
  replyBody,
  onReplyChange,
  onReplySubmit,
  onReplyCancel,
  submitting,
}: {
  reply: ReviewReply;
  children: ReviewReply[];
  isAuthenticated: boolean;
  onStartReply: () => void;
  isReplying: boolean;
  replyBody: string;
  onReplyChange: (v: string) => void;
  onReplySubmit: () => void | Promise<void>;
  onReplyCancel: () => void;
  submitting: boolean;
}) {
  const isMerchant = reply.author_type === "merchant";
  return (
    <div className="space-y-2">
      <div>
        <div className="flex items-center gap-2 text-xs text-[color:var(--storefront-text,var(--ink-900))]/50">
          <span className="font-semibold text-[color:var(--storefront-text,var(--ink-900))]/80">
            {reply.author_name}
          </span>
          {isMerchant && (
            <span className="inline-flex items-center rounded-full bg-[color:var(--storefront-accent,var(--moss-700))]/15 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-[color:var(--storefront-accent,var(--moss-700))]">
              Store team
            </span>
          )}
          <span>&middot; {formatDate(reply.created_at)}</span>
        </div>
        <p className="mt-1 text-sm leading-5 text-[color:var(--storefront-text,var(--ink-900))]/85 whitespace-pre-wrap">
          {reply.content}
        </p>
        {isAuthenticated && !isReplying && (
          <button
            type="button"
            onClick={onStartReply}
            className="mt-1 text-[11px] font-medium text-[color:var(--storefront-accent,var(--moss-700))] hover:underline"
          >
            Reply
          </button>
        )}
      </div>

      {isReplying && (
        <form
          className="ml-4 flex flex-col gap-2 border-l border-[color:var(--storefront-text,var(--ink-900))]/10 pl-3"
          onSubmit={(e) => {
            e.preventDefault();
            void onReplySubmit();
          }}
        >
          <textarea
            value={replyBody}
            onChange={(e) => onReplyChange(e.target.value)}
            rows={2}
            maxLength={5000}
            autoFocus
            placeholder={`Reply to ${reply.author_name}...`}
            className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-transparent px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-40 focus:outline-2 focus:outline-offset-2 focus:outline-[color:var(--storefront-accent,var(--moss-700))]"
            disabled={submitting}
          />
          <div className="flex items-center gap-2">
            <button
              type="submit"
              disabled={submitting || !replyBody.trim()}
              className="inline-flex items-center rounded-md bg-[color:var(--storefront-text,var(--ink-900))] px-3 py-1.5 text-xs font-medium text-[color:var(--storefront-on-accent,var(--paper-200))] hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {submitting ? "Posting..." : "Post reply"}
            </button>
            <button
              type="button"
              onClick={onReplyCancel}
              disabled={submitting}
              className="text-[11px] text-[color:var(--storefront-text,var(--ink-900))]/60 hover:opacity-100"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      {children.length > 0 && (
        <div className="ml-4 space-y-3 border-l border-[color:var(--storefront-text,var(--ink-900))]/10 pl-3">
          {children.map((child) => (
            <div key={child.id}>
              <div className="flex items-center gap-2 text-xs text-[color:var(--storefront-text,var(--ink-900))]/50">
                <span className="font-semibold text-[color:var(--storefront-text,var(--ink-900))]/80">
                  {child.author_name}
                </span>
                {child.author_type === "merchant" && (
                  <span className="inline-flex items-center rounded-full bg-[color:var(--storefront-accent,var(--moss-700))]/15 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-[color:var(--storefront-accent,var(--moss-700))]">
                    Store team
                  </span>
                )}
                <span>&middot; {formatDate(child.created_at)}</span>
              </div>
              <p className="mt-1 text-sm leading-5 text-[color:var(--storefront-text,var(--ink-900))]/85 whitespace-pre-wrap">
                {child.content}
              </p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ─── Optimistic reaction math ──────────────────────────────────────────

function applyReaction(r: Review, id: string, next: ReactionKey): Review {
  if (r.id !== id) return r;
  const prev = r.viewer_reaction;
  if (prev === next) return r; // clicking the active button is a no-op
  const counts = {
    helpful: r.helpful_count,
    not_helpful: r.not_helpful_count,
    useful: r.useful_count,
  };
  if (prev) {
    counts[prev as ReactionKey] = Math.max(0, counts[prev as ReactionKey] - 1);
  }
  counts[next] += 1;
  return {
    ...r,
    helpful_count: counts.helpful,
    not_helpful_count: counts.not_helpful,
    useful_count: counts.useful,
    viewer_reaction: next,
  };
}
