"use client";

// Lifted to @repo/ui as ImageCropDialog (#966).
//
// This wrapper keeps admin's behaviour and its import path exactly as
// they were: the "Editorial workbench" eyebrow, the four aspect options,
// admin's fixed 1000px resolution floor, and NO output cap — because in
// admin the cropped blob is the product image, not a preview.
//
// The storefront passes a ~1024px cap and the merchant's per-field
// min_px instead. See packages/ui/src/crop-image.ts for why those two
// callers must not share a default.

import * as React from "react";

import { ImageCropDialog } from "@repo/ui/image-crop-dialog";
import type { CropBox } from "@repo/ui/crop-image";

import { belowMinShortEdge, MIN_RESOLUTION_WARNING } from "./mediaResolution";

export interface MediaCropDialogProps {
  sourceUrl: string;
  aspect?: number;
  sourceMimeType?: string;
  onApply: (blob: Blob, box: CropBox, rotation: number) => void | Promise<void>;
  onCancel: () => void;
}

export function MediaCropDialog(props: MediaCropDialogProps): React.ReactElement {
  return (
    <ImageCropDialog
      {...props}
      eyebrow="Editorial workbench"
      lowResolutionWarning={(w, h) =>
        belowMinShortEdge(w, h) ? MIN_RESOLUTION_WARNING : null
      }
    />
  );
}
