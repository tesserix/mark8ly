"use client";

import { useCallback, useRef, useState } from "react";
import { Upload, FileText } from "lucide-react";

interface CsvUploadDropzoneProps {
  onFileSelected: (file: File) => void;
  selectedFileName?: string;
  disabled?: boolean;
}

export function CsvUploadDropzone({
  onFileSelected,
  selectedFileName,
  disabled = false,
}: CsvUploadDropzoneProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const validateAndSelect = useCallback(
    (file: File) => {
      setError(null);
      if (!file.name.endsWith(".csv") && file.type !== "text/csv") {
        setError("Please select a CSV file.");
        return;
      }
      onFileSelected(file);
    },
    [onFileSelected],
  );

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragOver(false);
      if (disabled) return;
      const file = e.dataTransfer.files[0];
      if (file) validateAndSelect(file);
    },
    [disabled, validateAndSelect],
  );

  const handleDragOver = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      if (!disabled) setDragOver(true);
    },
    [disabled],
  );

  const handleDragLeave = useCallback(() => {
    setDragOver(false);
  }, []);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (file) validateAndSelect(file);
    },
    [validateAndSelect],
  );

  return (
    <div className="flex flex-col gap-2">
      <div
        role="button"
        tabIndex={0}
        aria-label="Upload CSV file"
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onClick={() => !disabled && inputRef.current?.click()}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            if (!disabled) inputRef.current?.click();
          }
        }}
        className={[
          "flex flex-col items-center justify-center gap-3 rounded-md border border-dashed px-8 py-12",
          "cursor-pointer transition-colors",
          "bg-[var(--paper-200)]",
          dragOver
            ? "border-[var(--moss-700)] bg-[var(--moss-700)]/5"
            : "border-[var(--ink-900)]/20 hover:border-[var(--moss-700)]",
          disabled ? "pointer-events-none opacity-50" : "",
          "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--moss-700)]",
        ].join(" ")}
      >
        {selectedFileName ? (
          <>
            <FileText
              className="h-8 w-8 text-[var(--moss-700)]"
              aria-hidden="true"
            />
            <span className="font-[var(--font-body)] text-sm text-[var(--ink-900)]">
              {selectedFileName}
            </span>
            <span className="font-[var(--font-body)] text-xs text-[var(--ink-900)]/60">
              Click or drop to replace
            </span>
          </>
        ) : (
          <>
            <Upload
              className="h-8 w-8 text-[var(--ink-900)]/40"
              aria-hidden="true"
            />
            <p className="text-center">
              <span className="font-[var(--font-display)] text-base text-[var(--ink-900)]">
                Drag &amp; drop your CSV file here
              </span>
              <br />
              <span className="font-[var(--font-body)] text-sm text-[var(--ink-900)]/60">
                or click to browse
              </span>
            </p>
          </>
        )}
        <span className="font-[var(--font-body)] text-xs text-[var(--ink-900)]/40">
          50,000 row limit per file
        </span>
      </div>
      <input
        ref={inputRef}
        data-testid="csv-file-input"
        type="file"
        accept=".csv,text/csv"
        className="sr-only"
        onChange={handleChange}
        disabled={disabled}
        tabIndex={-1}
      />
      {error && (
        <p role="alert" className="font-[var(--font-body)] text-sm text-[var(--signal)]">
          {error}
        </p>
      )}
    </div>
  );
}
