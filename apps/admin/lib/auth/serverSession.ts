import { headers } from "next/headers";

import {
  fetchTenant,
  listMemberTenants,
  listStoresByTenant,
  type Membership,
  type Store,
  type Tenant,
  type TenantRole,
} from "@/lib/api/platform-api";
import { fetchSession } from "@/lib/auth/session";

/**
 * Helper used by every authenticated server component that needs to
 * render the admin chrome. Pulls the session headers that middleware
 * set, fetches the tenant, and returns the combined shape the shell
 * expects.
 *
 * Since middleware has already gated the request, the session headers
 * are guaranteed to be present by the time this runs. `fetchTenant`
 * may still return null if platform-api is unreachable — callers
 * should render gracefully in that case rather than crash.
 *
 * Phase O: `role` is populated from the `x-session-role` header that
 * middleware fetched from platform-api. Middleware fails the request
 * out to /login if the user has no role, so the role field is always
 * one of the four role strings here — never null.
 */
export interface ServerSessionContext {
  userId: string;
  email: string;
  tenantId: string;
  tenant: Tenant | null;
  tenantName: string;
  role: TenantRole;
  // Phase P: every tenant the user belongs to. An empty array or
  // one-element array means the switcher should stay hidden in the
  // shell. Fetched on every page render — there's no cache yet.
  memberships: Membership[];
  // Phase Q: every store under the current tenant, and the currently
  // active store. Phase Q derives "current store" as the first one
  // (by created_at) because there's no store switcher cookie yet;
  // when Phase Q.2 lands the switcher, `currentStore` will come from
  // the session cookie instead of this default.
  stores: Store[];
  currentStore: Store | null;
  storeName: string;
}

// Default role when the header is somehow missing (middleware regression,
// direct page hit without going through middleware in tests, etc).
// Choose the LEAST privileged role so UI accidentally rendered outside
// the middleware path doesn't silently reveal admin-only actions.
const DEFAULT_ROLE: TenantRole = "viewer";

function normalizeRole(raw: string | null): TenantRole {
  if (raw === "owner" || raw === "admin" || raw === "staff" || raw === "viewer") {
    return raw;
  }
  return DEFAULT_ROLE;
}

export async function getServerSessionContext(): Promise<ServerSessionContext> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const email = h.get("x-session-email") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = normalizeRole(h.get("x-session-role"));

  // Fetch tenant + memberships + stores in parallel. Everything
  // tolerates failure — empty results render a degraded shell
  // rather than crash.
  const [tenant, memberships, stores] = await Promise.all([
    fetchTenant(tenantId),
    userId ? listMemberTenants(userId).catch(() => []) : Promise.resolve([]),
    tenantId ? listStoresByTenant(tenantId) : Promise.resolve<Store[]>([]),
  ]);

  const tenantName = tenant?.name ?? "your store";
  // Phase Q.2: prefer the session-cookie store id (forwarded by
  // middleware as x-session-store-id). If the header is blank or
  // points at a store that no longer belongs to the current tenant
  // (e.g. after a switch-tenant), fall back to the first store.
  const sessionStoreId = h.get("x-session-store-id") ?? "";
  const fromSession = sessionStoreId
    ? stores.find((s) => s.id === sessionStoreId) ?? null
    : null;
  const currentStore = fromSession ?? stores[0] ?? null;
  const storeName = currentStore?.name ?? tenantName;

  return {
    userId,
    email,
    tenantId,
    tenant,
    tenantName,
    role,
    memberships,
    stores,
    currentStore,
    storeName,
  };
}

// ─── Store ID resolution ──────────────────────────────────────────
//
// Server actions receive x-session-store-id from middleware, but the
// header is often empty (session cookie doesn't include store_id).
// This helper resolves the store ID by falling back to the tenant's
// first store — same logic as getServerSessionContext.

export async function resolveStoreId(): Promise<string> {
  const h = await headers();
  const storeId = h.get("x-session-store-id") ?? "";
  if (storeId) return storeId;

  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!tenantId) return "";

  const stores = await listStoresByTenant(tenantId).catch(() => []);
  return stores[0]?.id ?? "";
}

// ─── Defensive auth resolver for route handlers ───────────────────
//
// Background: route handlers like /api/admin/.../invoice/email and
// proxyAdminApi 401 when the middleware-injected x-session-* headers
// are missing. In normal operation middleware always sets them, but
// we've seen surprise 401s in prod ("Couldn't send invoice — Session
// expired") that don't correspond to any /auth/session 401 in
// auth-bff logs. Most plausible causes: a Next.js 16 quirk in
// middleware-to-route-handler header forwarding, or a race between
// the auto-switch flow re-minting the cookie and an in-flight client
// fetch reading the old session.
//
// This helper is the bottom turtle: if middleware-injected headers are
// present, use them (the fast path); if not, validate the cookie
// directly against auth-bff one more time. The structured log emit
// makes it obvious from prod logs which path resolved.
//
// Not a substitute for the middleware path — middleware still does the
// role check, slug-to-tenant routing, etc. — but for routes where the
// only need is "who is this user", this fallback closes the
// surprise-401 gap.

export interface ResolvedAuth {
  userId: string;
  email: string;
  tenantId: string;
  source: "middleware-headers" | "auth-bff-fallback";
}

export async function resolveAuth(): Promise<ResolvedAuth | null> {
  const h = await headers();
  const userIdHdr = h.get("x-session-user-id") ?? "";
  const tenantIdHdr = h.get("x-session-tenant-id") ?? "";
  const emailHdr = h.get("x-session-email") ?? "";

  if (userIdHdr && tenantIdHdr) {
    return {
      userId: userIdHdr,
      email: emailHdr,
      tenantId: tenantIdHdr,
      source: "middleware-headers",
    };
  }

  // Headers missing — fall back to validating the cookie directly. The
  // browser's cookie reaches the route handler via the Cookie header
  // even when middleware-injected headers don't propagate.
  const cookie = h.get("cookie") ?? "";
  if (!cookie) return null;

  const session = await fetchSession(cookie);
  if (!session) return null;

  // eslint-disable-next-line no-console
  console.warn(
    "[serverSession] middleware headers missing; resolved via auth-bff fallback",
    { userId: session.userId, tenantId: session.tenantId },
  );

  return {
    userId: session.userId,
    email: session.email,
    tenantId: session.tenantId,
    source: "auth-bff-fallback",
  };
}

// ─── Permission helpers ────────────────────────────────────────────
//
// Single source of truth for client-visible "can this role do X?"
// checks. These MUST stay in lockstep with the DSL in
// infra/openfga/model.fga — UI gating is advisory; the backend
// PATCH /internal/tenants/:id runs the real FGA check. If you find
// yourself tempted to skip the backend check, don't: nav hiding is
// UX, not security.

export function canEditSettings(role: TenantRole): boolean {
  return role === "owner" || role === "admin";
}

// Phase P: mirrors the FGA can_invite_members permission. Same
// gate as can_edit_settings for now; may tighten to owner-only if
// a "seat count" paid feature ever lands.
export function canInviteMembers(role: TenantRole): boolean {
  return role === "owner" || role === "admin";
}

export function canViewSettings(role: TenantRole): boolean {
  return (
    role === "owner" ||
    role === "admin" ||
    role === "staff" ||
    role === "viewer"
  );
}
