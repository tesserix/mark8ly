import type { createApiClient } from "./client";
import { auditLogListSchema, type AuditLogListResponse } from "./schemas/audit-log";

export interface ListAuditLogsParams {
  action?: string;
  resource_type?: string;
  status?: string;
  severity?: string;
  page?: string;
  page_size?: string;
}

/**
 * Read-only audit log. Mirrors web routes.go:774-787 (list only; CSV export
 * is a web-only file download). Standard `{data, meta}` envelope.
 */
export function createAuditLogApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListAuditLogsParams) =>
      client.get<AuditLogListResponse>(
        "/audit-logs",
        params as Record<string, string>,
        auditLogListSchema,
      ),
  };
}

export type { AuditLogListResponse };
