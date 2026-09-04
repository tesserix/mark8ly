/**
 * platform-api client used by the admin server components. Only the
 * endpoints admin actually needs — nothing speculative. Right now
 * that's just the tenant lookup used by the dashboard.
 */

import type { StorefrontTheme } from "@repo/ui/storefront-theme";

import { platformInternalHeaders } from "@/lib/api/server/platformInternal";

const PLATFORM_API_URL =
  process.env.PLATFORM_API_URL ?? "http://localhost:8086";

// Server-side calls into /internal/* on platform-api carry the shared
// X-Internal-Auth secret so platform-api's RequireInternalAuth gate accepts
// them. /api/v1/* routes are public (slug-available, accept-invite token
// flows) and don't need the header — keep using a plain fetch there.
const internalInit = (extra: RequestInit = {}): RequestInit => ({
  cache: "no-store",
  ...extra,
  headers: platformInternalHeaders(
    Object.fromEntries(
      Object.entries((extra.headers as Record<string, string>) ?? {}),
    ),
  ),
});

/**
 * Tenant = the company that owns one or more stores. Phase Q moved
 * slug/currency/timezone/country to the Store interface; a tenant
 * keeps only company-level fields (name, owner, status).
 */
export interface Tenant {
  id: string;
  name: string;
  owner_user_id: string;
  owner_email: string;
  status: string;
  created_at: string;
  updated_at: string;
}

/**
 * Store = a single storefront under a tenant. Carries the public
 * URL identity and locale settings. Phase Q introduces this as a
 * first-class admin concept; Phase R adds store-level role grants.
 */
export interface Store {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  country_code: string;
  currency_code: string;
  timezone: string;
  logo_url?: string | null;
  storefront_theme: StorefrontTheme;
  status: string;
  created_at: string;
  updated_at: string;
}

interface TenantResponse {
  data: Tenant;
}

/**
 * Fetches a tenant row by ID. Hits the internal route at
 * /internal/tenants/:id which platform-api exposes for auth-bff + admin
 * use. Returns null on 404 or network failure so callers can branch
 * cleanly on "not found" without try/catching around every call site.
 */
export async function fetchTenant(id: string): Promise<Tenant | null> {
  if (!id) return null;
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/tenants/${id}`,
      internalInit(),
    );
    if (res.status === 404) return null;
    if (!res.ok) return null;
    const body = (await res.json()) as TenantResponse;
    return body.data;
  } catch {
    return null;
  }
}

/**
 * Lists all stores under the given tenant. Used by the admin server
 * session helper to resolve the current store (default = first).
 */
export async function listStoresByTenant(
  tenantId: string,
): Promise<Store[]> {
  if (!tenantId) return [];
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/tenants/${tenantId}/stores`,
      internalInit(),
    );
    if (!res.ok) return [];
    const body = (await res.json()) as { data: Store[] };
    return body.data ?? [];
  } catch {
    return [];
  }
}

/**
 * Fetches a single store by id. Used by the settings page when the
 * caller already knows which store is current.
 */
