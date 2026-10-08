// The buyer's three-step upload (#969), straight to marketplace-api with no
// Next proxy in between: ask for a signed PUT, send the bytes to GCS, then
// tell the server they arrived.
//
//   POST /personalisation/upload-url                  → { upload_id, url }
//   PUT  <signed GCS url>                              ← bytes never touch our servers
//   POST /personalisation/uploads/:uploadId/confirm
//
// Mirrors apps/storefront/lib/personalisation-upload.ts, with two
// differences a native client forces:
//
//   - the cart token travels in the body, because there is no cookie
//   - dimensions come from the picker rather than a decode: the asset is
//     already measured by the time we have it, so there is nothing to
//     read twice
//
// Dependencies are injected so the whole flow is testable without a
// network or a native module.

import type { PersonalisationUploadURL } from "@repo/mobile-shared/api/storefront-types";

/** An image on disk, ready to send. What pickPersonalisationImage returns. */
export interface PickedImage {
  /** file:// URI the native layer can read. */
  uri: string;
  name: string;
  /** Always one of the server's accepted types by the time it gets here. */
  type: string;
  width: number;
  height: number;
}

/** The slice of the storefront client this needs. */
export interface UploadApi {
  post<T>(path: string, body?: unknown): Promise<T>;
  delete<T>(path: string): Promise<T>;
}

export interface UploadDeps {
  putFile?: (
    url: string,
    file: PickedImage,
  ) => Promise<{ ok: boolean; status: number }>;
}

export interface UploadResult {
  uploadId: string;
  dimensions: { width: number; height: number } | null;
}

export class UploadError extends Error {
  readonly step: "sign" | "put" | "confirm";
  constructor(step: UploadError["step"], message: string) {
    super(message);
    this.step = step;
    this.name = "UploadError";
  }
}

/**
 * PUTs the file's bytes to the signed URL.
 *
 * fetch() on a file:// URI hands back a Blob backed by the native file,
 * so the image is streamed by the networking layer rather than read into
 * JS as base64 — which for a 12 MP photo is the difference between a
 * quick upload and a frozen UI.
 */
async function defaultPutFile(url: string, file: PickedImage) {
  const blob = await (await fetch(file.uri)).blob();
  const res = await fetch(url, {
    method: "PUT",
    body: blob,
    // Must match the content type the URL was signed for, or GCS rejects
    // the signature.
    headers: { "Content-Type": file.type },
  });
  return { ok: res.ok, status: res.status };
}

/**
 * Runs the full upload and returns the confirmed id.
 *
 * On a failed PUT or confirm it does NOT attempt to clean up the row.
 * That is deliberate: the 72-hour sweeper destroys anything unclaimed,
 * so a failed upload costs one orphan for three days, whereas a cleanup
 * call on an error path is another thing to fail while the buyer is
 * already looking at an error.
 */
export async function uploadPersonalisationImage(
  api: UploadApi,
  input: {
    productId: string;
    fieldId: string;
    cartToken: string;
    file: PickedImage;
  },
  deps: UploadDeps = {},
): Promise<UploadResult> {
  const put = deps.putFile ?? defaultPutFile;

  let signed: PersonalisationUploadURL;
  try {
    signed = await api.post<PersonalisationUploadURL>(
      "/personalisation/upload-url",
      {
        product_id: input.productId,
        field_id: input.fieldId,
        filename: input.file.name,
        content_type: input.file.type,
        cart_token: input.cartToken,
      },
    );
  } catch (err) {
    throw new UploadError(
      "sign",
      messageOf(err, "That photo could not be accepted."),
    );
  }
  if (!signed?.upload_id || !signed.url) {
    throw new UploadError("sign", "That photo could not be accepted.");
  }

  const dimensions =
    input.file.width > 0 && input.file.height > 0
      ? { width: input.file.width, height: input.file.height }
      : null;

  const putRes = await put(signed.url, input.file);
  if (!putRes.ok) {
    throw new UploadError(
      "put",
      "The photo could not be uploaded. Please try again.",
    );
  }

  try {
    await api.post(
      `/personalisation/uploads/${encodeURIComponent(signed.upload_id)}/confirm`,
      {
        cart_token: input.cartToken,
        ...(dimensions
          ? { width_px: dimensions.width, height_px: dimensions.height }
          : {}),
      },
    );
  } catch (err) {
    // The server's message is the useful one here — it is where the size
    // cap and the content-type check are actually enforced, and "that
    // image is too large, the limit is 15 MB" is something the buyer can
    // act on.
    throw new UploadError(
      "confirm",
      messageOf(err, "That photo could not be accepted."),
    );
  }

  return { uploadId: signed.upload_id, dimensions };
}

/** Removes an upload the buyer discarded. Best-effort. */
export async function deletePersonalisationUpload(
  api: UploadApi,
  uploadId: string,
  cartToken: string,
): Promise<void> {
  try {
    await api.delete(
      `/personalisation/uploads/${encodeURIComponent(uploadId)}?cart_token=${encodeURIComponent(cartToken)}`,
    );
  } catch {
    // The sweeper gets it within 72 hours either way. Failing the UI
    // because a cleanup call failed would punish the buyer for our
    // housekeeping.
  }
}

function messageOf(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback;
}
