// apps/storefront/lib/personalisation-upload.ts
//
// The buyer's three-step upload (#965): ask for a signed PUT, send the
// bytes straight to GCS, then tell the server it arrived.
//
// The PUT goes to Google, not through this origin. That is the point of a
// signed URL — the image never touches the storefront or marketplace-api,
// so neither pays for the bandwidth and neither needs a multipart body
// parser on an anonymous route.
//
// Dependencies are injected so the whole flow is testable without a
// network, which matters here because the storefront has no e2e suite in
// CI (#979).

export interface UploadDeps {
  fetchJson?: typeof fetch;
  putBlob?: (url: string, file: File) => Promise<{ ok: boolean; status: number }>;
  measure?: (file: File) => Promise<{ width: number; height: number } | null>;
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

/** Reads an image's intrinsic size in the browser. */
async function measureImage(file: File): Promise<{ width: number; height: number } | null> {
  if (typeof window === "undefined" || typeof createImageBitmap !== "function") return null;
  try {
    const bitmap = await createImageBitmap(file);
    const out = { width: bitmap.width, height: bitmap.height };
    bitmap.close?.();
    return out;
  } catch {
    // An unreadable image is not evidence of a bad one. The upload
    // proceeds and the resolution warning simply stays quiet.
    return null;
  }
}

async function defaultPut(url: string, file: File) {
  const res = await fetch(url, {
    method: "PUT",
    body: file,
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
  storeSlug: string,
  input: { productId: string; fieldId: string; file: File },
  deps: UploadDeps = {},
): Promise<UploadResult> {
  const doFetch = deps.fetchJson ?? fetch;
  const put = deps.putBlob ?? defaultPut;
  const measure = deps.measure ?? measureImage;

  const signRes = await doFetch(
    `/api/personalisation/upload-url?store=${encodeURIComponent(storeSlug)}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        product_id: input.productId,
        field_id: input.fieldId,
        filename: input.file.name,
        content_type: input.file.type,
      }),
    },
  );
  if (!signRes.ok) {
    throw new UploadError("sign", await readMessage(signRes, "That image could not be accepted."));
  }
  const signed = (await signRes.json()) as { upload_id?: string; url?: string };
  if (!signed.upload_id || !signed.url) {
    throw new UploadError("sign", "That image could not be accepted.");
  }

  // Measured BEFORE the PUT: the file is already in memory, and a failed
  // measurement must not cost a second read of a large image.
  const dimensions = await measure(input.file);

  const putRes = await put(signed.url, input.file);
  if (!putRes.ok) {
    throw new UploadError("put", "The image could not be uploaded. Please try again.");
  }

  const confirmRes = await doFetch(
    `/api/personalisation/uploads/${encodeURIComponent(signed.upload_id)}/confirm` +
      `?store=${encodeURIComponent(storeSlug)}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(
        dimensions ? { width_px: dimensions.width, height_px: dimensions.height } : {},
      ),
    },
  );
  if (!confirmRes.ok) {
    // The server's message is the useful one here — it is where the size
    // cap and the content-type check are actually enforced, and "that
    // image is too large, the limit is 15 MB" is something the buyer can
    // act on.
    throw new UploadError("confirm", await readMessage(confirmRes, "That image could not be accepted."));
  }

  return { uploadId: signed.upload_id, dimensions };
}

/** Removes an upload the buyer discarded. Best-effort. */
export async function deletePersonalisationUpload(
  storeSlug: string,
  uploadId: string,
  deps: UploadDeps = {},
): Promise<void> {
  const doFetch = deps.fetchJson ?? fetch;
  try {
    await doFetch(
      `/api/personalisation/uploads/${encodeURIComponent(uploadId)}?store=${encodeURIComponent(storeSlug)}`,
      { method: "DELETE" },
    );
  } catch {
    // The sweeper gets it within 72 hours either way. Failing the UI
    // because a cleanup call failed would punish the buyer for our
    // housekeeping.
  }
}

async function readMessage(res: Response, fallback: string): Promise<string> {
  try {
    const body = (await res.json()) as { message?: string };
    return body?.message || fallback;
  } catch {
    return fallback;
  }
}
