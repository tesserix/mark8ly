"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Eye } from "lucide-react";

import type {
  CsvImportJob,
  CsvImportJobStatus,
  ListCsvImportsResponse,
} from "@/lib/api/csvImports";

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

function extractFilename(gcsPath: string): string {
  const parts = gcsPath.split("/");
  return parts[parts.length - 1] ?? gcsPath;
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString(undefined, {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

export function CsvImportHistory() {
  const [jobs, setJobs] = useState<CsvImportJob[]>([]);
  const [loaded, setLoaded] = useState(false);

  const fetchJobs = useCallback(async () => {
    try {
      const res = await fetch("/api/products/csv-imports?page=1&page_size=10");
      if (!res.ok) return;
      const body = (await res.json()) as ListCsvImportsResponse;
      setJobs(body.data ?? []);
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    fetchJobs();
  }, [fetchJobs]);

  if (!loaded) {
    return (
      <p className="font-[var(--font-body)] text-sm text-[var(--ink-900)]/60 py-2">
        Loading history...
      </p>
    );
  }

  if (jobs.length === 0) {
    return (
      <p className="font-[var(--font-body)] text-sm text-[var(--ink-900)]/60 py-2">
        No import history yet
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <h2 className="font-[var(--font-display)] text-lg text-[var(--ink-900)]">
        Recent imports
      </h2>
      <div className="overflow-x-auto rounded-md border border-[var(--ink-900)]/10">
        <table className="w-full text-left font-[var(--font-body)] text-sm">
          <thead>
            <tr className="border-b border-[var(--ink-900)]/10 bg-[var(--paper-200)]">
              <th className="px-3 py-2 font-medium text-[var(--ink-900)]/70">
                Date
              </th>
              <th className="px-3 py-2 font-medium text-[var(--ink-900)]/70">
                File
              </th>
              <th className="px-3 py-2 font-medium text-[var(--ink-900)]/70">
                Status
              </th>
              <th className="px-3 py-2 font-medium text-[var(--ink-900)]/70 text-right">
                Rows
              </th>
              <th className="px-3 py-2 font-medium text-[var(--ink-900)]/70 text-right">
                Success
              </th>
              <th className="px-3 py-2 font-medium text-[var(--ink-900)]/70 text-right">
                Errors
              </th>
              <th className="px-3 py-2 font-medium text-[var(--ink-900)]/70">
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {jobs.map((job) => (
              <tr
                key={job.id}
                className="border-b border-[var(--ink-900)]/5 last:border-b-0"
              >
                <td className="px-3 py-2 text-[var(--ink-900)]/70 whitespace-nowrap">
                  {formatDate(job.created_at)}
                </td>
                <td className="px-3 py-2 text-[var(--ink-900)] truncate max-w-[200px]">
                  {extractFilename(job.gcs_path)}
                </td>
                <td className="px-3 py-2">
                  <span
                    className={[
                      "inline-flex rounded-full px-2 py-0.5 text-xs font-medium",
                      statusColor(job.status),
                    ].join(" ")}
                  >
                    {job.status}
                  </span>
                </td>
                <td className="px-3 py-2 text-right font-[var(--font-display)] text-[var(--ink-900)]">
                  {job.total_rows ?? "—"}
                </td>
                <td className="px-3 py-2 text-right font-[var(--font-display)] text-[var(--moss-700)]">
                  {job.success_count}
                </td>
                <td className="px-3 py-2 text-right font-[var(--font-display)] text-[var(--danger,#8B1A1A)]">
                  {job.error_count}
                </td>
                <td className="px-3 py-2">
                  <Link
                    href={`/products/import/${job.id}`}
                    className="inline-flex items-center gap-1 text-[var(--moss-700)] hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--moss-700)]"
                    aria-label={`View import ${extractFilename(job.gcs_path)}`}
                  >
                    <Eye className="h-3.5 w-3.5" aria-hidden="true" />
                    View
                  </Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
