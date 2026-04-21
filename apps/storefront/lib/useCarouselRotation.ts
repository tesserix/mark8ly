"use client";

// Shared rotation clock for auto-cycling carousels (product cards,
// media galleries). Exposes an accessible-by-default hook:
//   - honours `prefers-reduced-motion: reduce` (no cycling)
//   - pauses while `document.visibilityState === "hidden"` (no work
//     when the tab is backgrounded)
//   - exposes pause/play + explicit navigation so carousels can meet
//     WCAG 2.2.2 (Pause, Stop, Hide)
//   - runs one `setInterval` per page regardless of how many carousels
//     are mounted — a subscriber list replaces N × timers

import { useCallback, useEffect, useRef, useState } from "react";

// ── Module-level shared tick ────────────────────────────────
// Every carousel subscribes here and gets a tick (ms) every TICK_MS.
// The interval is created on first subscribe, torn down when the last
// unsubscribes, and pauses while the tab is hidden.

type Listener = (now: number) => void;

const TICK_MS = 250;
const listeners = new Set<Listener>();
let intervalId: ReturnType<typeof setInterval> | null = null;
let visibilityHandlerInstalled = false;

function startTicker(): void {
  if (intervalId !== null || typeof document === "undefined") return;
  if (document.visibilityState === "hidden") return;
  intervalId = setInterval(() => {
    const now = Date.now();
    listeners.forEach((fn) => {
      fn(now);
    });
  }, TICK_MS);
}

function stopTicker(): void {
  if (intervalId === null) return;
  clearInterval(intervalId);
  intervalId = null;
}

function installVisibilityHandler(): void {
  if (visibilityHandlerInstalled || typeof document === "undefined") return;
  visibilityHandlerInstalled = true;
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") {
      stopTicker();
    } else if (listeners.size > 0) {
      startTicker();
    }
  });
}

function subscribe(listener: Listener): () => void {
  installVisibilityHandler();
  listeners.add(listener);
  if (listeners.size === 1) startTicker();
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) stopTicker();
  };
}

// ── Hook ────────────────────────────────────────────────────

export interface UseCarouselRotationOptions {
  /** Number of slides. Rotation disabled if <= 1. */
  count: number;
  /** Milliseconds between auto-advances. */
  intervalMs: number;
  /** External pause signal (e.g. pointer-enter). */
  paused?: boolean;
}

export interface CarouselRotation {
  index: number;
  setIndex: (next: number) => void;
  /** User-toggled pause state — independent of the external `paused` prop. */
  isPaused: boolean;
  togglePaused: () => void;
  next: () => void;
  prev: () => void;
  /** True when OS-level reduced-motion is on — carousels should not cycle. */
  prefersReducedMotion: boolean;
}

export function useCarouselRotation({
  count,
  intervalMs,
  paused,
}: UseCarouselRotationOptions): CarouselRotation {
  const [index, setIndexState] = useState(0);
  const [isPaused, setIsPaused] = useState(false);
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(false);

  const nextTickAtRef = useRef<number>(0);
  const indexRef = useRef(0);
  indexRef.current = index;
  const countRef = useRef(count);
  countRef.current = count;

  // Watch OS-level reduced-motion preference.
  useEffect(() => {
    if (typeof window === "undefined") return;
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    setPrefersReducedMotion(mq.matches);
    const onChange = (e: MediaQueryListEvent) => setPrefersReducedMotion(e.matches);
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  const setIndex = useCallback((next: number) => {
    const total = countRef.current;
    if (total <= 0) return;
    const normalised = ((next % total) + total) % total;
    setIndexState(normalised);
    // Reset the deadline so manual navigation doesn't flip mid-interval.
    nextTickAtRef.current = Date.now() + intervalMs;
  }, [intervalMs]);

  const next = useCallback(() => setIndex(indexRef.current + 1), [setIndex]);
  const prev = useCallback(() => setIndex(indexRef.current - 1), [setIndex]);
  const togglePaused = useCallback(() => setIsPaused((p) => !p), []);

  // Subscribe to the shared ticker. The listener only advances when
  // everything lines up: count > 1, not paused, not reduced-motion,
  // and the per-carousel deadline has elapsed.
  useEffect(() => {
    if (count <= 1) return;
    if (prefersReducedMotion) return;
    if (isPaused) return;
    if (paused) return;

    nextTickAtRef.current = Date.now() + intervalMs;
    return subscribe((now) => {
      if (now < nextTickAtRef.current) return;
      nextTickAtRef.current = now + intervalMs;
      setIndexState((i) => (i + 1) % countRef.current);
    });
  }, [count, intervalMs, isPaused, paused, prefersReducedMotion]);

  // Clamp index if count shrinks.
  useEffect(() => {
    if (index >= count && count > 0) setIndexState(0);
  }, [count, index]);

  return {
    index,
    setIndex,
    isPaused,
    togglePaused,
    next,
    prev,
    prefersReducedMotion,
  };
}
