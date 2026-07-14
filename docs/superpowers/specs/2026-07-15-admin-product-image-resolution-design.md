# Web-admin product image resolution fix — design

**Date:** 2026-07-15
**App:** `apps/admin` (merchant product editor) + `services/marketplace-api` (media contract)
**Status:** approved design → ready for implementation plan

## Problem

In the web admin, **Product → add image destroys the merchant's high-resolution
original**. Store owners upload professional photoshoot images; the tool forces a
small, lossy 1:1 square and uploads only that. The pristine original is lost, so
there is nothing to recrop or re-render from later.

## Root cause (confirmed)

The **backend is already correct and non-destructive**:

- `AddMedia` (`services/marketplace-api/internal/product/service_single_media.go:104`)
  sets `GcsPathOriginal = req.StorageKey` — whatever file the finalize call points
  at *becomes* the pristine original.
- The recrop endpoints (`service_media_recrop.go`) produce a **separate**
  content-addressed cropped key and leave `gcs_path_original` untouched, so future
  recrops always read the pristine source.

The bug is **entirely frontend**:

- `apps/admin/components/products/form/MediaTab.tsx` — `handleFiles` opens the crop
  dialog on every fresh add; `handleCropApply` uploads **only the cropped blob**
  (`new File([blob], …)`, line ~254). The original `File` is never uploaded, so the
  destroyed square becomes `gcs_path_original`.
- `apps/admin/components/products/media/MediaCropDialog.tsx` — invoked with **no
  `aspect`**, defaulting to `aspect = 1` (forced 1:1 square).
- `apps/admin/components/products/media/cropImage.ts` — `cropToBlob` emits JPEG at
  **0.92**, always JPEG regardless of source format.

## Approach (decided)

**Frontend-first, reuse the existing non-destructive recrop path.** The add flow
stops cropping and uploads the pristine original; cropping becomes an optional,
explicit action that routes through the already-correct recrop endpoints. One tiny
**additive, backward-compatible** backend change lets cropped renditions preserve
their source format (PNG transparency / WebP), decided as option **B**.

### Decisions locked

| Decision | Choice |
| --- | --- |
| Fix approach | Frontend-only for the core fix; reuse existing recrop endpoints |
| Add-flow default | Upload original untouched, no forced crop; cropping optional |
| Aspect options | `4:5` (default), `1:1`, `3:4`, **Free** (no fixed aspect) |
| Min-resolution guard | **Warn, allow override** at ~1000px short edge |
| Crop rendition format | **Preserve source format**, JPEG `0.95` fallback (option **B**: tiny additive backend change) |

## Changes

### Part 1 — Add flow uploads the pristine original (core fix)

`MediaTab.tsx`:

- `handleFiles` no longer opens the crop dialog. It uploads **each dropped file
  untouched** via the existing `uploadMediaFile` pipeline (`mediaUploadClient.ts` —
  already infers the correct content-type, signs, PUTs the raw `File`, finalizes).
  Result: `gcs_path_original = storage_key = pristine original`, served directly as
  display.
- Remove the `pendingFreshQueue` state and the per-file crop-dialog draining loop
  in `handleCropApply` — dropped files upload directly in a batch. The `"fresh"`
  branch of `CropTarget` / `handleCropApply` is deleted; `handleCropApply` keeps
  only the `"recrop"` branch.
- `mediaToField`'s `gcs_path_original: ""` stays (the backend is authoritative for
  this field); add a clarifying comment so it is not mistaken for the bug.

### Part 2 — Optional cropping via the existing recrop path

The per-card **"crop" action already exists** (`handleAction` → `recropMedia` →
opens `MediaCropDialog`) and is already non-destructive. Enhance the shared dialog:

`MediaCropDialog.tsx`:

- Add an **aspect selector** control (segmented buttons): `4:5` (default), `1:1`,
  `3:4`, `Free`. Drives `aspect` state; `Free` passes `undefined` to `Cropper`.
  Replaces the hardcoded `aspect = 1` default.
