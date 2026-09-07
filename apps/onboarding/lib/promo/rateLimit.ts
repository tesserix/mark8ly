/**
 * Per-visitor cap on promo-code checks (#620).
 *
 * # Why this exists HERE and not only in the API
 *
 * The field calls a server action, so by the time the request reaches
 * platform-api the client IP is this app's pod — every visitor shares one
 * bucket there. platform-api's limiter is therefore a coarse service-wide cap,
 * and this is the layer that can actually see a visitor.
 *
 * Two layers, each doing what only it can: this one bounds a single visitor,
 * that one bounds the blast radius if this one is bypassed.
 *
 * In-memory and per-pod, like every other limiter in this estate. A visitor
 * spread across pods gets a multiple of the cap; that is acceptable for a
 * guessing surface whose codes are already capped per email address on
 * redemption.
 */

/** Window and cap. Loose enough to retype a code several times, tight enough
 *  that enumeration is pointless. */
export const PROMO_WINDOW_MS = 10 * 60 * 1000;
export const PROMO_MAX_ATTEMPTS = 12;

const attempts = new Map<string, number[]>();

/**
 * Records an attempt for `key` and reports whether it is within the cap.
 *
 * `now` is a parameter so the window can be tested without sleeping.
 */
export function allowPromoCheck(key: string, now: number = Date.now()): boolean {
  const cutoff = now - PROMO_WINDOW_MS;
  const fresh = (attempts.get(key) ?? []).filter((t) => t > cutoff);

  if (fresh.length >= PROMO_MAX_ATTEMPTS) {
    // Write the pruned list back even on refusal, so a key that stops being
    // used stops holding its timestamps.
    attempts.set(key, fresh);
    return false;
  }
  fresh.push(now);
  attempts.set(key, fresh);
  return true;
}

/** Clears all buckets. Tests only — nothing in the app calls it. */
export function resetPromoRateLimit(): void {
  attempts.clear();
}
