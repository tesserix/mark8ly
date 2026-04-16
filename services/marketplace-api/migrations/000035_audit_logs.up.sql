-- Audit log of administrative actions inside marketplace-api.
-- Every row is scoped to (tenant_id, store_id) so a tenant with multiple
-- stores has fully isolated audit trails per store.
--
-- Writers: internal/audit.Emitter (async, fire-and-forget).
-- Readers: GET /api/v1/admin/stores/:storeId/audit-logs (admin UI Settings -> Audit Logs).
--
-- The shape mirrors apps/admin/lib/api/settings-tier2-api.ts AuditLogEntry so
-- the handler can serialize directly without translation.
CREATE TABLE IF NOT EXISTS audit_logs (
  id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       UUID         NOT NULL,
  store_id        UUID         NOT NULL,

  actor_user_id   UUID,                                              -- nullable: system/internal events
  actor_email     TEXT,                                              -- nullable until BFF propagates X-User-Email
  actor_type      VARCHAR(16)  NOT NULL DEFAULT 'user',              -- user | system | api

  action          VARCHAR(64)  NOT NULL,                             -- "order.cancelled", "product.updated", ...
  resource_type   VARCHAR(32)  NOT NULL,                             -- "order" | "product" | "domain" | ...
  resource_id     TEXT,                                              -- string (orders use store-prefixed numbers)

  status          VARCHAR(16)  NOT NULL DEFAULT 'success',           -- success | failure
  severity        VARCHAR(16)  NOT NULL DEFAULT 'info',              -- info | warning | critical

  ip_address      INET,
  user_agent      TEXT,

  metadata        JSONB        NOT NULL DEFAULT '{}'::jsonb,

  created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

  CONSTRAINT audit_logs_actor_type_chk CHECK (actor_type IN ('user', 'system', 'api')),
  CONSTRAINT audit_logs_status_chk     CHECK (status IN ('success', 'failure')),
  CONSTRAINT audit_logs_severity_chk   CHECK (severity IN ('info', 'warning', 'critical'))
);

-- Hot path: the admin page lists newest-first within a store.
CREATE INDEX IF NOT EXISTS idx_audit_logs_store_created
  ON audit_logs (tenant_id, store_id, created_at DESC);

-- Resource lookup: "show me everything that happened to order X".
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource
  ON audit_logs (tenant_id, store_id, resource_type, resource_id, created_at DESC);

-- Filter: action / severity facets in the UI.
CREATE INDEX IF NOT EXISTS idx_audit_logs_action
  ON audit_logs (tenant_id, store_id, action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_severity
  ON audit_logs (tenant_id, store_id, severity, created_at DESC);
