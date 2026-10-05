// apps/storefront/lib/personalisation-summary.ts
//
// What the buyer reads back for each kind of answer, and whether a cart
// line is blocked (#966).
//
// In lib/ rather than beside the component because the storefront's
// vitest runs in `node` with no DOM: logic that decides what a buyer is
// told has to live somewhere it can be tested, which is the same reason
// lib/personalisation.ts exists next door.

import type { CartItemPersonalisation } from "@/lib/cart";
import { isExpired, type PreviewLookup } from "@/lib/personalisation-previews";

export const EXPIRED_MESSAGE =
  "Your photo has expired — please upload it again.";

/** The buyer-visible value of one answer, label aside. */
export function answerText(entry: CartItemPersonalisation): string | null {
  if (entry.text && entry.text.trim() !== "") return entry.text;
  if (entry.optionLabel) return entry.optionLabel;
  // A bare optionId is meaningless to a buyer. Carts stored before #966
  // have no optionLabel, so say nothing rather than print a uuid.
  if (entry.optionId) return null;
  if (entry.checked !== undefined) return entry.checked ? "Yes" : "No";
  return null;
}

/**
 * True when any answer on this line points at an upload the server says
 * it cannot read.
 *
 * False while previews are still loading, which matters: claiming expiry
 * from an empty lookup would flash "your photo has expired" on every
 * cart render before the first response lands.
 */
export function hasExpiredUpload(
  entries: readonly CartItemPersonalisation[] | undefined,
  previews: PreviewLookup,
): boolean {
  if (!entries) return false;
  return entries.some((e) => e.uploadId && isExpired(previews, e.uploadId));
}
