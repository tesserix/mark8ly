"use client";

import { useRef, useState } from "react";
import { Upload, X } from "lucide-react";

import {
  ACCEPT,
  MAX_BYTES,
  uploadBrandingImage,
  type BrandingImageKind,
} from "@/lib/brandingUpload";

interface ImageUploadInputProps {
  storeId: string;
  kind: BrandingImageKind;
  value: string | null | undefined;
  onChange: (url: string | null) => void;
  disabled?: boolean;
  label?: string;
  /** Shown under the control — e.g. "PNG, JPG, WebP up to 5 MB". */
  hint?: string;
}

function formatBytes(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${Math.round(bytes / (1024 * 1024))} MB`;
  return `${Math.round(bytes / 1024)} KB`;
}

export function ImageUploadInput({
  storeId,
  kind,
  value,
  onChange,
  disabled,
  label,
  hint,
}: ImageUploadInputProps) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const maxBytes = MAX_BYTES[kind];
  const accept = ACCEPT[kind];

  const handleFile = async (file: File) => {
    setError(null);
    if (file.size > maxBytes) {
      setError(`File too large (max ${formatBytes(maxBytes)})`);
      return;
    }
    try {
      setUploading(true);
      setProgress(0);
      const publicUrl = await uploadBrandingImage(storeId, file, kind, setProgress);
      onChange(publicUrl);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Upload failed");
    } finally {
      setUploading(false);
    }
  };

  const onClick = () => {
    if (disabled || uploading) return;
    inputRef.current?.click();
  };

  return (
    <div className="space-y-1.5">
      {label ? (
        <p className="text-sm font-medium text-foreground">{label}</p>
      ) : null}

      {value ? (
        <div className="flex items-start gap-3 rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] p-3">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={value}
            alt=""
            className="h-16 w-16 shrink-0 rounded border border-border object-cover"
          />
          <div className="min-w-0 flex-1 space-y-1">
            <p className="truncate text-xs text-foreground-secondary">{value}</p>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={onClick}
                disabled={disabled || uploading}
                className="inline-flex h-8 items-center rounded-md border border-border bg-[color:var(--background)] px-3 text-xs font-medium text-foreground hover:border-[color:var(--moss-700)] disabled:opacity-50"
              >
                Replace
              </button>
              <button
                type="button"
                onClick={() => onChange(null)}
                disabled={disabled || uploading}
                className="inline-flex h-8 items-center gap-1 rounded-md border border-transparent px-2 text-xs text-foreground-secondary hover:text-danger disabled:opacity-50"
                aria-label="Remove image"
              >
                <X className="h-3.5 w-3.5" />
                Remove
              </button>
            </div>
          </div>
        </div>
      ) : (
        <button
          type="button"
          onClick={onClick}
          disabled={disabled || uploading}
          className="flex w-full flex-col items-center justify-center gap-2 rounded-[var(--radius)] border border-dashed border-border bg-[color:var(--background-elevated)] px-4 py-6 text-sm text-foreground-secondary transition-colors hover:border-[color:var(--moss-700)] hover:text-foreground disabled:opacity-50"
        >
          <Upload className="h-5 w-5" />
          <span>{uploading ? `Uploading… ${progress}%` : "Click to upload an image"}</span>
        </button>
      )}

      <input
        ref={inputRef}
        type="file"
        accept={accept}
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) handleFile(f);
          e.target.value = "";
        }}
      />

      {uploading ? (
        <p className="text-xs text-foreground-secondary">Uploading… {progress}%</p>
      ) : null}
      {error ? <p className="text-xs text-danger">{error}</p> : null}
      {hint && !error ? (
        <p className="text-xs text-foreground-secondary">{hint}</p>
      ) : null}
    </div>
  );
}
