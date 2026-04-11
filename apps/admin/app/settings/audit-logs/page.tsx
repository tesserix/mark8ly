import { Suspense } from "react";
import { AdminShell } from "@/components/shell/AdminShell";
import { AdminPage } from "@/components/layout";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listAuditLogs } from "@/lib/api/settings-tier2-api";
import type { AuditLogsQuery } from "@/lib/api/settings-tier2-api";
import { AuditLogsClient } from "@/components/settings/AuditLogsClient";

/**
 * /settings/audit-logs — searchable audit trail.
 *
 * Server component reads URL search params, fetches the matching
 * page of audit logs, and passes data to the interactive client
 * component for filtering, expanding rows, and CSV export.
 */
export default async function AuditLogsPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = await getServerSessionContext();

  const params = await searchParams;

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <AdminPage
        eyebrow="Team & access"
        title="Audit logs"
        description="Review a detailed log of all actions taken in your store. Filter by user, action, severity, or date range."
      >
        {currentStore ? (
          <Suspense
            fallback={
              <p className="text-sm text-foreground-secondary">
                Loading audit logs…
              </p>
            }
          >
            <AuditLogsContent
              storeId={currentStore.id}
              userId={userId}
              tenantId={tenantId}
              searchParams={params}
            />
          </Suspense>
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store to view audit logs.
          </p>
        )}
      </AdminPage>
    </AdminShell>
  );
}

async function AuditLogsContent({
  storeId,
  userId,
  tenantId,
  searchParams,
}: {
  storeId: string;
  userId: string;
  tenantId: string;
  searchParams: Record<string, string | string[] | undefined>;
}) {
  const param = (key: string) => {
    const v = searchParams[key];
    return typeof v === "string" ? v : undefined;
  };

  const query: AuditLogsQuery = {
    search: param("search"),
    user: param("user"),
    action: param("action"),
    resource_type: param("resource_type"),
    severity: param("severity"),
    date_from: param("date_from"),
    date_to: param("date_to"),
    page: param("page") ? Number(param("page")) : undefined,
    pageSize: 25,
  };

  const data = await listAuditLogs(storeId, query, { userId, tenantId });

  return <AuditLogsClient initialData={data} />;
}
