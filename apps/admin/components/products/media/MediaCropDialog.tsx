"use client";

import * as React from "react";
import { useCallback, useEffect, useState } from "react";
import Cropper from "react-easy-crop";
import { cropToBlob, loadImage, type CropBox } from "./cropImage";
import { belowMinShortEdge, MIN_RESOLUTION_WARNING } from "./mediaResolution";

export interface MediaCropDialogProps {
  sourceUrl: string;
  aspect?: number;
  sourceMimeType?: string;
  onApply: (blob: Blob, box: CropBox, rotation: number) => void | Promise<void>;
  onCancel: () => void;
}

const ASPECT_OPTIONS: { label: string; value: number | undefined }[] = [
  { label: "4:5", value: 4 / 5 },
  { label: "1:1", value: 1 },
  { label: "3:4", value: 3 / 4 },
  { label: "Free", value: undefined },
];

export function MediaCropDialog({
  sourceUrl,
  aspect,
  sourceMimeType = "image/jpeg",
  onApply,
  onCancel,
}: MediaCropDialogProps): React.ReactElement {
  const [selectedAspect, setSelectedAspect] = useState<number | undefined>(
    aspect ?? 4 / 5,
  );
  const [crop, setCrop] = useState<{ x: number; y: number }>({ x: 0, y: 0 });
  const [zoom, setZoom] = useState<number>(1);
  const [rotation, setRotation] = useState<number>(0);
  const [pixelCrop, setPixelCrop] = useState<CropBox | null>(null);
  const [busy, setBusy] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  const lowResWarning =
    pixelCrop && belowMinShortEdge(pixelCrop.width, pixelCrop.height)
      ? MIN_RESOLUTION_WARNING
      : null;

  const handleCropComplete = useCallback((_area: CropBox, pixels: CropBox) => {
    setPixelCrop(pixels);
  }, []);

  const handleApply = useCallback(async (): Promise<void> => {
    if (!pixelCrop || busy) return;
    setBusy(true);
    setError(null);
    try {
      const img = await loadImage(sourceUrl);
      const blob = await cropToBlob(img, pixelCrop, rotation, {
        mimeType: sourceMimeType,
        quality: 0.95,
      });
      await onApply(blob, pixelCrop, rotation);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to apply crop");
      setBusy(false);
    }
  }, [pixelCrop, busy, sourceUrl, rotation, sourceMimeType, onApply]);

  useEffect(() => {
    const handler = (e: KeyboardEvent): void => {
      if (e.key === "Escape") {
        e.preventDefault();
        onCancel();
      } else if (e.key === "Enter" && pixelCrop && !busy) {
        e.preventDefault();
        void handleApply();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onCancel, handleApply, pixelCrop, busy]);

  return (
    <div
      role="dialog"
      aria-label="Crop image"
      aria-modal="true"
      className="fixed inset-0 z-50 flex flex-col bg-[var(--paper-200)] font-[var(--font-sans)] text-[var(--ink-900)]"
    >
      <header className="flex items-baseline justify-between border-b border-[var(--ink-100)] px-8 py-5">
        <h2 className="font-[var(--font-serif)] text-2xl tracking-tight">Crop image</h2>
        <span className="text-xs uppercase tracking-widest text-[var(--ink-500)]">
          Editorial workbench
        </span>
      </header>

      <div className="relative flex-1 bg-[var(--ink-900)]">
        <Cropper
          image={sourceUrl}
          crop={crop}
          zoom={zoom}
          rotation={rotation}
          aspect={selectedAspect}
          onCropChange={setCrop}
          onZoomChange={setZoom}
          onRotationChange={setRotation}
          onCropComplete={handleCropComplete}
        />
      </div>

      <div className="flex items-center gap-6 border-t border-[var(--ink-100)] bg-[var(--background-elevated)] px-8 py-4">
        <label className="flex items-center gap-3 text-sm">
          <span className="text-[var(--ink-700)]">Zoom</span>
          <input
            type="range"
            min={1}
            max={3}
            step={0.01}
            value={zoom}
            onChange={(e) => setZoom(Number(e.target.value))}
            className="h-1 w-48 accent-[var(--moss-700)]"
            aria-label="Zoom"
          />
        </label>
        <div className="flex items-center gap-2 text-sm">
          <button
            type="button"
            onClick={() => setRotation((r) => (r - 90 + 360) % 360)}
            className="rounded-md border border-[var(--ink-200)] px-3 py-1 focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
            aria-label="Rotate left"
          >
            Rotate left
          </button>
          <button
            type="button"
            onClick={() => setRotation((r) => (r + 90) % 360)}
            className="rounded-md border border-[var(--ink-200)] px-3 py-1 focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
            aria-label="Rotate right"
          >
            Rotate right
          </button>
        </div>
        <div className="flex items-center gap-2 text-sm" role="group" aria-label="Aspect ratio">
          <span className="text-[var(--ink-700)]">Aspect</span>
          {ASPECT_OPTIONS.map((opt) => {
            const isActive = selectedAspect === opt.value;
            return (
              <button
                key={opt.label}
                type="button"
                aria-pressed={isActive}
                onClick={() => setSelectedAspect(opt.value)}
                className={`rounded-md border px-3 py-1 focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] ${
                  isActive
                    ? "border-[var(--moss-700)] bg-[var(--moss-700)] text-[var(--paper-200)]"
                    : "border-[var(--ink-200)] text-[var(--ink-700)]"
                }`}
              >
                {opt.label}
              </button>
            );
          })}
        </div>
        {lowResWarning ? (
          <p role="status" className="text-sm text-[var(--warning)]">
            {lowResWarning}
          </p>
        ) : null}
        {error ? (
          <p role="alert" className="text-sm text-[var(--danger)]">
            {error}
          </p>
        ) : null}
      </div>

      <footer className="flex items-center justify-end gap-3 border-t border-[var(--ink-100)] bg-[var(--background-elevated)] px-8 py-4">
        <button
          type="button"
          onClick={onCancel}
          className="rounded-md px-4 py-2 text-sm text-[var(--ink-700)] hover:text-[var(--ink-900)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
        >
          Cancel
        </button>
        <button
          type="button"
          onClick={() => void handleApply()}
          disabled={!pixelCrop || busy}
          className="rounded-md bg-[var(--ink-900)] px-5 py-2 text-sm text-[var(--paper-200)] hover:bg-[var(--moss-700)] disabled:opacity-40 focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-2"
        >
          {busy ? "Applying…" : "Apply"}
        </button>
      </footer>
    </div>
  );
}
