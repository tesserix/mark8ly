import { z } from "zod";
import { paginated } from "../schema-helpers";

/**
 * An audit-log entry — audit.Response (internal/audit/models.go:104). All
 * fields are non-pointer strings (present, possibly ""); `metadata` is a
 * `map[string]any` that marshals to an object or JSON `null` when nil.
 */
export const auditLogEntrySchema = z.object({
  id: z.string(),
  timestamp: z.string(),
  user_email: z.string(),
  // "user" | "system" | "api"
  actor_type: z.string(),
  action: z.string(),
  resource_type: z.string(),
  resource_id: z.string(),
  status: z.string(),
  severity: z.string(),
  ip_address: z.string(),
  user_agent: z.string(),
  metadata: z.record(z.string(), z.unknown()).nullable(),
});
export type AuditLogEntry = z.infer<typeof auditLogEntrySchema>;

/** LIST envelope: standard `{data, meta}` (audit_logs.go:63). */
export const auditLogListSchema = paginated(auditLogEntrySchema);
export type AuditLogListResponse = z.infer<typeof auditLogListSchema>;
