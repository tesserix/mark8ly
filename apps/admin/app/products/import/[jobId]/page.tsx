"use client";

// apps/admin/app/products/import/[jobId]/page.tsx
//
// CSV import job status page with live polling. Uses client-side
// fetching for 2s polling (getCsvImportStatus is a server-only
// function, so we use a lightweight client fetch wrapper here).

import { useCallback, useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";

import { CsvJobProgress } from "@/components/products/csv/CsvJobProgress";
import { CsvErrorSummary } from "@/components/products/csv/CsvErrorSummary";
import type { CsvImportJob, CsvImportJobStatus } from "@/lib/api/csvImports";
import { cancelCsvImportAction } from "../actions";

const TERMINAL_STATUSES: ReadonlySet<CsvImportJobStatus> = new Set([
  "completed",
  "failed",
  "cancelled",
]);

export default function JobStatusPage() {
  const params = useParams<{ jobId: string }>();
  const jobId = params.jobId;

  const [job, setJob] = useState<CsvImportJob | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Poll for job status — uses the Next.js API route proxy pattern
  // that hits marketplace-api from the client via a lightweight
  // fetch to an internal API route. For simplicity we call the
  // marketplace-api directly here (same-origin proxy).
  const fetchStatus = useCallback(async () => {
    try {
      const res = await fetch(`/api/products/csv-imports/${jobId}`);
      if (!res.ok) {
        setError("Failed to fetch import status");
        return;
      }
      const data = (await res.json()) as CsvImportJob;
      setJob(data);
    } catch {
      setError("Failed to fetch import status");
    }
  }, [jobId]);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus]);

  // Polling handled inside CsvJobProgress, but we also poll here
  // on initial load to get the job data
  useEffect(() => {
    if (!job) return;
    if (TERMINAL_STATUSES.has(job.status)) return;

    const interval = setInterval(fetchStatus, 2000);
    return () => clearInterval(interval);
  }, [job, fetchStatus]);

  const handleCancel = useCallback(async () => {
    if (!job) return;
    const result = await cancelCsvImportAction(job.store_id, jobId);
    if (!result.ok && result.error) {
      setError(result.error.message);
    } else {
      fetchStatus();
    }
  }, [job, jobId, fetchStatus]);

  return (
    <main
      className="flex flex-col gap-8 px-8 py-6"
      aria-labelledby="job-status-heading"
    >
      {/* Header */}
      <div className="flex items-center gap-4">
        <Link
          href="/products/import"
          className="flex items-center gap-1 font-[var(--font-body)] text-sm text-[var(--ink-900)]/60 hover:text-[var(--moss-700)] transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--moss-700)]"
          aria-label="Back to import"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
        </Link>
        <h1
          id="job-status-heading"
          className="font-[var(--font-display)] text-2xl text-[var(--ink-900)]"
        >
          Import status
        </h1>
      </div>

      {error && (
        <p role="alert" className="font-[var(--font-body)] text-sm text-[var(--signal)]">
          {error}
        </p>
      )}

      {job ? (
        <div className="flex flex-col gap-6">
          <CsvJobProgress
            job={job}
            storeId={job.store_id}
            onCancel={handleCancel}
            onPoll={fetchStatus}
          />

          {/* Error summary — shown when errors exist and job is terminal */}
          {job.error_count > 0 &&
            TERMINAL_STATUSES.has(job.status) && (
              <CsvErrorSummary
                jobId={jobId}
                storeId={job.store_id}
                errorCount={job.error_count}
              />
            )}
        </div>
      ) : !error ? (
        <p className="font-[var(--font-body)] text-sm text-[var(--ink-900)]/60">
          Loading...
        </p>
      ) : null}
    </main>
  );
}
