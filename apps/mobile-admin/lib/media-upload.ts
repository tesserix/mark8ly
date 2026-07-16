/**
 * Pure, unit-testable helpers for the product-media upload flow. Kept
 * separate from `lib/product-display.ts` (display logic only) and from the
 * orchestration in `lib/admin-api/product-crud.ts` (network + react-query).
 *
 * Mirrors the web admin's battle-tested implementation
 * (`apps/admin/components/products/media/mediaUploadClient.ts:41-60`), which
 * is verified end-to-end against production.
 */

/** The subset of `ImagePickerAsset` these helpers need. */
export interface MediaAssetLike {
  fileName?: string | null;
  fileSize?: number;
}

/** The subset of `ImagePickerAsset` the upload orchestration needs. */
export interface PickedMediaAsset extends MediaAssetLike {
  uri: string;
  mimeType?: string;
}

export type MediaContentType = "image/png" | "image/jpeg" | "image/webp";

/**
 * `content_hash` is NOT a cryptographic hash — the backend treats it as a
 * path segment only (`media.BuildStorageKey`), never verified against the
 * bytes. Constraint is just `required,min=16,max=128`.
 *
 * Mirrors the web admin's `computeContentHash`: `${name}-${size}-${X}`,
 * padded to >=32 chars, non-alphanumerics replaced with "0", sliced to 64.
 * ImagePicker assets have no `lastModified` (there is no crypto available in
 * this app and none may be added), so the caller passes `Date.now()` in its
 * place as an explicit `timestamp` argument — that keeps this function pure
 * and its "stable for the same input" behaviour testable without faking the
 * clock.
 */
export function computeContentHash(asset: MediaAssetLike, timestamp: number): string {
  const base = `${asset.fileName ?? "upload"}-${asset.fileSize ?? 0}-${timestamp}`;
  let out = "";
  while (out.length < 32) {
    out += base;
  }
  return out.replace(/[^a-zA-Z0-9]/g, "0").slice(0, 64);
}

/**
 * `content_type` must be one of `image/png | image/jpeg | image/webp`
 * (`binding:"oneof"`). Defaults to jpeg, exactly like the web admin's
 * `inferContentType`.
 */
export function inferContentType(mimeType?: string | null): MediaContentType {
  if (mimeType === "image/png") return "image/png";
  if (mimeType === "image/webp") return "image/webp";
  return "image/jpeg";
}