export async function fetchStore(id: string): Promise<Store | null> {
  if (!id) return null;
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/stores/${id}`,
      internalInit(),
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { data: Store };
    return body.data;
  } catch {
    return null;
  }
}

/**
 * Creates a new store under the given tenant. Phase Q.2 adds this
 * for the "add a second store" flow. FGA-gated on the tenant's
 * `can_manage_stores` permission (owner or admin).
 */
export async function createStore(
  tenantId: string,
  payload: {
    uid: string;
    slug: string;
    name: string;
    country_code: string;
    currency_code: string;
    timezone: string;
  },
): Promise<Store> {
  const res = await fetch(
    `${PLATFORM_API_URL}/internal/tenants/${tenantId}/stores`,
    internalInit({
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    }),
  );
  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new PlatformApiError(
      res.status,
      body.error ?? "platform_api_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const body = (await res.json()) as { data: Store };
  return body.data;
}

/**
 * Patches the editable subset of a store row. Phase Q ships with
 * only `name` editable — currency / timezone / country edits need
 * their own slice (billing / picker / tax concerns).
 */
export async function updateStore(
  id: string,
  patch: {
    name?: string;
    timezone?: string;
    storefront_theme?: StorefrontTheme;
    uid: string;
  },
): Promise<Store> {
  const res = await fetch(
    `${PLATFORM_API_URL}/internal/stores/${id}`,
    internalInit({
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(patch),
    }),
  );
  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new PlatformApiError(
      res.status,
      body.error ?? "platform_api_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const body = (await res.json()) as { data: Store };
  return body.data;
}

/**
 * Tenant role string. Mirrors authz.Role in services/platform-api —
 * keep the unions in sync when adding a new role. The empty string
 * sentinel is only used by `fetchTenantMe`'s null-on-404 fallback
 * and should never leak into UI-facing code.
 */
export type TenantRole = "owner" | "admin" | "staff" | "viewer";

/**
 * A single entry in the caller's tenant list. Returned by
 * `/api/v1/users/me/tenants`. Used by the admin sign-in flow
 * (to pick a default tenant or redirect to /pick-tenant) and by
 * the top-bar tenant switcher.
 */
export interface Membership {
  tenant_id: string;
  name: string;
  role: TenantRole;
}

/**
 * Lists every tenant the user has any role on. Throws
 * PlatformApiError on non-2xx so the caller can distinguish
 * "empty tenant list" (return value []) from "API down" (throw).
 */
export async function listMemberTenants(uid: string): Promise<Membership[]> {
  const res = await fetch(
    `${PLATFORM_API_URL}/api/v1/users/me/tenants?uid=${encodeURIComponent(uid)}`,
    { cache: "no-store" },
  );
  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new PlatformApiError(
      res.status,
      body.error ?? "platform_api_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const body = (await res.json()) as { data: Membership[] };
  return body.data ?? [];
}

interface TenantMeResponse {
  data: { role: TenantRole };
}

/**
 * Fetches the caller's role on a tenant. Used by admin middleware to
 * forward the role into every server-rendered page via the
 * `x-session-role` header.
 *
 * Returns null if the caller has no role (platform-api 404) or if
 * the call fails entirely — the caller (middleware) treats null as
 * "sign the user out" since admin pages without a role are
 * meaningless.
 */
export async function fetchTenantMe(
  tenantId: string,
  uid: string,
): Promise<TenantRole | null> {
  if (!tenantId || !uid) return null;
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/tenants/${tenantId}/me?uid=${encodeURIComponent(uid)}`,
      internalInit(),
    );
    if (!res.ok) return null;
    const body = (await res.json()) as TenantMeResponse;
    return body.data.role;
  } catch {
    return null;
  }
}

// ── Invitations (Phase P) ─────────────────────────────────────────

export interface Invitation {
  id: string;
  tenant_id: string;
  store_id?: string | null;
  email: string;
  role: InviteRole;
  status: "pending" | "accepted" | "expired" | "revoked";
  expires_at: string;
  invited_by_user_id: string;
  created_at: string;
  accepted_at?: string | null;
}

export interface InvitationVerifyResult {
  invitation_id: string;
  tenant_id: string;
  tenant_name: string;
  tenant_slug: string;
  email: string;
  role: TenantRole;
  expires_at: string;
}

/**
 * Creates a pending invitation on the given tenant. Phase P: the
 * admin BFF is the only caller, forwarding the inviting user's uid
 * from the validated session cookie. platform-api enforces the
 * `can_invite_members` FGA check on top.
 */
/**
 * Phase R: roles accepted by invitation. Tenant-wide invites use
 * admin/staff/viewer (mirrors Phase P). Store-scoped invites use
 * manager/staff/viewer (mirrors the store DSL relations).
 */
export type InviteRole = "admin" | "manager" | "staff" | "viewer";

