"use client";

import {
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type PointerEvent,
  type TransitionEvent,
} from "react";
import type { PublicTestimonial } from "@/lib/testimonials";

const DWELL_MS = 5000;

function useMediaQuery(query: string) {
  return useSyncExternalStore(
    (onChange) => {
      const media = window.matchMedia(query);
      media.addEventListener("change", onChange);
      return () => media.removeEventListener("change", onChange);
    },
    () => window.matchMedia(query).matches,
    () => false,
  );
}

function useReviews() {
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
  return reviews;
}

export function Testimonials() {
  const reviews = useReviews();
  const wide = useMediaQuery("(min-width: 64rem)");
  const medium = useMediaQuery("(min-width: 40rem)");
  const reducedMotion = useMediaQuery("(prefers-reduced-motion: reduce)");
  const [index, setIndex] = useState(0);
  const [animate, setAnimate] = useState(true);
  const [paused, setPaused] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const queuedStep = useRef(0);
  const swipeStart = useRef<number | null>(null);

  const count = reviews.length;
  const moving = count > (wide ? 3 : medium ? 2 : 1);
  const autoplay = moving && !reducedMotion;
  const playing = autoplay && !paused && !hovered && !focused;
  const position = moving ? index : 0;

  useEffect(() => {
    if (!playing) return;
    const timer = setTimeout(() => {
      setAnimate(true);
      setIndex((i) => Math.min(i + 1, count));
    }, DWELL_MS);
    return () => clearTimeout(timer);
  }, [playing, index, count]);

  // The jump back to the matching real slide must paint without a transition first.
  useEffect(() => {
    if (animate) return;
    let inner = 0;
    const outer = requestAnimationFrame(() => {
      inner = requestAnimationFrame(() => {
        setAnimate(true);
        const step = queuedStep.current;
        queuedStep.current = 0;
        if (step) setIndex((i) => i + step);
      });
    });
    return () => {
      cancelAnimationFrame(outer);
      cancelAnimationFrame(inner);
    };
  }, [animate]);

  if (count === 0) return null;

  function step(direction: 1 | -1) {
    if (!animate || index >= count) return;
    if (direction === -1 && index === 0) {
      queuedStep.current = -1;
      setAnimate(false);
      setIndex(count);
      return;
    }
    setAnimate(true);
    setIndex(index + direction);
  }

  function settle(event: TransitionEvent<HTMLDivElement>) {
    if (event.target !== event.currentTarget || index < count) return;
    setAnimate(false);
    setIndex(index - count);
  }

  function endSwipe(event: PointerEvent<HTMLDivElement>) {
    const start = swipeStart.current;
    swipeStart.current = null;
    if (start === null) return;
    const distance = event.clientX - start;
    if (Math.abs(distance) > 40) step(distance < 0 ? 1 : -1);
  }

  const slides = moving ? [...reviews, ...reviews] : reviews;

  return (
    <section
      aria-labelledby="testimonials-title"
      className="border-t border-border-subtle py-16 sm:py-24"
    >
      <div
        className="mx-auto max-w-6xl px-6"
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        onFocus={() => setFocused(true)}
        onBlur={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget)) setFocused(false);
        }}
      >
        <div className="flex flex-col gap-6 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="eyebrow mb-5">From our sellers</p>
            <h2
              id="testimonials-title"
              className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground"
            >
              In their own words.
            </h2>
          </div>
          {moving && (
            <div className="flex shrink-0 gap-2">
              {autoplay && (
                <ControlButton
                  label={paused ? "Play testimonials" : "Pause testimonials"}
                  onClick={() => setPaused(!paused)}
                  path={paused ? "M8 5.5v13l10-6.5z" : "M9 6v12M15 6v12"}
                />
              )}
              <ControlButton
                label="Previous testimonial"
                onClick={() => step(-1)}
                path="M15 18l-6-6 6-6"
              />
              <ControlButton
                label="Next testimonial"
                onClick={() => step(1)}
                path="M9 18l6-6-6-6"
              />
            </div>
          )}
        </div>
        <div
          className="@container mt-12 touch-pan-y overflow-hidden [--slide:85cqw] sm:[--slide:calc((100cqw_-_1.5rem)/2)] lg:[--slide:calc((100cqw_-_3rem)/3)]"
          onPointerDown={(e) => {
            if (e.pointerType !== "mouse") swipeStart.current = e.clientX;
          }}
          onPointerUp={endSwipe}
          onPointerCancel={() => (swipeStart.current = null)}
        >
          <div
            aria-live={playing ? "off" : "polite"}
            className={`flex gap-6 ${animate ? "transition-transform duration-700 ease-in-out" : "transition-none"}`}
            style={{
              transform: `translateX(calc(${position} * (var(--slide) + 1.5rem) * -1))`,
            }}
            onTransitionEnd={settle}
          >
            {slides.map((review, i) => {
              const copy = i >= count;
              return (
                <figure
                  key={copy ? `${review.store_id}-repeat` : review.store_id}
                  role={copy ? undefined : "group"}
                  aria-roledescription={copy ? undefined : "testimonial"}
                  aria-label={copy ? undefined : `${i + 1} of ${count}`}
                  aria-hidden={copy || undefined}
                  inert={copy}
                  className="flex w-(--slide) shrink-0 flex-col rounded-lg border border-border-subtle bg-background p-6 sm:p-8"
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
                      {review.position}
                      {review.display_name ? ` · ${review.display_name}` : ""}
                    </p>
                    <p className="mt-1 text-sm text-foreground-secondary">
                      Online store of {review.store_name}
                    </p>
                  </figcaption>
                </figure>
              );
            })}
          </div>
        </div>
      </div>
    </section>
  );
}

type ControlButtonProps = { label: string; onClick: () => void; path: string };

function ControlButton({ label, onClick, path }: ControlButtonProps) {
  return (
    <button
      type="button"
      aria-label={label}
      onClick={onClick}
      className="inline-flex h-11 w-11 items-center justify-center rounded-full border border-border-subtle text-foreground hover:bg-paper-300"
    >
      <svg
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        <path d={path} />
      </svg>
    </button>
  );
}
