"use client";

// The buyer's personalisation form (#965).
//
// Renders one control per field the merchant defined. The form owns no
// rules of its own: validation lives in lib/personalisation.ts and the
// API sends only the keys that apply to a kind, so presence is the signal
// — `max_length` on a text field, `min_px` on an image.
//
// Nothing here is a security boundary. Checkout revalidates every answer
// and reprices every delta from the catalog (#967). This governs what the
// buyer is allowed to try, and what they are told before they try it.

import { useCallback, useState } from "react";

import type { StorefrontPersonalisationField } from "@/lib/api/marketplace-api";
import {
  isBelowMinimumResolution,
  type PersonalisationAnswers,
  type PersonalisationProblem,
} from "@/lib/personalisation";
import {
  deletePersonalisationUpload,
  uploadPersonalisationImage,
} from "@/lib/personalisation-upload";
import {
  PREVIEW_MAX_DIMENSION,
  prepareCrop,
  uploadCropPreview,
  type CropRect,
} from "@/lib/personalisation-crop";
import { ImageCropDialog } from "@repo/ui/image-crop-dialog";
import type { CropBox } from "@repo/ui/crop-image";

export interface PersonalisationFormProps {
  storeSlug: string;
  productId: string;
  fields: readonly StorefrontPersonalisationField[];
  answers: PersonalisationAnswers;
  onChange: (next: PersonalisationAnswers) => void;
  /** Problems to show, supplied by the parent after a submit attempt. */
  problems?: readonly PersonalisationProblem[];
}

interface UploadState {
  busy: boolean;
  error?: string;
  /** Measured client-side; drives the low-resolution warning only. */
  softWarning?: string;
}

const inputClass =
  "w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-transparent px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]";

