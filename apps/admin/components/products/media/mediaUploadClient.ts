// apps/admin/components/products/media/mediaUploadClient.ts
//
// Client-side media upload pipeline: request a signed URL from the
// marketplace-api, PUT the raw file to GCS, then POST /media to
// finalize the row. On a 403 from the PUT (usually a stale/expired
// signed URL) the upload is retried exactly once with a freshly
// issued signed URL. `putFn` is injectable so the unit test can avoid
// the real XHR/fetch upload.

import type { AdminMediaResponse } from "@/lib/api/marketplace-api";

export interface SignedUrlResponse {
  upload_url: string;
  storage_key: string;
  expires_at: string;
}

export type PutFn = (
  url: string,
  file: File,
  onProgress?: (pct: number) => void,
) => Promise<{ ok: boolean; status: number }>;

export interface UploadMediaFileArgs {
  storeId: string;
  productId: string;
  file: File;
  position: number;
  variantId?: string | null;
  alt?: string;
  onProgress?: (pct: number) => void;
  /** Injectable for tests. Defaults to an XHR-based PUT in the browser. */
  putFn?: PutFn;
}

const MARKETPLACE_API_URL =
  process.env.NEXT_PUBLIC_MARKETPLACE_API_URL ??
  process.env.MARKETPLACE_API_URL ??
  "";

function computeContentHash(file: File): string {
  // Deterministic best-effort identifier for the upload-url request.
  // The backend treats `content_hash` as an idempotency / addressing
  // hint, not a cryptographic commitment. Real SHA-256 happens server
  // side when the file lands in GCS. We still want a stable non-empty
  // 16+ char value here.
  const base = `${file.name}-${file.size}-${file.lastModified}`;
  // Pad to 16 chars minimum by concatenating until long enough.
  let out = "";
  while (out.length < 32) {
    out += base;
  }
  return out.replace(/[^a-zA-Z0-9]/g, "0").slice(0, 64);
}

function inferContentType(file: File): "image/png" | "image/jpeg" | "image/webp" {
  if (file.type === "image/png") return "image/png";
  if (file.type === "image/webp") return "image/webp";
  return "image/jpeg";
}

async function requestSignedUrl(
  storeId: string,
  productId: string,
  file: File,
): Promise<SignedUrlResponse> {
  const body = {
    content_hash: computeContentHash(file),
    filename: file.name,
    content_type: inferContentType(file),
  };
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/${productId}/media/upload-url`,
    {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    throw new Error(`signed url request failed: ${res.status}`);
  }
  const raw = (await res.json()) as {
    upload_url?: string;
    url?: string;
    storage_key: string;
    expires_at: string;
  };
  return {
    upload_url: raw.upload_url ?? raw.url ?? "",
    storage_key: raw.storage_key,
    expires_at: raw.expires_at,
  };
}

async function finalize(
  storeId: string,
  productId: string,
  storageKey: string,
  args: UploadMediaFileArgs,
): Promise<AdminMediaResponse> {
  const body: Record<string, unknown> = {
    storage_key: storageKey,
    url: storageKey,
    position: args.position,
    media_type: "image",
  };
  if (args.alt !== undefined) body.alt = args.alt;
  if (args.variantId) body.variant_id = args.variantId;

  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/${productId}/media`,
    {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    throw new Error(`finalize media failed: ${res.status}`);
  }
  return (await res.json()) as AdminMediaResponse;
}

function defaultPutFn(): PutFn {
  return async (url, file, onProgress) => {
    // Browser path: XHR gives us upload progress, fetch does not.
    if (typeof XMLHttpRequest !== "undefined") {
      return new Promise((resolve) => {
        const xhr = new XMLHttpRequest();
        xhr.open("PUT", url);
        if (file.type) xhr.setRequestHeader("Content-Type", file.type);
        xhr.upload.onprogress = (e) => {
          if (onProgress && e.lengthComputable) {
            onProgress(Math.round((e.loaded / e.total) * 100));
          }
        };
        xhr.onload = () => resolve({ ok: xhr.status >= 200 && xhr.status < 300, status: xhr.status });
        xhr.onerror = () => resolve({ ok: false, status: xhr.status || 0 });
        xhr.send(file);
      });
    }
    const res = await fetch(url, { method: "PUT", body: file });
    return { ok: res.ok, status: res.status };
  };
}

export async function uploadMediaFile(
  args: UploadMediaFileArgs,
): Promise<AdminMediaResponse> {
  const put = args.putFn ?? defaultPutFn();

  const first = await requestSignedUrl(args.storeId, args.productId, args.file);
  const firstPut = await put(first.upload_url, args.file, args.onProgress);

  let storageKey = first.storage_key;

  if (!firstPut.ok) {
    if (firstPut.status !== 403) {
      throw new Error(`upload failed: PUT returned ${firstPut.status}`);
    }
    // Retry once with a fresh signed URL.
    const fresh = await requestSignedUrl(args.storeId, args.productId, args.file);
    const secondPut = await put(fresh.upload_url, args.file, args.onProgress);
    if (!secondPut.ok) {
      throw new Error(`upload failed after retry: PUT returned ${secondPut.status}`);
    }
    storageKey = fresh.storage_key;
  }

  return finalize(args.storeId, args.productId, storageKey, args);
}
