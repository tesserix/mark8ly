import type { GiftCard } from "@repo/mobile-shared/api/types";

/**
 * Which gift-card status changes are legal, as pure functions.
 *
 * Deliberately a module of its OWN rather than living beside the mutation in
 * `lib/admin-api/gift-card-actions.ts`. That module imports `useApiClient`
 * and through it the auth provider and `expo/virtual/env`, so every suite
 * that merely wants to know which gestures a row should arm would have to
 * boot — or stub — the entire auth chain to ask. The rule is the thing most
 * worth testing here and it has no dependencies at all, so it is kept
 * reachable without any.
 */

/**
 * The only two statuses `POST /gift-cards/:id/{enable,disable}` can produce.
 *
 * Named as the TARGET rather than as a verb because that is what the backend
 * implements: the repository's `SetStatus` is an atomic
 * `UPDATE … WHERE status = ?` and asking for the state a card is already in
 * succeeds with an idempotent 200. It is not a transition with an event
 * attached.
 */
export type GiftCardStatusTarget = "active" | "disabled";

/**
 * Whether a card is past its expiry.
 *
 * `now` is injectable so the rule can be tested against a fixed instant
 * rather than against whenever the suite happens to run.
 *
 * Fails OPEN on an unparseable timestamp: the server is authoritative and
 * will answer 410 if the card really has expired. Failing closed would
 * strand a good card behind a gesture the merchant cannot reach and cannot
 * diagnose.
 */
export function giftCardHasExpired(
  card: Pick<GiftCard, "expires_at">,
  now: number = Date.now(),
): boolean {
  if (!card.expires_at) return false;
  const at = Date.parse(card.expires_at);
  return Number.isFinite(at) && at <= now;
}

/**
 * Whether this card may legally be moved to `to` — the single source of
 * truth behind BOTH the Gift cards screen's swipe gate and its long-press
 * menu's `disabled` flags, so the two can never disagree about one row.
 *
 * Three rules, each mirroring a specific server refusal:
 *
 *  1. **Only `active` and `disabled` cards toggle.** `pending`, `depleted`
 *     and `refunded` are refused with 409 `invalid_transition`
 *     (giftcard/repository.go `classifyStatusTransition`). A `pending` card
 *     in particular has not been paid for yet.
 *  2. **An expired card toggles in NEITHER direction** — 410
 *     `gift_card_expired`. 🔴 There is no `expired` STATUS in the backend
 *     enum (giftcard/models.go: pending|active|disabled|depleted|refunded);
 *     expiry is a TIMESTAMP, so an expired card still reads
 *     `status: "active"`. A gate that switched on status alone would arm
 *     both gestures on it and collect a 410 every time.
 *  3. **Already at the target is refused HERE but not by the server**, which
 *     answers an idempotent 200. This one is for the merchant, not the wire:
 *     an "Enable" offered on a card that is already active is an action that
 *     visibly does nothing, and in a list with no undo that reads as a
 *     failure.
 *
 * A card failing both directions gets no `SwipeRow` at all on the screen —
 * an armed gesture that can only 4xx is worse than no gesture.
 */
export function canSetGiftCardStatus(
  card: Pick<GiftCard, "status" | "expires_at">,
  to: GiftCardStatusTarget,
  now: number = Date.now(),
): boolean {
  if (card.status !== "active" && card.status !== "disabled") return false;
  if (card.status === to) return false;
  return !giftCardHasExpired(card, now);
}