export function PersonalisationForm({
  storeSlug,
  productId,
  fields,
  answers,
  onChange,
  problems = [],
}: PersonalisationFormProps) {
  const [uploads, setUploads] = useState<Record<string, UploadState>>({});
  // The upload currently being cropped, plus the signed GET for its
  // PRISTINE original to crop against. Null when no dialog is open.
  const [cropping, setCropping] = useState<{
    field: StorefrontPersonalisationField;
    uploadId: string;
    sourceUrl: string;
    uploadUrl: string;
  } | null>(null);

  const problemFor = useCallback(
    (fieldId: string) => problems.find((p) => p.fieldId === fieldId)?.message,
    [problems],
  );

  const setAnswer = useCallback(
    (fieldId: string, patch: Partial<PersonalisationAnswers[string]>) => {
      onChange({ ...answers, [fieldId]: { ...answers[fieldId], ...patch } });
    },
    [answers, onChange],
  );

  const handleFile = useCallback(
    async (field: StorefrontPersonalisationField, file: File | undefined) => {
      if (!file) return;
      setUploads((u) => ({ ...u, [field.id]: { busy: true } }));
      try {
        const { uploadId, dimensions } = await uploadPersonalisationImage(
          storeSlug,
          { productId, fieldId: field.id, file },
          {},
        );
        const existing = answers[field.id]?.uploadIds ?? [];
        const max = field.max_images ?? 1;
        // Replace rather than append when only one is allowed: a buyer
        // picking a second photo means "use this one instead", and
        // silently refusing would read as a broken picker.
        const nextIds = max === 1 ? [uploadId] : [...existing, uploadId].slice(0, max);
        if (max === 1 && existing[0] && existing[0] !== uploadId) {
          void deletePersonalisationUpload(storeSlug, existing[0]);
        }
        onChange({ ...answers, [field.id]: { ...answers[field.id], uploadIds: nextIds } });

        setUploads((u) => ({
          ...u,
          [field.id]: {
            busy: false,
            softWarning: isBelowMinimumResolution(field, dimensions)
              ? `This image is smaller than ${field.min_px}px, so it may look soft when printed. A larger photo will print better.`
              : undefined,
          },
        }));
      } catch (err) {
        setUploads((u) => ({
          ...u,
          [field.id]: {
            busy: false,
            error: err instanceof Error ? err.message : "That image could not be accepted.",
          },
        }));
      }
    },
    [answers, onChange, productId, storeSlug],
  );

  const removeUpload = useCallback(
    (field: StorefrontPersonalisationField, uploadId: string) => {
      const next = (answers[field.id]?.uploadIds ?? []).filter((id) => id !== uploadId);
      onChange({ ...answers, [field.id]: { ...answers[field.id], uploadIds: next } });
      void deletePersonalisationUpload(storeSlug, uploadId);
      setUploads((u) => ({ ...u, [field.id]: { busy: false } }));
    },
    [answers, onChange, storeSlug],
  );

  // Opening the dialog is itself a server round trip: PrepareCrop stores
  // the rectangle and hands back a signed GET for the original and a
  // signed PUT for the preview. Opened with the buyer's current crop (or
  // the whole image first time) so the rectangle is recorded even if they
  // abandon the dialog.
  const beginCrop = useCallback(
    async (field: StorefrontPersonalisationField, uploadId: string) => {
      setUploads((u) => ({ ...u, [field.id]: { busy: true } }));
      try {
        // A zero-origin, whole-image rectangle is a safe opening value:
        // the server validates it as positive, and the real one replaces
        // it the moment the buyer applies a crop.
        const { sourceUrl, uploadUrl } = await prepareCrop(storeSlug, uploadId, {
          x: 0,
          y: 0,
          w: 1,
          h: 1,
          rotation: 0,
        });
        setCropping({ field, uploadId, sourceUrl, uploadUrl });
        setUploads((u) => ({ ...u, [field.id]: { busy: false } }));
      } catch (err) {
        setUploads((u) => ({
          ...u,
          [field.id]: {
            busy: false,
            error: err instanceof Error ? err.message : "That crop could not be started.",
          },
        }));
      }
    },
    [storeSlug],
  );

  // box arrives in ORIGINAL pixels and is what gets persisted; blob is a
  // ~1024px preview and is disposable. Getting that round the wrong way
  // is how a figurine ships with a blurry face — see
  // lib/personalisation-crop.ts.
  const applyCrop = useCallback(
    async (blob: Blob, box: CropBox, rotation: number) => {
      if (!cropping) return;
      const { field, uploadId, uploadUrl } = cropping;
      const rect: CropRect = {
        x: Math.round(box.x),
        y: Math.round(box.y),
        w: Math.round(box.width),
        h: Math.round(box.height),
        rotation,
      };
      setCropping(null);
      setUploads((u) => ({ ...u, [field.id]: { busy: true } }));
      try {
        // Rectangle first, preview second. The rectangle is the durable
        // record of what the buyer chose; a preview that fails to upload
        // leaves a correct crop with a stale thumbnail rather than a lost
        // choice, which is why the PUT is not rolled back on failure.
        await prepareCrop(storeSlug, uploadId, rect);
        await uploadCropPreview(uploadUrl, blob);
        setUploads((u) => ({ ...u, [field.id]: { busy: false } }));
      } catch (err) {
        setUploads((u) => ({
          ...u,
          [field.id]: {
            busy: false,
            error: err instanceof Error ? err.message : "That crop could not be saved.",
          },
        }));
      }
    },
    [cropping, storeSlug],
  );

  if (fields.length === 0) return null;

  return (
    <section className="flex flex-col gap-6 border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-6">
      <h2 className="font-serif text-lg text-[color:var(--storefront-text,var(--ink-900))]">
        Make it yours
      </h2>

      {[...fields]
        .sort((a, b) => a.position - b.position || a.key.localeCompare(b.key))
        .map((field) => {
          const answer = answers[field.id] ?? {};
          const problem = problemFor(field.id);
          const state = uploads[field.id];
          const describedBy = problem ? `${field.id}-error` : undefined;

          return (
            <div key={field.id} className="flex flex-col gap-2">
              <label
                htmlFor={field.id}
                className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
              >
                {field.label}
                {field.required && (
                  <span aria-hidden className="ml-1 opacity-60">
                    *
                  </span>
                )}
                {!field.required && <span className="ml-2 text-xs opacity-60">(optional)</span>}
              </label>

              {field.help_text && (
                <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
                  {field.help_text}
                </p>
              )}

              {field.kind === "image" && (
                <div className="flex flex-col gap-2">
                  <input
                    id={field.id}
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    disabled={state?.busy}
                    aria-invalid={problem ? true : undefined}
                    aria-describedby={describedBy}
                    onChange={(e) => void handleFile(field, e.target.files?.[0])}
                    className="text-sm"
                  />
                  {state?.busy && (
                    <p role="status" className="text-xs opacity-70">
                      Uploading…
                    </p>
                  )}
                  {(answer.uploadIds ?? []).map((id) => (
                    <div key={id} className="flex items-center gap-3 text-xs">
                      <span className="opacity-70">Image added</span>
                      <button
                        type="button"
                        onClick={() => void beginCrop(field, id)}
                        className="underline opacity-70 hover:opacity-100"
                      >
                        Crop
                      </button>
                      <button
                        type="button"
                        onClick={() => removeUpload(field, id)}
                        className="underline opacity-70 hover:opacity-100"
                      >
                        Remove
                      </button>
                    </div>
                  ))}
                  {state?.softWarning && (
                    // A warning, not an error: the buyer may know their
                    // photo is small and want it anyway. Blocking here
                    // would be us overruling them about their own image.
                    <p role="status" className="text-xs text-[color:var(--storefront-warning)]">
                      {state.softWarning}
                    </p>
                  )}
                  {state?.error && (
                    <p role="alert" className="text-xs text-[color:var(--storefront-danger)]">
                      {state.error}
                    </p>
                  )}
                </div>
              )}

              {field.kind === "text" && (
                <input
                  id={field.id}
                  type="text"
                  maxLength={field.max_length}
                  value={answer.text ?? ""}
                  aria-invalid={problem ? true : undefined}
                  aria-describedby={describedBy}
                  onChange={(e) => setAnswer(field.id, { text: e.target.value })}
                  className={inputClass}
                />
              )}

              {field.kind === "textarea" && (
                <textarea
                  id={field.id}
                  rows={3}
                  maxLength={field.max_length}
                  value={answer.text ?? ""}
                  aria-invalid={problem ? true : undefined}
                  aria-describedby={describedBy}
                  onChange={(e) => setAnswer(field.id, { text: e.target.value })}
                  className={`${inputClass} resize-y`}
                />
              )}

              {field.kind === "select" && (
                <select
                  id={field.id}
                  value={answer.optionId ?? ""}
                  aria-invalid={problem ? true : undefined}
                  aria-describedby={describedBy}
                  onChange={(e) => setAnswer(field.id, { optionId: e.target.value || undefined })}
                  className={inputClass}
                >
                  <option value="">Choose…</option>
                  {(field.options ?? []).map((o) => (
                    <option key={o.id} value={o.id}>
                      {o.label}
                      {Number.parseFloat(o.price_delta) > 0 ? ` (+${o.price_delta})` : ""}
                    </option>
                  ))}
                </select>
              )}

              {field.kind === "checkbox" && (
                <label className="flex items-center gap-2 text-sm">
                  <input
                    id={field.id}
                    type="checkbox"
                    checked={answer.checked === true}
                    aria-invalid={problem ? true : undefined}
                    aria-describedby={describedBy}
                    onChange={(e) => setAnswer(field.id, { checked: e.target.checked })}
                  />
                  <span>
                    {field.label}
                    {Number.parseFloat(field.price_delta ?? "0") > 0 && ` (+${field.price_delta})`}
                  </span>
                </label>
              )}

              {field.max_length !== undefined &&
                (field.kind === "text" || field.kind === "textarea") && (
                  <p className="text-right text-xs opacity-60">
                    {(answer.text ?? "").length}/{field.max_length}
                  </p>
                )}

              {problem && (
                <p
                  id={`${field.id}-error`}
                  role="alert"
                  className="text-xs text-[color:var(--storefront-danger)]"
                >
                  {problem}
                </p>
              )}
            </div>
          );
        })}
    
      {cropping ? (
        <ImageCropDialog
          sourceUrl={cropping.sourceUrl}
          onApply={applyCrop}
          onCancel={() => setCropping(null)}
          title="Crop your photo"
          applyLabel="Use this crop"
          // The blob is a PREVIEW. The pristine original stays in the
          // bucket and is what the merchant prints from.
          previewMaxDimension={PREVIEW_MAX_DIMENSION}
          // The merchant's own min_px for this field, not a fixed house
          // number — it is what the buyer was told on the form.
          lowResolutionWarning={(w, h) =>
            cropping.field.min_px && Math.min(w, h) < cropping.field.min_px
              ? `This crop is smaller than ${cropping.field.min_px}px, so it may look soft when printed.`
              : null
          }
        />
      ) : null}
</section>
  );
}