export async function createInvitation(
  tenantId: string,
  payload: {
    uid: string;
    email: string;
    role: InviteRole;
    store_id?: string;
  },
): Promise<Invitation> {
  const res = await fetch(
    `${PLATFORM_API_URL}/internal/tenants/${tenantId}/invitations`,
    internalInit({
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    }),
  );
  if (!res.ok) {
    throw await platformError(res);
  }
  const body = (await res.json()) as { data: Invitation };
  return body.data;
}

/**
 * Lists pending invitations for the tenant. Used by the
 * /settings/team page.
 */
export async function listPendingInvitations(
  tenantId: string,
): Promise<Invitation[]> {
  const res = await fetch(
    `${PLATFORM_API_URL}/internal/tenants/${tenantId}/invitations`,
    internalInit(),
  );
  if (!res.ok) {
    throw await platformError(res);
  }
  const body = (await res.json()) as { data: Invitation[] };
  return body.data ?? [];
}

/**
 * A member on the team listing: the tenant owner plus any teammate
 * who has already accepted their invite. Source of truth is the
 * platform-api /internal/tenants/{id}/members endpoint, which joins
 * the tenants row (owner) with accepted invitations rows.
 */
export interface TeamMember {
  email: string;
  role: string;
  kind: "owner" | "invited";
  accepted_at?: string;
}

/**
 * Lists the current team for the tenant — owner row first, then
 * every accepted invitation. Used by /settings/team.
 */
export async function listTeamMembers(
  tenantId: string,
): Promise<TeamMember[]> {
  const res = await fetch(
    `${PLATFORM_API_URL}/internal/tenants/${tenantId}/members`,
    internalInit(),
  );
  if (!res.ok) {
    throw await platformError(res);
  }
  const body = (await res.json()) as { data: TeamMember[] };
  return body.data ?? [];
}

/**
 * Changes an already-accepted teammate's tenant-level role. The
 * target is identified by email so the admin UI doesn't need to know
 * GIP UIDs; the platform-api resolves email → UID via the accepted
 * invitation row. Actor's UID is required for the server-side authz
 * guard (owner/admin only, cannot demote self, admin cannot touch
 * other admins or the owner).
 */
export async function updateMemberRole(
  tenantId: string,
  payload: {
    email: string;
    new_role: string;
    uid: string;
  },
): Promise<{ email: string; role: string }> {
  const res = await fetch(
    `${PLATFORM_API_URL}/internal/tenants/${tenantId}/members/role`,
    internalInit({
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    }),
  );
  if (!res.ok) {
    throw await platformError(res);
  }
  const body = (await res.json()) as {
    data: { email: string; role: string };
  };
  return body.data;
}

/**
 * Revokes a pending invitation. Requires the inviting user's uid
 * for the FGA re-check on the server side.
 */
export async function revokeInvitation(
  tenantId: string,
  invitationId: string,
  uid: string,
): Promise<void> {
  const res = await fetch(
    `${PLATFORM_API_URL}/internal/tenants/${tenantId}/invitations/${invitationId}`,
    internalInit({
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ uid }),
    }),
  );
  if (!res.ok) {
    throw await platformError(res);
  }
}

/**
 * Deletes a tenant's account. Mirrors the mobile app's account-deletion
 * flow. Requires the requesting user's uid for the server-side authz
 * guard (owner only).
 */
export async function deleteTenantAccount(
  tenantId: string,
  uid: string,
): Promise<void> {
  const res = await fetch(
    `${PLATFORM_API_URL}/internal/tenants/${tenantId}/account`,
    internalInit({
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ uid }),
    }),
  );
  if (!res.ok) {
    throw await platformError(res);
  }
}

/**
 * Verifies a raw invitation token and returns the public summary
 * for the accept page. Does NOT require an authenticated caller.
 */
export async function verifyInvitationToken(
  token: string,
): Promise<InvitationVerifyResult> {
  const res = await fetch(
    `${PLATFORM_API_URL}/api/v1/invitations/verify?token=${encodeURIComponent(token)}`,
    { cache: "no-store" },
  );
  if (!res.ok) {
    throw await platformError(res);
  }
  const body = (await res.json()) as { data: InvitationVerifyResult };
  return body.data;
}

