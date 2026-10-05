// Lifted to @repo/ui for the buyer's personalisation form (#966).
//
// Re-exported rather than deleted so the ~dozen admin call sites and
// their tests keep their import path. The canonical implementation —
// including the maxDimension cap that makes the storefront's preview a
// preview rather than the print source — lives in
// packages/ui/src/crop-image.ts.
//
// Admin passes no cap: here the cropped blob IS the product image.
export {
  cropToBlob,
  fitScale,
  loadImage,
  type CropBox,
  type CropOutputOptions,
} from "@repo/ui/crop-image";
