// Client-side branding-image upload pipeline.
// 1. POST /branding/upload-url → {upload_url, public_url, storage_key}
// 2. PUT file to GCS using upload_url
// 3. Return public_url so the caller can set it on the branding form field.

// Same-origin proxy — see
// apps/admin/app/api/admin/stores/[storeId]/branding/upload-url/route.ts
// for why we don't call marketplace-api directly from the browser.

export type BrandingImageKind = "logo" | "favicon" | "hero" | "aside" | "section";

export interface UploadProgressHandler {
  (pct: number): void;
}

interface SignedURLResponse {
  upload_url: string;
  public_url: string;
  storage_key: string;
  expires_at: string;
}

async function requestSignedUrl(
  storeId: string,
  file: File,
  kind: BrandingImageKind,
): Promise<SignedURLResponse> {
  const res = await fetch(
    `/api/admin/stores/${storeId}/branding/upload-url`,
    {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: JSON.stringify({
        filename: file.name,
        content_type: file.type || "application/octet-stream",
        kind,
      }),
    },
  );
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`upload-url ${res.status}: ${text || "failed"}`);
  }
  return (await res.json()) as SignedURLResponse;
}

function putWithProgress(
  url: string,
  file: File,
  onProgress?: UploadProgressHandler,
): Promise<{ ok: boolean; status: number }> {
  if (typeof XMLHttpRequest === "undefined") {
    return fetch(url, { method: "PUT", body: file, headers: file.type ? { "Content-Type": file.type } : undefined }).then(
      (r) => ({ ok: r.ok, status: r.status }),
    );
  }
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

export async function uploadBrandingImage(
  storeId: string,
  file: File,
  kind: BrandingImageKind,
  onProgress?: UploadProgressHandler,
): Promise<string> {
  const signed = await requestSignedUrl(storeId, file, kind);
  const put = await putWithProgress(signed.upload_url, file, onProgress);
  if (!put.ok) throw new Error(`upload failed: PUT ${put.status}`);
  return signed.public_url;
}

export const MAX_BYTES: Record<BrandingImageKind, number> = {
  logo: 1 * 1024 * 1024,
  favicon: 1 * 1024 * 1024,
  hero: 5 * 1024 * 1024,
  aside: 5 * 1024 * 1024,
  section: 5 * 1024 * 1024,
};

export const ACCEPT: Record<BrandingImageKind, string> = {
  logo: "image/png,image/jpeg,image/webp,image/svg+xml",
  favicon: "image/png,image/jpeg,image/webp,image/svg+xml,image/x-icon,image/vnd.microsoft.icon",
  hero: "image/png,image/jpeg,image/webp",
  aside: "image/png,image/jpeg,image/webp",
  section: "image/png,image/jpeg,image/webp",
};