/**
 * Accepts an invitation by writing the role tuple to FGA.
 *
 * GIP path: the caller passes the verified GIP uid + email from the
 * client sign-in flow; platform-api re-checks the email against the
 * invitation row and rejects on mismatch.
 *
 * Zitadel path: there is no `uid` to send — the invitee has no provider
 * account yet, and creating it is what accept now does (see
 * services/platform-api/internal/invitation/service.go's AcceptInput).
 * The caller sends `password` instead, and platform-api provisions the
 * Zitadel user, the admin project grant, and BOTH FGA tuples
 * (email-keyed and Zitadel-uid-keyed) before marking the invitation
 * accepted. `verified_email` is required on both paths and is still
 * strictly matched against the invitation.
 *
 * `uid` stays required-shaped for the GIP caller by convention rather
 * than by types: JSON.stringify drops undefined keys, so a GIP caller
 * passing exactly {token, uid, verified_email} still emits a
 * byte-identical body to the one this function has always sent.
 */
export async function acceptInvitation(payload: {
  token: string;
  uid?: string;
  verified_email: string;
  password?: string;
  first_name?: string;
  last_name?: string;
}): Promise<{ tenant_id: string; role: TenantRole }> {
  const res = await fetch(`${PLATFORM_API_URL}/api/v1/invitations/accept`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    cache: "no-store",
  });
  if (!res.ok) {
    throw await platformError(res);
  }
  const body = (await res.json()) as {
    data: { tenant_id: string; role: TenantRole };
  };
  return body.data;
}

async function platformError(res: Response): Promise<PlatformApiError> {
  let body: { error?: string; message?: string } = {};
  try {
    body = await res.json();
  } catch {
    // ignore
  }
  return new PlatformApiError(
    res.status,
    body.error ?? "platform_api_error",
    body.message ?? `HTTP ${res.status}`,
  );
}

export class PlatformApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

interface TenantSummary {
  id: string;
  slug: string;
  name: string;
  owner_user_id: string;
  owner_email: string;
}

/**
 * Looks up a workspace tenant by GIP UID. Used by the /login server
 * action to bridge a freshly minted GIP id_token to the workspace_tenant
 * field auth-bff /auth/auto-login requires.
 *
 * Throws on any non-2xx so the caller can map the error code to a
 * user-friendly message ("no store found for this account" on 404, etc).
 */
