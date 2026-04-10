"use client";

import { useState, useTransition, useCallback, useRef } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { formatDistanceToNow, format } from "date-fns";
import {
  Search,
  Download,
  ChevronDown,
  ChevronRight,
  Filter,
} from "lucide-react";

import type {
  AuditLogEntry,
  AuditLogsResponse,
  AuditLogsQuery,
} from "@/lib/api/settings-tier2-api";
import { exportAuditCSV } from "@/app/settings/actions";

interface AuditLogsClientProps {
  initialData: AuditLogsResponse | null;
}

const SEVERITY_STYLES: Record<string, string> = {
  info: "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60",
  warning: "bg-[color:var(--warning)]/10 text-[color:var(--warning)]",
  error: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
  critical: "bg-[color:var(--danger)]/10 text-[color:var(--danger)]",
};

export function AuditLogsClient({ initialData }: AuditLogsClientProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [isExporting, startExport] = useTransition();

  // Filter state from URL params
  const currentSearch = searchParams.get("search") ?? "";
  const currentUser = searchParams.get("user") ?? "";
  const currentAction = searchParams.get("action") ?? "";
  const currentResourceType = searchParams.get("resource_type") ?? "";
  const currentSeverity = searchParams.get("severity") ?? "";
  const currentDateFrom = searchParams.get("date_from") ?? "";
  const currentDateTo = searchParams.get("date_to") ?? "";
  const currentPage = Number(searchParams.get("page") ?? "1");

  const updateFilters = useCallback(
    (updates: Record<string, string>) => {
      const params = new URLSearchParams(searchParams.toString());
      for (const [key, value] of Object.entries(updates)) {
        if (value) {
          params.set(key, value);
        } else {
          params.delete(key);
        }
      }
      // Reset to page 1 when filters change
      if (!("page" in updates)) {
        params.delete("page");
      }
      router.push(`/settings/audit-logs?${params.toString()}`);
    },
    [router, searchParams],
  );

  function handleExportCSV() {
    startExport(async () => {
      const query: AuditLogsQuery = {
        search: currentSearch || undefined,
        user: currentUser || undefined,
        action: currentAction || undefined,
        resource_type: currentResourceType || undefined,
        severity: currentSeverity || undefined,
        date_from: currentDateFrom || undefined,
        date_to: currentDateTo || undefined,
      };
      const result = await exportAuditCSV(query);
      if (result.ok) {
        const blob = new Blob([result.data], { type: "text/csv" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = `audit-logs-${format(new Date(), "yyyy-MM-dd")}.csv`;
        a.click();
        URL.revokeObjectURL(url);
      }
    });
  }

  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const entries = initialData?.data ?? [];
  const meta = initialData?.meta ?? { page: 1, page_size: 25, total: 0, total_pages: 1 };

  return (
    <div className="space-y-6">
      {/* Search + Export Row */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative flex-1 max-w-md">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-foreground-tertiary" aria-hidden="true" />
          <input
            type="text"
            defaultValue={currentSearch}
            onChange={(e) => {
              const value = e.target.value;
              clearTimeout(searchTimeoutRef.current);
              searchTimeoutRef.current = setTimeout(() => updateFilters({ search: value }), 300);
            }}
            className="h-10 w-full rounded-[6px] border border-border bg-[color:var(--background-elevated)] pl-9 pr-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
            placeholder="Search audit logs..."
          />
        </div>
        <button
          type="button"
          onClick={handleExportCSV}
          disabled={isExporting}
          className="inline-flex h-10 items-center gap-2 rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-4 text-sm font-medium text-foreground transition-colors hover:bg-[color:var(--paper-200)] disabled:opacity-50"
        >
          <Download className="h-4 w-4" aria-hidden="true" />
          {isExporting ? "Exporting..." : "Export CSV"}
        </button>
      </div>

      {/* Filter Row */}
      <div className="flex flex-wrap items-center gap-3">
        <Filter className="h-4 w-4 text-foreground-secondary" aria-hidden="true" />
        <FilterInput
          label="User"
          value={currentUser}
          placeholder="user@example.com"
          onChange={(v) => updateFilters({ user: v })}
        />
        <FilterInput
          label="Action"
          value={currentAction}
          placeholder="e.g. create, update"
          onChange={(v) => updateFilters({ action: v })}
        />
        <FilterInput
          label="Resource type"
          value={currentResourceType}
          placeholder="e.g. product, order"
          onChange={(v) => updateFilters({ resource_type: v })}
        />
        <FilterSelect
          label="Severity"
          value={currentSeverity}
          options={["", "info", "warning", "error", "critical"]}
          onChange={(v) => updateFilters({ severity: v })}
        />
        <FilterInput
          label="From"
          value={currentDateFrom}
          type="date"
          ariaLabel="Filter from date"
          onChange={(v) => updateFilters({ date_from: v })}
        />
        <FilterInput
          label="To"
          value={currentDateTo}
          type="date"
          ariaLabel="Filter to date"
          onChange={(v) => updateFilters({ date_to: v })}
        />
      </div>

      {/* Table */}
      {entries.length === 0 ? (
        <div className="py-10 text-center">
          <p className="text-sm text-[color:var(--ink-900)]/50">
            No audit log entries found matching your filters.
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left">
                <th className="pb-3 pr-2 w-6" />
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">Timestamp</th>
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">User</th>
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">Action</th>
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">Resource</th>
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">Status</th>
                <th className="pb-3 font-medium text-foreground-secondary">Severity</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <AuditLogRow
                  key={entry.id}
                  entry={entry}
                  expanded={expandedId === entry.id}
                  onToggle={() =>
                    setExpandedId(expandedId === entry.id ? null : entry.id)
                  }
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Pagination */}
      {meta.total_pages > 1 && (
        <div className="flex items-center justify-between pt-2">
          <p className="text-xs text-foreground-secondary">
            Page {meta.page} of {meta.total_pages} ({meta.total} entries)
          </p>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={meta.page <= 1}
              onClick={() => updateFilters({ page: String(meta.page - 1) })}
              className="h-8 rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-3 text-xs font-medium text-foreground transition-colors hover:bg-[color:var(--paper-200)] disabled:opacity-50"
            >
              Previous
            </button>
            <button
              type="button"
              disabled={meta.page >= meta.total_pages}
              onClick={() => updateFilters({ page: String(meta.page + 1) })}
              className="h-8 rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-3 text-xs font-medium text-foreground transition-colors hover:bg-[color:var(--paper-200)] disabled:opacity-50"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Audit Log Row ────────────────────────────────────────────────────

function AuditLogRow({
  entry,
  expanded,
  onToggle,
}: {
  entry: AuditLogEntry;
  expanded: boolean;
  onToggle: () => void;
}) {
  return (
    <>
      <tr
        className="border-b border-border cursor-pointer hover:bg-[color:var(--paper-200)]/50"
        onClick={onToggle}
        tabIndex={0}
        role="button"
        aria-expanded={expanded}
        onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onToggle(); } }}
      >
        <td className="py-3 pr-2">
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-foreground-secondary" aria-hidden="true" />
          ) : (
            <ChevronRight className="h-4 w-4 text-foreground-secondary" aria-hidden="true" />
          )}
        </td>
        <td className="py-3 pr-4 text-foreground-secondary whitespace-nowrap">
          {formatDistanceToNow(new Date(entry.timestamp), { addSuffix: true })}
        </td>
        <td className="py-3 pr-4 text-foreground">{entry.user_email}</td>
        <td className="py-3 pr-4 font-mono text-xs text-foreground">{entry.action}</td>
        <td className="py-3 pr-4 text-foreground-secondary">
          {entry.resource_type}
          {entry.resource_id && (
            <span className="ml-1 font-mono text-xs text-foreground-tertiary">
              #{entry.resource_id.slice(0, 8)}
            </span>
          )}
        </td>
        <td className="py-3 pr-4 text-foreground">{entry.status}</td>
        <td className="py-3">
          <span
            className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${
              SEVERITY_STYLES[entry.severity] ?? SEVERITY_STYLES.info
            }`}
          >
            {entry.severity}
          </span>
        </td>
      </tr>
      {expanded && (
        <tr className="border-b border-border bg-[color:var(--paper-200)]/30">
          <td colSpan={7} className="px-8 py-4">
            <div className="grid gap-3 text-xs sm:grid-cols-2">
              <div>
                <span className="font-medium text-foreground-secondary">IP address:</span>{" "}
                <span className="font-mono text-foreground">{entry.ip_address}</span>
              </div>
              <div>
                <span className="font-medium text-foreground-secondary">User agent:</span>{" "}
                <span className="text-foreground">{entry.user_agent}</span>
              </div>
              <div className="sm:col-span-2">
                <span className="font-medium text-foreground-secondary">Full timestamp:</span>{" "}
                <span className="text-foreground">{format(new Date(entry.timestamp), "PPpp")}</span>
              </div>
              {Object.keys(entry.metadata).length > 0 && (
                <div className="sm:col-span-2">
                  <span className="font-medium text-foreground-secondary">Metadata:</span>
                  <pre className="mt-1 overflow-x-auto rounded-[6px] bg-[color:var(--background-elevated)] p-3 text-xs text-foreground">
                    {JSON.stringify(entry.metadata, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  );
}

// ─── Filter Primitives ────────────────────────────────────────────────

function FilterInput({
  label,
  value,
  placeholder,
  type = "text",
  ariaLabel,
  onChange,
}: {
  label: string;
  value: string;
  placeholder?: string;
  type?: string;
  ariaLabel?: string;
  onChange: (value: string) => void;
}) {
  const inputId = `filter-${label.toLowerCase().replace(/\s+/g, "-")}`;
  const timeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  return (
    <div className="space-y-1">
      <label htmlFor={inputId} className="text-[10px] font-medium uppercase tracking-widest text-foreground-tertiary">
        {label}
      </label>
      <input
        id={inputId}
        type={type}
        defaultValue={value}
        placeholder={placeholder}
        aria-label={ariaLabel}
        onChange={(e) => {
          const v = e.target.value;
          clearTimeout(timeoutRef.current);
          timeoutRef.current = setTimeout(() => onChange(v), 300);
        }}
        className="h-8 w-full sm:w-36 rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-2 text-xs text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
      />
    </div>
  );
}

function FilterSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: string[];
  onChange: (value: string) => void;
}) {
  const selectId = `filter-${label.toLowerCase().replace(/\s+/g, "-")}`;
  return (
    <div className="space-y-1">
      <label htmlFor={selectId} className="text-[10px] font-medium uppercase tracking-widest text-foreground-tertiary">
        {label}
      </label>
      <select
        id={selectId}
        defaultValue={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-8 w-full sm:w-36 rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-2 text-xs text-foreground focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
      >
        <option value="">All</option>
        {options
          .filter(Boolean)
          .map((opt) => (
            <option key={opt} value={opt}>
              {opt}
            </option>
          ))}
      </select>
    </div>
  );
}
