"use client";

import { FileWarning, Download } from "lucide-react";

interface CsvErrorSummaryProps {
  jobId: string;
  storeId: string;
  errorCount: number;
}

export function CsvErrorSummary({
  jobId,
  storeId,
  errorCount,
}: CsvErrorSummaryProps) {
  if (errorCount === 0) return null;

  return (
    <div className="flex items-center gap-4 rounded-[6px] border border-[var(--ink-900)]/10 bg-[var(--background-elevated)] px-4 py-3">
      <FileWarning
        className="h-5 w-5 shrink-0 text-[var(--danger,#8B1A1A)]"
        aria-hidden="true"
      />
      <div className="flex flex-1 items-center gap-3">
        <p className="font-[var(--font-body)] text-sm text-[var(--ink-900)]">
          <span className="font-[var(--font-display)] text-base">
            {errorCount}
          </span>{" "}
          {errorCount === 1 ? "row" : "rows"} had errors
        </p>
        <a
          href={`/api/products/csv-imports/${jobId}/errors.csv`}
          download={`import-errors-${jobId}.csv`}
          className={[
            "inline-flex items-center gap-1.5 rounded-[6px] px-3 py-1.5",
            "font-[var(--font-body)] text-sm font-medium text-[var(--moss-700)]",
            "border border-[var(--moss-700)]/20 transition-colors",
            "hover:bg-[var(--moss-700)]/5",
            "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--moss-700)]",
          ].join(" ")}
          aria-label="Download error report"
        >
          <Download className="h-3.5 w-3.5" aria-hidden="true" />
          Download error report
        </a>
      </div>
    </div>
  );
}
