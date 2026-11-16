"use client";

import { useEffect, useState } from "react";
import type { PublicTestimonial } from "@/lib/testimonials";

export function Testimonials() {
  const [reviews, setReviews] = useState<PublicTestimonial[]>([]);
  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      try {
        const res = await fetch("/api/testimonials", {
          cache: "no-store",
          signal: AbortSignal.any([
            controller.signal,
            AbortSignal.timeout(6000),
          ]),
        });
        if (!res.ok) return;
        const body = (await res.json()) as { data: PublicTestimonial[] };
        if (!controller.signal.aborted) setReviews(body.data);
      } catch {
        /* The homepage remains usable when reviews are unavailable. */
      }
    }
    void load();
    return () => controller.abort();
  }, []);
  if (reviews.length === 0) return null;
  return (
    <section
      aria-labelledby="testimonials-title"
      className="border-t border-border-subtle py-16 sm:py-24"
    >
      <div className="mx-auto max-w-6xl px-6">
        <p className="eyebrow mb-5">From our sellers</p>
        <h2
          id="testimonials-title"
          className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground"
        >
          In their own words.
        </h2>
        <div className="mt-12 grid gap-6 md:grid-cols-3">
          {reviews.map((review) => (
            <figure
              key={review.store_id}
              className="flex flex-col rounded-lg border border-border-subtle bg-background p-6 sm:p-8"
            >
              <span
                role="img"
                aria-label={`${review.rating} out of 5 stars`}
                className="text-lg tracking-widest text-moss-700"
              >
                <span aria-hidden="true">
                  {"★".repeat(review.rating)}
                  {"☆".repeat(5 - review.rating)}
                </span>
              </span>
              <blockquote className="mb-8 mt-5 flex-1 text-lg leading-relaxed text-foreground">
                “{review.quote}”
              </blockquote>
              <figcaption>
                <p className="font-medium text-foreground">
                  {review.author_name}
                </p>
                <p className="mt-1 text-sm text-foreground-secondary">
                  Online store of {review.store_name}
                </p>
              </figcaption>
            </figure>
          ))}
        </div>
      </div>
    </section>
  );
}
