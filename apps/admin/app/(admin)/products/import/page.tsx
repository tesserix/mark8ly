"use client";

// apps/admin/app/products/import/page.tsx
//
// CSV import page. Route-scoped: papaparse is only loaded here, not in
// the products list bundle. Renders the upload dropzone, preview table,
// column mapping, and import history.

import { useCallback, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import Papa from "papaparse";
import { ArrowLeft } from "lucide-react";
import Link from "next/link";

import { CsvUploadDropzone } from "@/components/products/csv/CsvUploadDropzone";
import { CsvPreviewTable } from "@/components/products/csv/CsvPreviewTable";
import {
  CsvColumnMapping,
  autoMapColumns,
  type ColumnMapping,
} from "@/components/products/csv/CsvColumnMapping";
import { CsvImportHistory } from "@/components/products/csv/CsvImportHistory";
import { submitCsvImportAction } from "./actions";

export default function ImportPage() {
  const router = useRouter();
  const [isPending, startTransition] = useTransition();
  const [file, setFile] = useState<File | null>(null);
  const [previewHeaders, setPreviewHeaders] = useState<string[]>([]);
  const [previewRows, setPreviewRows] = useState<string[][]>([]);
  const [mapping, setMapping] = useState<ColumnMapping>({});
  const [error, setError] = useState<string | null>(null);

  const handleFileSelected = useCallback((selected: File) => {
    setFile(selected);
    setError(null);

    // Parse first 5 rows client-side for preview
    Papa.parse(selected, {
      preview: 6, // 1 header + 5 data rows
      complete: (result) => {
        const allRows = result.data as string[][];
        if (allRows.length === 0) {
          setPreviewHeaders([]);
          setPreviewRows([]);
          return;
        }
        const headers = allRows[0] ?? [];
        const dataRows = allRows.slice(1, 6);
        setPreviewHeaders(headers);
        setPreviewRows(dataRows);
        setMapping(autoMapColumns(headers));
      },
      error: () => {
        setError("Failed to parse CSV file. Please check the file format.");
      },
    });
  }, []);

  const handleSubmit = useCallback(() => {
    if (!file) return;

    startTransition(async () => {
      const formData = new FormData();
      formData.append("file", file);

      const result = await submitCsvImportAction("__STORE_ID__", formData);
      if (!result.ok && result.error) {
        setError(result.error.message);
      }
      // On success, the server action redirects to /products/import/[jobId]
    });
  }, [file]);

  const hasTitleMapping = Object.values(mapping).includes("title");

  return (
    <main
      className="flex flex-col gap-8"
      aria-labelledby="import-heading"
    >
      {/* Header */}
      <div className="flex items-center gap-4">
        <Link
          href="/products"
          className="flex items-center gap-1 font-[var(--font-body)] text-sm text-[var(--ink-900)]/60 hover:text-[var(--moss-700)] transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--moss-700)]"
          aria-label="Back to products"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
        </Link>
        <h1
          id="import-heading"
          className="font-[var(--font-display)] text-2xl text-[var(--ink-900)]"
        >
          Import products
        </h1>
      </div>

      {/* Upload zone */}
      <section aria-label="Upload CSV">
        <CsvUploadDropzone
          onFileSelected={handleFileSelected}
          selectedFileName={file?.name}
          disabled={isPending}
        />
      </section>

      {/* Preview */}
      {previewHeaders.length > 0 && (
        <section aria-label="CSV preview" className="flex flex-col gap-4">
          <h2 className="font-[var(--font-display)] text-lg text-[var(--ink-900)]">
            Preview
          </h2>
          <CsvPreviewTable headers={previewHeaders} rows={previewRows} />

          <CsvColumnMapping
            csvHeaders={previewHeaders}
            mapping={mapping}
            onMappingChange={setMapping}
          />

          {/* Submit */}
          <div className="flex items-center gap-4 pt-2">
            <button
              type="button"
              onClick={handleSubmit}
              disabled={isPending || !hasTitleMapping}
              className={[
                "rounded-md px-5 py-2 font-[var(--font-body)] text-sm font-medium transition-colors",
                "bg-[var(--ink-900)] text-[var(--background-elevated)]",
                "hover:bg-[var(--ink-900)]/85",
                "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--moss-700)]",
                "disabled:opacity-50 disabled:cursor-not-allowed",
              ].join(" ")}
            >
              {isPending ? "Uploading..." : "Start import"}
            </button>
            {!hasTitleMapping && previewHeaders.length > 0 && (
              <p className="font-[var(--font-body)] text-sm text-[var(--ink-900)]/60">
                Map at least the &ldquo;title&rdquo; column to continue
              </p>
            )}
          </div>
        </section>
      )}

      {/* Error */}
      {error && (
        <p role="alert" className="font-[var(--font-body)] text-sm text-[var(--signal)]">
          {error}
        </p>
      )}

      {/* Import history — rendered below */}
      <section aria-label="Import history" className="border-t border-[var(--ink-900)]/10 pt-6">
        <CsvImportHistory />
      </section>
    </main>
  );
}
