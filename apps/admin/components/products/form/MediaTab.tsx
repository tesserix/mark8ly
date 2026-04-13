"use client";

import * as React from "react";
import { useCallback, useMemo, useState } from "react";
import { useFieldArray, useFormContext } from "react-hook-form";
import type {
  AdminMediaResponse,
  RecropMediaResult,
  SessionHeaders,
  UpdateMediaInput,
} from "@/lib/api/marketplace-api";
import {
  deleteMedia as defaultDeleteMedia,
  recropMedia as defaultRecropMedia,
  updateMedia as defaultUpdateMedia,
} from "@/lib/api/marketplace-api";
import { uploadMediaFile as defaultUploadMediaFile } from "@/components/products/media/mediaUploadClient";
import type { ProductFormValues } from "@/lib/validation/product-form";
import { MediaGrid } from "@/components/products/media/MediaGrid";
import {
  MediaUploader,
  type MediaUploaderProgressItem,
} from "@/components/products/media/MediaUploader";
import type { MediaAction } from "@/components/products/media/MediaCard";
import { MediaCropDialog } from "@/components/products/media/MediaCropDialog";
import type { CropBox } from "@/components/products/media/cropImage";

type MutationResult<T> = { ok: true; data: T } | { ok: false; error: { message: string } };

export interface MediaTabDeps {
  uploadMediaFile?: typeof defaultUploadMediaFile;
  recropMedia?: (
    storeId: string,
    productId: string,
    mediaId: string,
    body: { crop_box: CropBox; rotation?: number; filename?: string },
    session: SessionHeaders,
  ) => Promise<MutationResult<RecropMediaResult>>;
  updateMedia?: (
    storeId: string,
    productId: string,
    mediaId: string,
    body: UpdateMediaInput,
    session: SessionHeaders,
  ) => Promise<MutationResult<true>>;
  deleteMedia?: (
    storeId: string,
    productId: string,
    mediaId: string,
    session: SessionHeaders,
  ) => Promise<MutationResult<true>>;
  putBlob?: (url: string, blob: Blob) => Promise<{ ok: boolean; status: number }>;
}

export interface MediaTabProps extends MediaTabDeps {
  storeId: string;
  productId: string;
  session: SessionHeaders;
}

interface CropTarget {
  mediaId?: string;
  sourceUrl: string;
  mode: "fresh" | "recrop";
  file?: File;
  uploadUrl?: string;
  newStorageKey?: string;
}

async function defaultPutBlob(
  url: string,
  blob: Blob,
): Promise<{ ok: boolean; status: number }> {
  const res = await fetch(url, { method: "PUT", body: blob });
  return { ok: res.ok, status: res.status };
}

function mediaToField(m: AdminMediaResponse): ProductFormValues["media"] extends (infer U)[] | undefined ? U : never {
  // The finalize-media response omits `variant_id` for product-scope
  // uploads, leaving it as `undefined`. The form's Zod schema requires
  // `z.string().nullable()` (rejects undefined) — coerce here so the
  // form validates and `handleSubmit` actually fires on Save.
  return {
    id: m.id,
    url: m.url,
    alt: m.alt ?? "",
    position: m.position,
    variant_id: m.variant_id ?? null,
    storage_key: m.storage_key,
    gcs_path_original: "",
  };
}

