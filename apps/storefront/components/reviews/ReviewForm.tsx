"use client";

// apps/storefront/components/reviews/ReviewForm.tsx
//
// Interactive star-rating form for submitting a product review.
// Posts to the same-origin API proxy which forwards to marketplace-api.

import { useCallback, useState, useTransition } from "react";

interface ReviewFormProps {
  productHandle: string;
  // When false, the form collects name + email and posts to the
  // public `/guest` endpoint. When true it reuses the customer
  // session and posts to the authenticated endpoint.
  isAuthenticated: boolean;
}

const MAX_TITLE_LENGTH = 300;
const MAX_CONTENT_LENGTH = 5000;
const MAX_NAME_LENGTH = 120;
const MAX_EMAIL_LENGTH = 320;

const GUEST_IDENTITY_STORAGE_KEY = "mp_guest_identity";

function loadGuestIdentity(): { name: string; email: string } {
  if (typeof window === "undefined") return { name: "", email: "" };
  try {
    const raw = window.localStorage.getItem(GUEST_IDENTITY_STORAGE_KEY);
    if (!raw) return { name: "", email: "" };
    const parsed = JSON.parse(raw) as { name?: string; email?: string };
    return { name: parsed.name ?? "", email: parsed.email ?? "" };
  } catch {
    return { name: "", email: "" };
  }
}

function saveGuestIdentity(name: string, email: string): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(
      GUEST_IDENTITY_STORAGE_KEY,
      JSON.stringify({ name, email }),
    );
  } catch {
    /* ignore storage errors */
  }
}

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

export function ReviewForm({ productHandle, isAuthenticated }: ReviewFormProps) {
  const [rating, setRating] = useState(0);
  const [hoveredRating, setHoveredRating] = useState(0);
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [guestName, setGuestName] = useState(() =>
    isAuthenticated ? "" : loadGuestIdentity().name,
  );
  const [guestEmail, setGuestEmail] = useState(() =>
    isAuthenticated ? "" : loadGuestIdentity().email,
  );
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
      if (!isAuthenticated) {
        if (!guestName.trim()) {
          setError("Please enter your name.");
          return;
        }
        if (!guestEmail.trim()) {
          setError("Please enter your email.");
          return;
        }
      }

      startTransition(async () => {
        try {
          const endpoint = isAuthenticated
            ? `/api/reviews/${encodeURIComponent(productHandle)}`
            : `/api/reviews/${encodeURIComponent(productHandle)}/guest`;
          const payload = isAuthenticated
            ? {
                rating,
                title: title.trim() || undefined,
                content: content.trim(),
              }
            : {
                rating,
                title: title.trim() || undefined,
                content: content.trim(),
                customer_name: guestName.trim(),
                customer_email: guestEmail.trim(),
              };
          const res = await fetch(endpoint, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
          });

          if (!res.ok) {
            const body = await res.json().catch(() => null);
            const message =
              (body as { message?: string } | null)?.message ??
              "Something went wrong. Please try again.";
            setError(message);
            return;
          }

          // Remember the guest identity for the next comment/review
          // on this browser so the visitor doesn't have to retype.
          if (!isAuthenticated) {
            saveGuestIdentity(guestName.trim(), guestEmail.trim());
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
    [
      rating,
      title,
      content,
      productHandle,
      isAuthenticated,
      guestName,
      guestEmail,
    ],
  );

  if (success) {
    return (
      <div
        className="rounded-md border border-[color:var(--storefront-accent,var(--moss-700))]/20 bg-[color:var(--storefront-accent,var(--moss-700))]/5 px-5 py-4"
        role="status"
      >
        <p className="text-sm leading-6 text-[color:var(--storefront-text,var(--ink-900))]">
          Thank you for your review. It will appear after moderation.
        </p>
      </div>
    );
  }

  const activeRating = hoveredRating || rating;

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-5">
      <h3 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-xl text-[color:var(--storefront-text,var(--ink-900))]">
        Write a review
      </h3>

      {/* Star rating */}
      <fieldset className="flex flex-col gap-2">
        <legend className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
          Rating
        </legend>
        <div
          className="flex gap-1"
          onMouseLeave={() => setHoveredRating(0)}
          role="radiogroup"
          aria-label="Rating"
          aria-required="true"
          aria-invalid={error && !rating ? true : undefined}
          aria-describedby={error ? "review-form-error" : undefined}
        >
          {[1, 2, 3, 4, 5].map((star) => (
            <button
              key={star}
              type="button"
              onClick={() => setRating(star)}
              onMouseEnter={() => setHoveredRating(star)}
              className={`cursor-pointer transition-colors ${
                star <= activeRating
                  ? "text-[color:var(--storefront-text,var(--ink-900))]"
                  : "text-[color:var(--storefront-text,var(--ink-900))]/20"
              } focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]`}
              role="radio"
              aria-checked={star === rating}
              aria-label={`${star} star${star !== 1 ? "s" : ""}`}
            >
              <StarIcon filled={star <= activeRating} />
            </button>
          ))}
        </div>
      </fieldset>

      {/* Guest identity — only shown when no customer session.
          We need name + email to attribute the review and enforce
          the UNIQUE(store, product, email) invariant on re-submit. */}
      {!isAuthenticated && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
              Name
            </span>
            <input
              type="text"
              value={guestName}
              onChange={(e) => setGuestName(e.target.value)}
              maxLength={MAX_NAME_LENGTH}
              required
              autoComplete="name"
              placeholder="Your name"
              aria-describedby={error ? "review-form-error" : undefined}
              className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-surface)] px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:text-[color:var(--storefront-text,var(--ink-900))]/30 focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]"
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
              Email
            </span>
            <input
              type="email"
              value={guestEmail}
              onChange={(e) => setGuestEmail(e.target.value)}
              maxLength={MAX_EMAIL_LENGTH}
              required
              autoComplete="email"
              placeholder="you@example.com"
              aria-describedby={error ? "review-form-error" : undefined}
              className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-surface)] px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:text-[color:var(--storefront-text,var(--ink-900))]/30 focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]"
            />
          </label>
        </div>
      )}

      {/* Title (optional) */}
      <label className="flex flex-col gap-1.5">
        <span className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
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
          className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-surface)] px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:text-[color:var(--storefront-text,var(--ink-900))]/30 focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]"
        />
      </label>

      {/* Content (required) */}
      <label className="flex flex-col gap-1.5">
        <span className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
          Review
        </span>
        <textarea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          maxLength={MAX_CONTENT_LENGTH}
          rows={4}
          required
          placeholder="Share details of your experience with this product"
          aria-describedby={error ? "review-form-error" : undefined}
          aria-invalid={error ? true : undefined}
          className="resize-y rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-surface)] px-3 py-2 text-sm leading-6 text-[color:var(--storefront-text,var(--ink-900))] placeholder:text-[color:var(--storefront-text,var(--ink-900))]/30 focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]"
        />
      </label>

      {/* Error */}
      {error && (
        <p
          id="review-form-error"
          className="text-sm text-[color:var(--storefront-danger)]"
          role="alert"
        >
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={isPending}
        className="self-start rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-2.5 text-sm font-semibold text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))] disabled:opacity-40"
      >
        {isPending ? "Submitting..." : "Submit review"}
      </button>
    </form>
  );
}
