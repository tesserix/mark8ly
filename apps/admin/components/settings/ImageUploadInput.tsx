"use client";

// Branding image uploader — compact, editorial. No boxed card wrapper;
// the uploaded state is just a clickable thumbnail + a trash icon for
// removal. The empty state is a compact upload button, not a full-width
// dashed dropzone (logos and favicons are small — full width reads as
// visual noise).

import { useRef, useState } from "react";
import { Trash2, Upload } from "lucide-react";

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

// Preview size per asset — favicons are tiny (32px), logos render a bit
// larger so the uploaded state gives meaningful preview. Match by kind.
function previewSizeClass(kind: BrandingImageKind): string {
  return kind === "favicon" ? "h-10 w-10" : "h-14 w-14";
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

  const openFilePicker = () => {
    if (disabled || uploading) return;
    inputRef.current?.click();
  };

  return (
    <div className="space-y-1.5">
      {label ? (
        <p className="text-sm font-medium text-foreground">{label}</p>
      ) : null}

      {value ? (
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={openFilePicker}
            disabled={disabled || uploading}
            aria-label="Replace image"
            title="Click to replace"
            className={`${previewSizeClass(kind)} shrink-0 overflow-hidden rounded-md border border-border bg-[color:var(--background-elevated)] transition-colors hover:border-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50`}
          >
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={value}
              alt=""
              className="h-full w-full object-cover"
            />
          </button>
          <button
            type="button"
            onClick={() => onChange(null)}
            disabled={disabled || uploading}
            aria-label="Remove image"
            title="Remove"
            className="inline-flex h-8 w-8 items-center justify-center rounded-md text-foreground-tertiary transition-colors hover:bg-[color:var(--danger)]/[0.06] hover:text-[color:var(--danger)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
          >
            <Trash2 className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      ) : (
        <button
          type="button"
          onClick={openFilePicker}
          disabled={disabled || uploading}
          className="inline-flex items-center gap-2 rounded-md border border-border bg-[color:var(--background-elevated)] px-3 py-2 text-sm text-foreground-secondary transition-colors hover:border-[color:var(--moss-700)] hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
        >
          <Upload className="h-4 w-4" aria-hidden="true" />
          <span>{uploading ? `Uploading… ${progress}%` : "Upload image"}</span>
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

      {uploading && value ? (
        <p className="text-xs text-foreground-secondary">Uploading… {progress}%</p>
      ) : null}
      {error ? <p className="text-xs text-[color:var(--danger)]">{error}</p> : null}
      {hint && !error ? (
        <p className="text-xs text-foreground-secondary">{hint}</p>
      ) : null}
    </div>
  );
}
