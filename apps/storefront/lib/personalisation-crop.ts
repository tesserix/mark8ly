// apps/storefront/lib/personalisation-crop.ts
//
// Letting the buyer crop what they uploaded (#966).
//
// # The derived blob is a PREVIEW, never the print source
//
// This is the inversion that makes the storefront's crop different from
// the merchant's. In apps/admin the cropped blob IS the product image.
// Here the buyer's upload is the asset — the merchant prints a figurine
// from it — and the blob is only so the shopper can see what they chose.
//
// `canvas.toBlob()` re-encodes, so deriving the print source from it
// would quietly turn a 48 MP phone photo into something soft, and the
// first person to notice would be the customer holding a figurine with a
// blurry face. So:
//
//   storage_key_original  untouched, pristine, what gets printed
//   crop                  a rectangle in ORIGINAL pixels
//   storage_key           this ~1024px preview, disposable
//
// A re-crop therefore re-derives from the original rather than from the
// last preview, and the buyer can crop repeatedly without compounding
// loss.

/** Longest edge of the generated preview. */
export const PREVIEW_MAX_DIMENSION = 1024;

export interface CropRect {
  x: number;
  y: number;
  w: number;
  h: number;
  rotation: number;
}

export interface CropDeps {
  fetchJson?: typeof fetch;
  putBlob?: (url: string, blob: Blob) => Promise<{ ok: boolean }>;
}

export class CropError extends Error {
  readonly step: "prepare" | "put";
  constructor(step: CropError["step"], message: string) {
    super(message);
    this.step = step;
    this.name = "CropError";
  }
}

async function defaultPut(url: string, blob: Blob) {
  const res = await fetch(url, {
    method: "PUT",
    body: blob,
    // Must match what the URL was signed for or GCS rejects it.
    headers: { "Content-Type": "image/jpeg" },
  });
  return { ok: res.ok };
}

export interface PreparedCrop {
  /** Signed GET for the PRISTINE original, to crop against. */
  sourceUrl: string;
  uploadUrl: string;
}

/**
 * Records the rectangle and gets back somewhere to put the preview.
 *
 * The rectangle is persisted BEFORE the preview exists, which is the
 * right order: the rectangle is the durable record of what the buyer
 * chose, and a preview that fails to upload leaves a correct crop with a
 * stale thumbnail rather than a lost choice.
 */
export async function prepareCrop(
  storeSlug: string,
  uploadId: string,
  rect: CropRect,
  deps: CropDeps = {},
): Promise<PreparedCrop> {
  const doFetch = deps.fetchJson ?? fetch;
  const res = await doFetch(
    `/api/personalisation/uploads/${encodeURIComponent(uploadId)}/crop` +
      `?store=${encodeURIComponent(storeSlug)}`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ crop: rect }),
    },
  );
  if (!res.ok) {
    throw new CropError("prepare", "That crop could not be saved. Please try again.");
  }
  const body = (await res.json()) as { source_url?: string; upload_url?: string };
  if (!body.source_url || !body.upload_url) {
    throw new CropError("prepare", "That crop could not be saved. Please try again.");
  }
  return { sourceUrl: body.source_url, uploadUrl: body.upload_url };
}

/**
 * Sends the generated preview to the signed PUT.
 *
 * A failure here is NOT fatal to the crop: the rectangle is already
 * stored, so the merchant still prints the right thing. The buyer just
 * keeps seeing the previous thumbnail, which is why the caller treats
 * this as best-effort rather than rolling anything back.
 */
export async function uploadCropPreview(
  uploadUrl: string,
  blob: Blob,
  deps: CropDeps = {},
): Promise<void> {
  const put = deps.putBlob ?? defaultPut;
  const res = await put(uploadUrl, blob);
  if (!res.ok) {
    throw new CropError("put", "The preview could not be saved, but your crop was.");
  }
}