export async function getTenantByOwner(uid: string): Promise<TenantSummary> {
  const res = await fetch(
    `${PLATFORM_API_URL}/api/v1/tenants/by-owner?uid=${encodeURIComponent(uid)}`,
    { cache: "no-store" },
  );
  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new PlatformApiError(
      res.status,
      body.error ?? "platform_api_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const body = (await res.json()) as { data: TenantSummary };
  return body.data;
}

// ─────────────────────────────────────────────────────────────────────
// Password reset — branded flow owned by platform-api. Replaces the
// direct GIP sendOobCode call so merchants never see the Firebase
// default email or the auth action page URL.
// ─────────────────────────────────────────────────────────────────────

export type RequestPasswordResetResult =
  | { ok: true }
  | {
      ok: false;
      code: "too_many_attempts" | "upstream_unavailable" | "unknown";
      message: string;
    };

export async function requestPasswordReset(
  email: string,
): Promise<RequestPasswordResetResult> {
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/auth/password-reset/request`,
      internalInit({
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email }),
      }),
    );
    if (res.status === 204) return { ok: true };

    if (res.status === 429) {
      return {
        ok: false,
        code: "too_many_attempts",
        message:
          "Too many requests. Please wait a moment and try again.",
      };
    }
    if (res.status === 502 || res.status === 503) {
      return {
        ok: false,
        code: "upstream_unavailable",
        message:
          "We couldn't reach the identity service. Please try again in a moment.",
      };
    }
    return {
      ok: false,
      code: "unknown",
      message:
        "We couldn't send the reset email. Please try again in a moment.",
    };
  } catch {
    return {
      ok: false,
      code: "upstream_unavailable",
      message:
        "We couldn't reach the identity service. Please try again in a moment.",
    };
  }
}

export type ConfirmPasswordResetResult =
  | { ok: true }
  | {
      ok: false;
      code:
        | "invalid_oob_code"
        | "weak_password"
        | "upstream_unavailable"
        | "invalid_request"
        | "unknown";
      message: string;
    };

export async function confirmPasswordReset(
  oobCode: string,
  newPassword: string,
): Promise<ConfirmPasswordResetResult> {
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/auth/password-reset/confirm`,
      internalInit({
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ oob_code: oobCode, new_password: newPassword }),
      }),
    );
    if (res.status === 204) return { ok: true };

    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore — fall through to status-based mapping
    }
    const message =
      body.message ?? "Something went wrong resetting your password.";

    switch (res.status) {
      case 410:
        return { ok: false, code: "invalid_oob_code", message };
      case 400:
        if (body.error === "weak_password") {
          return { ok: false, code: "weak_password", message };
        }
        return { ok: false, code: "invalid_request", message };
      case 502:
      case 503:
        return { ok: false, code: "upstream_unavailable", message };
      default:
        return { ok: false, code: "unknown", message };
    }
  } catch {
    return {
      ok: false,
      code: "upstream_unavailable",
      message:
        "We couldn't reach the identity service. Please try again in a moment.",
    };
  }
}

/* ============================================================
   Locations + slug availability
   ------------------------------------------------------------
   Shared shape with onboarding's platform-api client so the Add
   Store form and the editable timezone picker in General settings
   can reuse the IANA-seeded /api/v1/locations/* endpoints.
   ============================================================ */

export interface Country {
  code: string;
  name: string;
  native_name: string;
  calling_code: string;
  currency_code: string;
  flag_emoji: string;
  region: string;
}

export interface State {
  id: string;
  country_code: string;
  code: string;
  name: string;
  type: string;
}

export interface Currency {
  code: string;
  name: string;
  symbol: string;
  decimal_places: number;
}

export interface Timezone {
  id: string;
  name: string;
  utc_offset: string;
}

export async function listCountries(): Promise<Country[]> {
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/api/v1/locations/countries`,
      { cache: "no-store" },
    );
    if (!res.ok) return [];
    const body = (await res.json()) as { data: Country[] };
    return body.data ?? [];
  } catch {
    return [];
  }
}

export async function listStatesByCountry(code: string): Promise<State[]> {
  if (!code) return [];
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/api/v1/locations/countries/${encodeURIComponent(code)}/states`,
      { cache: "no-store" },
    );
    if (!res.ok) return [];
    const body = (await res.json()) as { data: State[] };
    return body.data ?? [];
  } catch {
    return [];
  }
}

export async function listCurrencies(): Promise<Currency[]> {
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/api/v1/locations/currencies`,
      { cache: "no-store" },
    );
    if (!res.ok) return [];
    const body = (await res.json()) as { data: Currency[] };
    return body.data ?? [];
  } catch {
    return [];
  }
}

export async function listTimezones(): Promise<Timezone[]> {
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/api/v1/locations/timezones`,
      { cache: "no-store" },
    );
    if (!res.ok) return [];
    const body = (await res.json()) as { data: Timezone[] };
    return body.data ?? [];
  } catch {
    return [];
  }
}

export async function isStoreSlugAvailable(
  slug: string,
): Promise<{ slug: string; available: boolean }> {
  const res = await fetch(
    `${PLATFORM_API_URL}/api/v1/stores/slug-available?slug=${encodeURIComponent(slug)}`,
    { cache: "no-store" },
  );
  if (!res.ok) {
    throw await platformError(res);
  }
  const body = (await res.json()) as {
    data: { slug: string; available: boolean };
  };
  return body.data;
}
