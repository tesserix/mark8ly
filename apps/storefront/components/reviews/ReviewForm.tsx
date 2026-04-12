"use client";

// apps/storefront/components/reviews/ReviewForm.tsx
//
// Interactive star-rating form for submitting a product review.
// Posts to the same-origin API proxy which forwards to marketplace-api.

import { useCallback, useState, useTransition } from "react";

interface ReviewFormProps {
  productHandle: string;
}

const MAX_TITLE_LENGTH = 300;
const MAX_CONTENT_LENGTH = 5000;

function StarIcon({ filled, half }: { filled: boolean; half?: boolean }) {
  return (
    <svg
      viewBox="0 0 20 20"
      className="h-6 w-6"
      fill={filled ? "currentColor" : "none"}
      stroke="currentColor"
      strokeWidth={1.2}
      aria-hidden="true"
    >
      {half ? (
        <>
          <defs>
            <clipPath id="star-half-clip">
              <rect x="0" y="0" width="10" height="20" />
            </clipPath>
          </defs>
          <path
            d="M10 1.5l2.47 5.01 5.53.8-4 3.9.94 5.49L10 14.26 4.06 16.7l.94-5.49-4-3.9 5.53-.8L10 1.5z"
            fill="currentColor"
            clipPath="url(#star-half-clip)"
          />
          <path
            d="M10 1.5l2.47 5.01 5.53.8-4 3.9.94 5.49L10 14.26 4.06 16.7l.94-5.49-4-3.9 5.53-.8L10 1.5z"
            fill="none"
          />
        </>
      ) : (
        <path d="M10 1.5l2.47 5.01 5.53.8-4 3.9.94 5.49L10 14.26 4.06 16.7l.94-5.49-4-3.9 5.53-.8L10 1.5z" />
      )}
    </svg>
  );
}

export function ReviewForm({ productHandle }: ReviewFormProps) {
  const [rating, setRating] = useState(0);
  const [hoveredRating, setHoveredRating] = useState(0);
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [isPending, startTransition] = useTransition();

  const handleSubmit = useCallback(
    (e: React.FormEvent<HTMLFormElement>) => {
      e.preventDefault();
      setError(null);

      if (rating < 1 || rating > 5) {
        setError("Please select a rating.");
        return;
      }
      if (!content.trim()) {
        setError("Please write a review.");
        return;
      }

      startTransition(async () => {
        try {
          const res = await fetch(
            `/api/reviews/${encodeURIComponent(productHandle)}`,
            {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({
                rating,
                title: title.trim() || undefined,
                content: content.trim(),
              }),
            },
          );

          if (!res.ok) {
            const body = await res.json().catch(() => null);
            const message =
              (body as { message?: string } | null)?.message ??
              "Something went wrong. Please try again.";
            setError(message);
            return;
          }

          setSuccess(true);
          setRating(0);
          setTitle("");
          setContent("");
        } catch {
          setError("Network error. Please check your connection and try again.");
        }
      });
    },
    [rating, title, content, productHandle],
  );

  if (success) {
    return (
      <div
        className="rounded-md border border-[color:var(--moss-700)]/20 bg-[color:var(--moss-700)]/5 px-5 py-4"
        role="status"
      >
        <p className="text-sm leading-6 text-[color:var(--ink-900)]">
          Thank you for your review. It will appear after moderation.
        </p>
      </div>
    );
  }

  const activeRating = hoveredRating || rating;

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-5">
      <h3 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-xl text-[color:var(--ink-900)]">
        Write a review
      </h3>

      {/* Star rating */}
      <fieldset className="flex flex-col gap-2">
        <legend className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--ink-900)] opacity-60">
          Rating
        </legend>
        <div
          className="flex gap-1"
          onMouseLeave={() => setHoveredRating(0)}
          role="radiogroup"
          aria-label="Rating"
        >
          {[1, 2, 3, 4, 5].map((star) => (
            <button
              key={star}
              type="button"
              onClick={() => setRating(star)}
              onMouseEnter={() => setHoveredRating(star)}
              className={`cursor-pointer transition-colors ${
                star <= activeRating
                  ? "text-[color:var(--ink-900)]"
                  : "text-[color:var(--ink-900)]/20"
              } focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]`}
              role="radio"
              aria-checked={star === rating}
              aria-label={`${star} star${star !== 1 ? "s" : ""}`}
            >
              <StarIcon filled={star <= activeRating} />
            </button>
          ))}
        </div>
      </fieldset>

      {/* Title (optional) */}
      <label className="flex flex-col gap-1.5">
        <span className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--ink-900)] opacity-60">
          Title{" "}
          <span className="normal-case tracking-normal opacity-60">
            (optional)
          </span>
        </span>
        <input
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          maxLength={MAX_TITLE_LENGTH}
          placeholder="Summarise your experience"
          className="rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
        />
      </label>

      {/* Content (required) */}
      <label className="flex flex-col gap-1.5">
        <span className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--ink-900)] opacity-60">
          Review
        </span>
        <textarea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          maxLength={MAX_CONTENT_LENGTH}
          rows={4}
          required
          placeholder="Share details of your experience with this product"
          className="resize-y rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-2 text-sm leading-6 text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
        />
      </label>

      {/* Error */}
      {error && (
        <p className="text-sm text-[color:var(--signal,#C23B22)]" role="alert">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={isPending}
        className="self-start rounded-md bg-[color:var(--ink-900)] px-6 py-2.5 text-sm font-semibold text-[color:var(--paper-200)] transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
      >
        {isPending ? "Submitting..." : "Submit review"}
      </button>
    </form>
  );
}
