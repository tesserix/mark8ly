"use client";

// What the buyer chose, shown back to them (#966).
//
// Rendered on the cart line, the checkout review and the buyer's own
// order detail. Without it, two personalised lines of one variant are
// visually identical — a mug printed "Asha" and the same mug printed
// "Ravi" render as the same row, and the buyer cannot tell whether they
// ordered the right thing or whether the quantity stepper they just
// pressed belonged to the other one.
//
// # The expired case is the one that matters
//
// Uploads are swept at 72 hours. Cart lines live in localStorage
// indefinitely. So a shopper can return to a cart that looks perfect and
// is not buyable. The message goes HERE, on the line, next to the thing
// that is wrong — not on the pay button, which would tell them something
// is broken without saying what or where.

import Image from "next/image";

import type { CartItemPersonalisation } from "@/lib/cart";
import { isExpired, type PreviewLookup } from "@/lib/personalisation-previews";
import { answerText, EXPIRED_MESSAGE } from "@/lib/personalisation-summary";

export interface PersonalisationSummaryProps {
  entries: readonly CartItemPersonalisation[] | undefined;
  previews: PreviewLookup;
  /** Rendered smaller on a dense checkout review than on the cart. */
  size?: "compact" | "default";
  /** Called when the buyer wants to replace an expired image. */
  onReupload?: (entry: CartItemPersonalisation) => void;
}

export function PersonalisationSummary({
  entries,
  previews,
  size = "default",
  onReupload,
}: PersonalisationSummaryProps) {
  if (!entries || entries.length === 0) return null;

  const thumb = size === "compact" ? 32 : 44;

  return (
    <ul
      role="list"
      className="mt-2 flex flex-col gap-1.5 text-xs text-[color:var(--storefront-text,var(--ink-900))]/70"
    >
      {entries.map((entry, i) => {
        const expired = entry.uploadId ? isExpired(previews, entry.uploadId) : false;
        const preview = entry.uploadId ? previews.previews[entry.uploadId] : undefined;
        const value = answerText(entry);

        return (
          <li key={`${entry.fieldId}-${i}`} className="flex items-start gap-2">
            {entry.uploadId ? (
              expired ? (
                // No broken <Image>: a 403 placeholder beside "expired"
                // reads as two faults rather than one.
                <span
                  aria-hidden="true"
                  style={{ width: thumb, height: thumb }}
                  className="shrink-0 rounded border border-dashed border-[color:var(--storefront-text,var(--ink-900))]/25"
                />
              ) : preview ? (
                <span
                  className="relative shrink-0 overflow-hidden rounded border border-[color:var(--storefront-text,var(--ink-900))]/10"
                  style={{ width: thumb, height: thumb }}
                >
                  <Image
                    src={preview.url}
                    alt={
                      entry.fieldLabel
                        ? `Your image for ${entry.fieldLabel}`
                        : "Your uploaded image"
                    }
                    fill
                    sizes={`${thumb}px`}
                    className="object-cover"
                    // The bucket is private and the URL is signed and
                    // short-lived; Next's optimiser would cache a URL
                    // that outlives its own signature.
                    unoptimized
                  />
                </span>
              ) : (
                // Still loading. A neutral block, not a spinner — a row
                // of spinners on a cart is noise.
                <span
                  aria-hidden="true"
                  style={{ width: thumb, height: thumb }}
                  className="shrink-0 animate-pulse rounded bg-[color:var(--storefront-text,var(--ink-900))]/10"
                />
              )
            ) : null}

            <span className="flex min-w-0 flex-col gap-0.5">
              {entry.fieldLabel ? (
                <span className="font-medium">{entry.fieldLabel}</span>
              ) : null}

              {expired ? (
                <>
                  <span
                    role="status"
                    className="text-[color:var(--storefront-accent,var(--moss-700))]"
                  >
                    {EXPIRED_MESSAGE}
                  </span>
                  {onReupload ? (
                    <button
                      type="button"
                      onClick={() => onReupload(entry)}
                      className="self-start underline underline-offset-2 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
                    >
                      Upload a new photo
                    </button>
                  ) : null}
                </>
              ) : value ? (
                // pre-wrap: the buyer's line breaks are theirs, and an
                // engraving that loses them is the wrong engraving.
                <span className="whitespace-pre-wrap break-words">{value}</span>
              ) : entry.uploadId ? (
                <span className="opacity-70">Your photo</span>
              ) : null}
            </span>
          </li>
        );
      })}
    </ul>
  );
}

// Re-exported from lib so cart/checkout/order call sites import the
// component and its predicate from one place.
export { hasExpiredUpload, EXPIRED_MESSAGE } from "@/lib/personalisation-summary";
