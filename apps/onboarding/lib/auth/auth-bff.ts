// Server-side typed client for auth-bff.
//
// Used from server actions only — never imported by client components.
// The auto-login route mints a session cookie which we forward back to
// the user's browser via Next's response headers.

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
  /** The Set-Cookie header value from auth-bff. The caller is responsible
   *  for forwarding it to the browser response. */
  setCookie: string;
}

export async function autoLogin(req: AutoLoginRequest): Promise<AutoLoginResult> {
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

  // The session cookie is in the Set-Cookie response header. We grab it
  // here and the server action forwards it on the Next.js response.
  const setCookie = res.headers.get("set-cookie") ?? "";

  return {
    uid: body.data.uid,
    email: body.data.email,
    tenant_id: body.data.tenant_id,
    setCookie,
  };
}