export function MediaTab({
  storeId,
  productId,
  session,
  uploadMediaFile = defaultUploadMediaFile,
  recropMedia = defaultRecropMedia,
  updateMedia = defaultUpdateMedia,
  deleteMedia = defaultDeleteMedia,
  putBlob = defaultPutBlob,
}: MediaTabProps): React.ReactElement {
  const { control } = useFormContext<ProductFormValues>();
  const { fields, append, remove, replace, update } = useFieldArray({
    control,
    name: "media",
    keyName: "_rhfKey",
  });

  const [progress, setProgress] = useState<MediaUploaderProgressItem[]>([]);
  const [cropTarget, setCropTarget] = useState<CropTarget | null>(null);

  // Convert field array back to AdminMediaResponse shape for MediaGrid.
  const gridItems = useMemo<AdminMediaResponse[]>(
    () =>
      fields.map((f) => ({
        id: f.id,
        url: f.url,
        storage_key: f.storage_key,
        alt: f.alt ?? "",
        position: f.position,
        media_type: "image",
        variant_id: f.variant_id ?? null,
        width: null,
        height: null,
        bytes: null,
      })),
    [fields],
  );

  const setProgressItem = useCallback(
    (id: string, patch: Partial<MediaUploaderProgressItem>) => {
      setProgress((prev) =>
        prev.map((p) => (p.id === id ? { ...p, ...patch } : p)),
      );
    },
    [],
  );

  const uploadOne = useCallback(
    async (file: File, position: number, progressId: string): Promise<void> => {
      try {
        const result = await uploadMediaFile({
          storeId,
          productId,
          file,
          position,
          onProgress: (pct) => setProgressItem(progressId, { percent: pct }),
        });
        append(mediaToField(result));
        setProgressItem(progressId, { percent: 100, status: "done" });
      } catch (err: unknown) {
        setProgressItem(progressId, {
          status: "error",
          error: err instanceof Error ? err.message : "upload failed",
        });
      }
    },
    [append, productId, storeId, setProgressItem, uploadMediaFile],
  );

  const [pendingFreshQueue, setPendingFreshQueue] = useState<File[]>([]);

  // When a user drops files we open the crop dialog on the first one.
  // Each Apply produces a blob that we then upload via uploadOne.
  const handleFiles = useCallback(
    (files: File[]) => {
      const first = files[0];
      if (!first) return;
      const rest = files.slice(1);
      setPendingFreshQueue(rest);
      const url = URL.createObjectURL(first);
      setCropTarget({ sourceUrl: url, mode: "fresh", file: first });
    },
    [],
  );

  const handleAction = useCallback(
    (action: MediaAction, media: AdminMediaResponse) => {
      if (action === "delete") {
        void (async () => {
          const idx = fields.findIndex((f) => f.id === media.id);
          if (idx >= 0) remove(idx);
          await deleteMedia(storeId, productId, media.id, session);
        })();
        return;
      }
      if (action === "set-primary") {
        const idx = fields.findIndex((f) => f.id === media.id);
        if (idx <= 0) return;
        const next = [...gridItems];
        const moved = next.splice(idx, 1)[0];
        if (!moved) return;
        next.unshift(moved);
        replace(next.map(mediaToField));
        return;
      }
      if (action === "crop") {
        void (async () => {
          const res = await recropMedia(
            storeId,
            productId,
            media.id,
            { crop_box: { x: 0, y: 0, width: 0, height: 0 } },
            session,
          );
          if (!res.ok) return;
          setCropTarget({
            mediaId: media.id,
            sourceUrl: res.data.source_original_url,
            mode: "recrop",
            uploadUrl: res.data.upload_url,
            newStorageKey: res.data.new_storage_key,
          });
        })();
      }
    },
    [deleteMedia, fields, gridItems, productId, recropMedia, remove, replace, session, storeId],
  );

  const handleReorder = useCallback(
    (next: AdminMediaResponse[]) => {
      replace(next.map(mediaToField));
    },
    [replace],
  );

  const handleCropApply = useCallback(
    async (blob: Blob, _box: CropBox, _rotation: number) => {
      if (!cropTarget) return;
      const target = cropTarget;
      setCropTarget(null);
      URL.revokeObjectURL(target.sourceUrl);

      if (target.mode === "fresh" && target.file) {
        const freshFile = target.file;
        const progressId = crypto.randomUUID();
        setProgress((p) => [
          ...p,
          { id: progressId, filename: freshFile.name, percent: 0, status: "uploading" },
        ]);
        const croppedFile = new File([blob], freshFile.name, { type: "image/jpeg" });
        await uploadOne(croppedFile, fields.length, progressId);
        // Drain any queued files (one crop dialog each).
        const nextFile = pendingFreshQueue[0];
        if (nextFile) {
          setPendingFreshQueue(pendingFreshQueue.slice(1));
          const url = URL.createObjectURL(nextFile);
          setCropTarget({ sourceUrl: url, mode: "fresh", file: nextFile });
        }
        return;
      }

      if (target.mode === "recrop" && target.mediaId && target.uploadUrl && target.newStorageKey) {
        const putRes = await putBlob(target.uploadUrl, blob);
        if (!putRes.ok) return;
        const updateRes = await updateMedia(
          storeId,
          productId,
          target.mediaId,
          { storage_key: target.newStorageKey },
          session,
        );
        if (!updateRes.ok) return;
        const idx = fields.findIndex((f) => f.id === target.mediaId);
        const current = idx >= 0 ? fields[idx] : undefined;
        if (current) {
          const nextItem: ProductFormValues["media"] extends (infer U)[] | undefined ? U : never = {
            id: current.id,
            url: current.url,
            alt: current.alt,
            position: current.position,
            variant_id: current.variant_id,
            storage_key: target.newStorageKey,
            gcs_path_original: current.gcs_path_original,
          };
          update(idx, nextItem);
        }
      }
    },
    [cropTarget, fields, pendingFreshQueue, productId, putBlob, session, storeId, update, updateMedia, uploadOne],
  );

  const handleCropCancel = useCallback(() => {
    if (cropTarget) URL.revokeObjectURL(cropTarget.sourceUrl);
    setCropTarget(null);
    setPendingFreshQueue([]);
  }, [cropTarget]);

  return (
    <div className="flex flex-col gap-6">
      <MediaUploader onFiles={handleFiles} progressItems={progress} />
      <MediaGrid items={gridItems} onReorder={handleReorder} onAction={handleAction} />
      {cropTarget ? (
        <MediaCropDialog
          sourceUrl={cropTarget.sourceUrl}
          onApply={handleCropApply}
          onCancel={handleCropCancel}
        />
      ) : null}
    </div>
  );
}
