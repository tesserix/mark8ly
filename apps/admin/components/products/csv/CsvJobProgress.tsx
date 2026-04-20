"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { Ban } from "lucide-react";

import type { CsvImportJob, CsvImportJobStatus } from "@/lib/api/csvImports";

const TERMINAL_STATUSES: ReadonlySet<CsvImportJobStatus> = new Set([
  "completed",
  "failed",
  "cancelled",
]);

function statusColor(status: CsvImportJobStatus): string {
  switch (status) {
    case "completed":
      return "bg-[var(--moss-700)] text-[var(--background-elevated)]";
    case "failed":
      return "bg-[var(--danger)] text-[var(--background-elevated)]";
    case "cancelled":
      return "bg-[var(--ink-900)]/20 text-[var(--ink-900)]";
    default:
      return "bg-[var(--moss-700)]/15 text-[var(--moss-700)]";
  }
}

interface CsvJobProgressProps {
  job: CsvImportJob;
  storeId: string;
  onCancel: () => void;
  /** Optional callback to fetch latest job status (for polling). */
  onPoll?: () => void;
}

export function CsvJobProgress({
  job,
  storeId,
  onCancel,
  onPoll,
}: CsvJobProgressProps) {
  const router = useRouter();
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const isTerminal = TERMINAL_STATUSES.has(job.status);
  const canCancel = job.status === "queued" || job.status === "running";
  const totalRows = job.total_rows ?? 0;
  const processed = job.success_count + job.error_count;
  const percent = totalRows > 0 ? Math.round((processed / totalRows) * 100) : 0;

  // Poll every 2s when not terminal
  useEffect(() => {
    if (isTerminal) {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
      return;
    }

    intervalRef.current = setInterval(() => {
      if (onPoll) {
        onPoll();
      } else {
        router.refresh();
      }
    }, 2000);

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, [isTerminal, onPoll, router]);

  return (
    <div className="flex flex-col gap-5">
      {/* Status badge */}
      <div className="flex items-center gap-3">
        <span
          className={[
            "inline-flex rounded-full px-2.5 py-0.5 font-[var(--font-body)] text-xs font-medium",
            statusColor(job.status),
          ].join(" ")}
        >
          {job.status}
        </span>
        {canCancel && (
          <button
            type="button"
            onClick={onCancel}
            className={[
              "inline-flex items-center gap-1.5 rounded-md px-3 py-1.5",
              "font-[var(--font-body)] text-sm text-[var(--ink-900)]/70",
              "border border-[var(--ink-900)]/10 transition-colors",
              "hover:border-[var(--danger)] hover:text-[var(--danger)]",
              "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--moss-700)]",
            ].join(" ")}
            aria-label="Cancel import"
          >
            <Ban className="h-3.5 w-3.5" aria-hidden="true" />
            Cancel
          </button>
        )}
      </div>

      {/* Progress bar */}
      <div className="flex flex-col gap-2">
        <div
          role="progressbar"
          aria-valuenow={percent}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label={`Import progress: ${percent}%`}
          className="h-2 w-full overflow-hidden rounded-full bg-[var(--paper-200)]"
        >
          <div
            className="h-full rounded-full bg-[var(--moss-700)] transition-[width] duration-300 ease-out"
            style={{
              width: `${percent}%`,
              ...(percent > 0 ? {} : { minWidth: 0 }),
            }}
          />
        </div>
        <p className="font-[var(--font-body)] text-xs text-[var(--ink-900)]/60">
          {processed} of {totalRows} rows processed ({percent}%)
        </p>
      </div>

      {/* Counters */}
      <div className="flex gap-8">
        <div className="flex flex-col">
          <span className="font-[var(--font-display)] text-2xl text-[var(--moss-700)]">
            {job.success_count}
          </span>
          <span className="font-[var(--font-body)] text-xs text-[var(--ink-900)]/60">
            successful
          </span>
        </div>
        <div className="flex flex-col">
          <span className="font-[var(--font-display)] text-2xl text-[var(--danger,#8B1A1A)]">
            {job.error_count}
          </span>
          <span className="font-[var(--font-body)] text-xs text-[var(--ink-900)]/60">
            errors
          </span>
        </div>
        {totalRows > 0 && (
          <div className="flex flex-col">
            <span className="font-[var(--font-display)] text-2xl text-[var(--ink-900)]">
              {totalRows}
            </span>
            <span className="font-[var(--font-body)] text-xs text-[var(--ink-900)]/60">
              total rows
            </span>
          </div>
        )}
      </div>
    </div>
  );
}