- After crop completion, evaluate the output short edge; if `< 1000px` show a
  **non-blocking inline warning** ("Smaller than recommended for a crisp
  storefront"). **Apply still works.**
- Thread the source content-type through so the committed blob and the signed PUT
  agree (see Part 4).

`cropImage.ts`:

- `cropToBlob` gains a `format` / `quality` parameter. Emits the **source MIME**
  (`image/png` | `image/webp` | `image/jpeg`) when known; JPEG **0.95** fallback.
- Export a small helper to read an image's natural dimensions for the guard.

### Part 3 — Min-resolution guard on add (~1000px short edge, warn)

`MediaTab.tsx` / `MediaUploader`:

- Before/at upload, read each file's natural dimensions. If short edge `< 1000px`,
  surface a **non-blocking warning** in the uploader/progress UI. Upload proceeds.
- Guard logic lives in a small pure helper (unit-tested) so both add and crop paths
  share one threshold constant.

### Part 4 — Backend: recrop signs the correct content-type (option B)

Additive and backward-compatible — `content_type` is optional and defaults to the
current `"image/jpeg"`, so existing callers are unaffected.

- `services/marketplace-api/internal/handlers/admin/validation.go` —
  `RecropMediaRequest` gains `ContentType string` with
  `binding:"omitempty,oneof=image/png image/jpeg image/webp"`.
- `services/marketplace-api/internal/handlers/admin/media.go` — `Recrop` passes
  `ContentType` into the service request (defaulting to `"image/jpeg"` when empty).
- `services/marketplace-api/internal/product/service_media_recrop.go` —
  `RecropMediaRequest` gains `ContentType`; `RecropMedia` uses it in
  `putSigner.SignedUploadURL(ctx, newKey, contentType, ttl)` (default
  `"image/jpeg"`), and derives the new key's filename extension from it.
- Frontend `recropMedia` client wrapper + `MediaTab` recrop invocation send the
  source `content_type`.

**No migration, no schema change, no storefront change.** The pristine original is
served as display until a crop is committed; the storefront keeps rendering the
stored `url`.

## Data flow (after)

**Add:** drop file → `uploadMediaFile(rawFile)` → signed PUT (native content-type)
→ `POST /media` finalize → `gcs_path_original = storage_key = original` → grid
shows original. Warn if short edge < 1000px.

**Crop (optional):** card "crop" → `POST …/recrop {content_type}` → signed GET
(original) + signed PUT (new key, matching content-type) → dialog: pick aspect,
crop in canvas at source format / 0.95 → PUT cropped blob → `PATCH /media
{storage_key: new}` → `gcs_path_original` unchanged. Warn if crop output < 1000px.

## Error handling

- Upload failures already surface per-item in the progress UI (unchanged).
- Recrop 501 (dev FakeUploader) already handled; content-type addition does not
  change that path.
- Min-res guard is advisory only — never blocks, never throws.
- Dimension read failures fail open (skip the warning, allow the upload).

## Testing

- **Unit (`apps/admin`):** `MediaTab` add now uploads the raw `File` and opens **no**
  dialog; `cropImage.cropToBlob` honors format + 0.95; min-resolution guard helper;
  aspect selector state in `MediaCropDialog`.
- **Unit (`marketplace-api`):** `RecropMedia` signs the PUT with the supplied
  content-type and defaults to `image/jpeg`; filename extension derives from it.
- **E2E (`apps/admin/tests/e2e/products-media-flow.spec.ts`):** currently asserts the
  crop dialog appears **on add** — that encodes the buggy behavior. Update it so add
  uploads directly (no dialog) and the crop dialog is exercised via the card "crop"
  action. `seed-product-images.spec.ts` kept green.

## Out of scope

- Server-side image processing / on-the-fly display variants (option C).
- Mobile-admin upload path (separate Phase 2 item).
- Storefront rendering changes.
