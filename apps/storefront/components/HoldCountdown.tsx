"use client";

import { useEffect, useState } from "react";

/**
 * Shows how long the cart's stock holds have left (#232).
 *
 * # Why it is quiet by default
 *
 * A countdown is a pressure device, and this store's voice is calm and
 * unhurried. So it reads as a statement of fact in the page's own ink at low
 * emphasis, and only takes on the signal colour in the last two minutes,
 * when it is genuinely information the shopper needs rather than a nudge.
 *
 * It renders nothing at all when there is no hold, which is a normal state:
 * a hold may have failed silently, and inventing "unreserved" text would
 * manufacture anxiety about something the shopper cannot act on and that
 * checkout will enforce anyway.
 */
export function HoldCountdown({ expiresAt }: { expiresAt: string | null }) {
  const [remainingMs, setRemainingMs] = useState<number | null>(null);

  useEffect(() => {
    if (!expiresAt) {
      setRemainingMs(null);
      return;
    }
    const target = new Date(expiresAt).getTime();
    if (Number.isNaN(target)) {
      setRemainingMs(null);
      return;
    }
    const tick = () => setRemainingMs(target - Date.now());
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [expiresAt]);

  if (remainingMs === null) return null;

  // Expired: say what it means for the shopper rather than showing 0:00.
  // The items may still be there — checkout re-checks — so the honest
  // wording is that they are no longer reserved, not that they are gone.
  if (remainingMs <= 0) {
    return (
      <p
        className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-70"
        role="status"
      >
        Your items are no longer reserved. They may still be available —
        we&rsquo;ll check when you place your order.
      </p>
    );
  }

  const totalSeconds = Math.floor(remainingMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  const urgent = remainingMs <= 2 * 60 * 1000;

  return (
    <p
      className={
        urgent
          ? "text-sm text-[color:var(--signal,#C2410C)]"
          : "text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-70"
      }
      // polite, not assertive: this updates every second and must not
      // interrupt a screen-reader user mid-field.
      aria-live="polite"
      role="status"
    >
      Items reserved for{" "}
      <span className="tabular-nums">
        {minutes}:{seconds.toString().padStart(2, "0")}
      </span>
    </p>
  );
}
