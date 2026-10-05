// Canvas-based crop helper, shared by the merchant's media workbench and
// the buyer's personalisation form (#966).
//
// # The two callers want opposite things, and that is the whole point
//
// In apps/admin the cropped blob IS the product image. Full resolution is
// correct there: the merchant cropped it, that is the asset, nothing else
// exists.
//
// On the storefront it is the opposite. The buyer's upload is the asset —
// it is what the merchant prints a figurine from — and the cropped blob
// is only a PREVIEW so the shopper can see what they chose. Deriving the
// print source from a canvas would be a quiet disaster:
// `canvas.toBlob()` re-encodes, so a 48 MP phone photo comes back
// materially smaller and softer, and nobody notices until a figurine
// ships with a blurry face.
//
// So `maxDimension` caps the OUTPUT, and the CropBox the caller gets back
// is always in ORIGINAL pixels regardless. The storefront keeps the
// pristine original, stores the box as a rectangle, and treats the blob
// as disposable. Admin passes no cap and keeps the blob.

export interface CropBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface CropOutputOptions {
  mimeType?: string;
  quality?: number;
  /**
   * Longest output edge, in pixels. Omit for a full-resolution crop.
   *
   * Set this when the blob is a preview rather than the asset. It scales
   * the canvas down; it does NOT change the CropBox, which stays in
   * original pixels so the real crop can be reproduced at full
   * resolution later.
   */
  maxDimension?: number;
}

/**
 * Scale factor that fits `w`x`h` inside `maxDimension`.
 *
 * Never upscales — returns 1 for an image already under the cap, so a
 * small photo is not blown up into a blurry larger one for no reason.
 */
export function fitScale(w: number, h: number, maxDimension?: number): number {
  if (!maxDimension || maxDimension <= 0) return 1;
  const longest = Math.max(w, h);
  if (longest <= maxDimension) return 1;
  return maxDimension / longest;
}

export async function cropToBlob(
  image: HTMLImageElement,
  box: CropBox,
  rotationDeg: number,
  opts: CropOutputOptions = {},
): Promise<Blob> {
  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("2d context unavailable");

  // Math.round on all coordinates so a fractional box from
  // react-easy-crop snaps to whole pixels and the output has
  // deterministic dimensions.
  const sx = Math.round(box.x);
  const sy = Math.round(box.y);
  const sw = Math.round(box.width);
  const sh = Math.round(box.height);

  const scale = fitScale(sw, sh, opts.maxDimension);
  const dw = Math.max(1, Math.round(sw * scale));
  const dh = Math.max(1, Math.round(sh * scale));

  canvas.width = dw;
  canvas.height = dh;

  if (rotationDeg !== 0) {
    ctx.translate(dw / 2, dh / 2);
    ctx.rotate((rotationDeg * Math.PI) / 180);
    ctx.translate(-dw / 2, -dh / 2);
  }

  // Source rect in original pixels, destination rect possibly scaled —
  // drawImage does the resampling in one step.
  ctx.drawImage(image, sx, sy, sw, sh, 0, 0, dw, dh);

  const mimeType = opts.mimeType ?? "image/jpeg";
  const quality = opts.quality ?? 0.95;

  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (b) => (b ? resolve(b) : reject(new Error("canvas.toBlob returned null"))),
      mimeType,
      quality,
    );
  });
}

export function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    // Required for canvas reads of a cross-origin object. The buyer's
    // artwork comes from a signed GCS URL, so without this the canvas is
    // tainted and toBlob throws a SecurityError.
    img.crossOrigin = "anonymous";
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error(`failed to load ${src}`));
    img.src = src;
  });
}
