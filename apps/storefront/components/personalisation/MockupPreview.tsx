"use client";

// The buyer's artwork, composited into the merchant's mockup (#966).
//
// "Here is your photo on the actual shirt" rather than "here is your
// photo". The design doc puts this at roughly 90% of the perceived value
// of a 3D preview for about 2% of the cost, and the cost really is that
// low: print_area arrives as PERCENTAGES of the mockup's own dimensions,
// so the composite is two absolutely-positioned elements and no canvas,
// no server-side image work, and nothing to recompute when the merchant
// re-exports the mockup at a different size.
//
// This is a PREVIEW and is allowed to be approximate. It shows the
// buyer's cropped preview scaled into the print area; the merchant still
// prints from the pristine original and the crop rectangle. A buyer who
// is surprised at this stage is a refund avoided, which is the whole
// point of showing it.

import Image from "next/image";

export interface PrintArea {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface MockupPreviewProps {
  mockupUrl: string;
  printArea: PrintArea;
  /** Signed, short-lived URL for the buyer's own image. */
  artworkUrl?: string;
  /** The field's label, for the alt text. */
  fieldLabel: string;
}

/** Clamps a percentage into [0, 100] so a bad rectangle cannot escape. */
function pct(n: number): number {
  if (!Number.isFinite(n)) return 0;
  return Math.min(100, Math.max(0, n));
}

export function MockupPreview({
  mockupUrl,
  printArea,
  artworkUrl,
  fieldLabel,
}: MockupPreviewProps) {
  // Clamped rather than trusted. The rectangle is merchant-authored and
  // round-trips through jsonb; a negative or >100 value would position
  // the artwork outside the mockup and overlap the rest of the form.
  const left = pct(printArea.x);
  const top = pct(printArea.y);
  const width = pct(printArea.w);
  const height = pct(printArea.h);

  return (
    <figure className="mt-3 flex flex-col gap-1.5">
      <div className="relative w-full max-w-xs overflow-hidden rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/10">
        {/* The mockup sets the box's aspect ratio by flowing naturally;
            everything else is positioned against it. Unoptimized is not
            needed here — the mockup is ordinary public product media. */}
        <Image
          src={mockupUrl}
          alt=""
          width={400}
          height={400}
          className="h-auto w-full object-contain"
        />

        {artworkUrl ? (
          <span
            className="absolute overflow-hidden"
            style={{
              left: `${left}%`,
              top: `${top}%`,
              width: `${width}%`,
              height: `${height}%`,
            }}
          >
            <Image
              src={artworkUrl}
              alt={`Your artwork shown on the product for ${fieldLabel}`}
              fill
              sizes="320px"
              // object-cover, not contain: the print area is the area
              // that gets printed, and showing letterboxing here would
              // promise the buyer white margins they will not get.
              className="object-cover"
              // Signed, short-lived, private bucket — Next's optimiser
              // would cache a URL that outlives its own signature.
              unoptimized
            />
          </span>
        ) : (
          // The outline alone still tells the buyer WHERE their photo
          // goes, which is most of the value before they have uploaded.
          <span
            aria-hidden="true"
            className="absolute border-2 border-dashed border-[color:var(--storefront-accent,var(--moss-700))]/50"
            style={{
              left: `${left}%`,
              top: `${top}%`,
              width: `${width}%`,
              height: `${height}%`,
            }}
          />
        )}
      </div>
      <figcaption className="text-xs text-[color:var(--storefront-text,var(--ink-900))]/60">
        {artworkUrl
          ? "A preview of how your photo will be printed."
          : "Your photo will be printed in the marked area."}
      </figcaption>
    </figure>
  );
}
