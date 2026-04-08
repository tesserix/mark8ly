// Server-side typed client for auth-bff. Used from server actions only.
//
// /auth/auto-login mints a session cookie which the caller is responsible
// for forwarding to the browser via Next's response headers.

import { config } from "@/lib/config";

const base = config.authBffUrl;

export class AuthBffError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

interface AutoLoginRequest {
  idToken: string;
  expectedTenantId: string;
  workspaceTenant: string;
}

interface AutoLoginResult {
  uid: string;
  email: string;
  tenant_id: string;
  /** Set-Cookie header value from auth-bff. The caller forwards this to
   *  the browser response. */
  setCookie: string;
}

interface SwitchTenantResult {
  /** Set-Cookie header value from auth-bff. The caller forwards this
   *  to the browser response. */
  setCookie: string;
}

/**
 * Switches the current session's tenant id. Phase P: called from
 * the tenant switcher dropdown and from the accept-invite server
 * action after a successful role grant.
 *
 * The caller MUST forward its own session cookies via cookieHeader
 * so auth-bff can read the existing session and verify membership
 * against the target tenant.
 */
/**
 * Switches the current session's store id under the same tenant.
 * Phase Q.2 — used by the store switcher and by the "Add store"
 * flow after creating a new store so the user lands in the newly
 * created store immediately.
 */
export async function switchStore(
  storeId: string,
  cookieHeader: string,
): Promise<SwitchTenantResult> {
  const res = await fetch(`${base}/auth/switch-store`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Cookie: cookieHeader,
    },
    body: JSON.stringify({ store_id: storeId }),
    cache: "no-store",
  });

  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? "auth_bff_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const setCookie = res.headers.get("set-cookie") ?? "";
  return { setCookie };
}

export async function switchTenant(
  tenantId: string,
  cookieHeader: string,
): Promise<SwitchTenantResult> {
  const res = await fetch(`${base}/auth/switch-tenant`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Cookie: cookieHeader,
    },
    body: JSON.stringify({ tenant_id: tenantId }),
    cache: "no-store",
  });

  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? "auth_bff_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const setCookie = res.headers.get("set-cookie") ?? "";
  return { setCookie };
}

export async function autoLogin(
  req: AutoLoginRequest,
): Promise<AutoLoginResult> {
  const res = await fetch(`${base}/auth/auto-login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      id_token: req.idToken,
      expected_tenant_id: req.expectedTenantId,
      workspace_tenant: req.workspaceTenant,
    }),
    cache: "no-store",
  });

  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new AuthBffError(
      res.status,
      body.error ?? "auth_bff_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }

  const body = (await res.json()) as {
    data: { uid: string; email: string; tenant_id: string };
  };
  const setCookie = res.headers.get("set-cookie") ?? "";

  return {
    uid: body.data.uid,
    email: body.data.email,
    tenant_id: body.data.tenant_id,
    setCookie,
  };
}
