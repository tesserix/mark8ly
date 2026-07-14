// Advisory resolution guard + MIME helpers shared by the add and
// recrop media paths. The guard NEVER blocks — it only warns.

export const MIN_SHORT_EDGE_PX = 1000;

export const MIN_RESOLUTION_WARNING =
  "Smaller than recommended for a crisp storefront (under 1000px). You can still use it.";

/** True when the image's shorter edge is under the recommended floor. */
export function belowMinShortEdge(width: number, height: number): boolean {
  if (!Number.isFinite(width) || !Number.isFinite(height)) return false;
  return Math.min(width, height) < MIN_SHORT_EDGE_PX;
}

/** Best-effort MIME inference from a URL or storage key extension. */
export function inferMimeFromUrl(url: string): "image/png" | "image/webp" | "image/jpeg" {
  const clean = url.split("?")[0]?.toLowerCase() ?? "";
  if (clean.endsWith(".png")) return "image/png";
  if (clean.endsWith(".webp")) return "image/webp";
  return "image/jpeg";
}

/** Reads a File's natural pixel dimensions via an off-screen Image. */
export function readFileDimensions(file: File): Promise<{ width: number; height: number }> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      const dims = { width: img.naturalWidth, height: img.naturalHeight };
      URL.revokeObjectURL(url);
      resolve(dims);
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error(`failed to read dimensions for ${file.name}`));
    };
    img.src = url;
  });
}
