// Canvas-based crop helper. Produces a JPEG blob from an image and a
// pixel-space crop box. Uses Math.round on all coordinates so a
// fractional crop box from react-easy-crop snaps to whole pixels and
// the output image has deterministic dimensions.

export interface CropBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface CropOutputOptions {
  mimeType?: string;
  quality?: number;
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

  const sx = Math.round(box.x);
  const sy = Math.round(box.y);
  const sw = Math.round(box.width);
  const sh = Math.round(box.height);

  canvas.width = sw;
  canvas.height = sh;

  if (rotationDeg !== 0) {
    ctx.translate(sw / 2, sh / 2);
    ctx.rotate((rotationDeg * Math.PI) / 180);
    ctx.translate(-sw / 2, -sh / 2);
  }

  ctx.drawImage(image, sx, sy, sw, sh, 0, 0, sw, sh);

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
    img.crossOrigin = "anonymous";
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error(`failed to load ${src}`));
    img.src = src;
  });
}
