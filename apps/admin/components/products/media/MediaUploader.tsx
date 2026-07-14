"use client";

import * as React from "react";
import { useRef } from "react";

export interface MediaUploaderProgressItem {
  id: string;
  filename: string;
  percent: number;
  status: "uploading" | "done" | "error";
  error?: string;
  warning?: string;
}

export interface MediaUploaderProps {
  onFiles: (files: File[]) => void;
  progressItems?: MediaUploaderProgressItem[];
}

export function MediaUploader({
  onFiles,
  progressItems = [],
}: MediaUploaderProps): React.ReactElement {
  const inputRef = useRef<HTMLInputElement>(null);

  const handleFiles = (fileList: FileList | null): void => {
    if (!fileList || fileList.length === 0) return;
    onFiles(Array.from(fileList));
  };

  return (
    <div className="flex flex-col gap-4">
      <label
        htmlFor="media-uploader-input"
        className="flex cursor-pointer flex-col items-center justify-center gap-2 rounded-md border border-dashed border-[var(--ink-200)] bg-[var(--paper-200)] px-8 py-12 text-center text-[var(--ink-700)] transition-colors hover:border-[var(--moss-700)] focus-within:border-[var(--moss-700)] focus-within:ring-2 focus-within:ring-[var(--moss-700)]"
      >
        <span className="font-[var(--font-serif)] text-xl text-[var(--ink-900)]">
          Drop images or click to browse
        </span>
        <span className="text-xs uppercase tracking-widest text-[var(--ink-500)]">
          JPEG · PNG · WEBP
        </span>
        <input
          id="media-uploader-input"
          ref={inputRef}
          type="file"
          multiple
          accept="image/jpeg,image/png,image/webp"
          className="sr-only"
          onChange={(e) => handleFiles(e.target.files)}
        />
      </label>

      {progressItems.length > 0 ? (
        <ul role="list" className="flex flex-col gap-2">
          {progressItems.map((item) => (
            <li
              key={item.id}
              className="flex items-center gap-3 border-b border-[var(--ink-100)] py-2 text-sm"
            >
              <span className="flex-1 truncate text-[var(--ink-900)]">{item.filename}</span>
              <span className="w-40 h-1 overflow-hidden rounded-full bg-[var(--ink-100)]">
                <span
                  className={`block h-full ${
                    item.status === "error"
                      ? "bg-[var(--danger)]"
                      : "bg-[var(--moss-700)]"
                  }`}
                  style={{ width: `${item.percent}%` }}
                />
              </span>
              <span className="w-12 text-right text-xs uppercase tracking-widest text-[var(--ink-500)]">
                {item.status === "done" ? "Done" : item.status === "error" ? "Error" : `${item.percent}%`}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
