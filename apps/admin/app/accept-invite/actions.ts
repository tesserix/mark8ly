"use server";

// Phase P — accept-invite server action.
//
// The client gets the user to a GIP id_token via one of three paths
// on the AcceptInviteForm:
//
//   a) "I already have a Mark8ly account" → signInWithPassword
//   b) "Continue with Google"              → signInWithGoogle
//   c) "Create a new account"              → signUp (new GIP user)
//
// Whichever path is picked hands us {idToken, uid, verifiedEmail}.
// We then:
//   1. Call platform-api accept — writes the FGA role tuple, marks
//      the invitation accepted, returns the tenant_id.
//   2. Auto-login against auth-bff so a session cookie exists.
//   3. Switch that session to the invitation's tenant.
//   4. Return the tenant_id + role so the client can redirect.
//
// Errors from any step map to a discriminated-union result that the
// client can surface inline.

import { cookies } from "next/headers";

import {
  acceptInvitation,
  PlatformApiError,
} from "@/lib/api/platform-api";
import { autoLogin, AuthBffError } from "@/lib/auth/auth-bff";
import { publicConfig } from "@/lib/config";

export interface AcceptInviteInput {
  token: string;
  idToken: string;
  uid: string;
  verifiedEmail: string;
}

export type AcceptInviteResult =
  | { ok: true; tenantId: string }
  | { ok: false; code: string; message: string };

function fail(err: unknown): {
  ok: false;
  code: string;
  message: string;
} {
  if (err instanceof PlatformApiError || err instanceof AuthBffError) {
    return { ok: false, code: err.code, message: err.message };
  }
  return {
    ok: false,
    code: "unknown",
    message: err instanceof Error ? err.message : String(err),
  };
}

export async function acceptInvite(
  input: AcceptInviteInput,
): Promise<AcceptInviteResult> {
  try {
    // 1. Write the FGA tuple + mark the invitation accepted. Returns
    // the tenant id we need for steps 2 and 3.
    const accepted = await acceptInvitation({
      token: input.token,
      uid: input.uid,
      verified_email: input.verifiedEmail,
    });

    // 2. Mint an initial session cookie on the invitee's NEW tenant.
    // If this is a brand-new GIP user, auto-login will retry against
    // FGA until the owner/member tuple propagates — but we just wrote
    // it synchronously in step 1, so it's already there.
    const login = await autoLogin({
      idToken: input.idToken,
      expectedTenantId: publicConfig.gipTenantId,
      workspaceTenant: accepted.tenant_id,
    });
    if (login.setCookie) {
      const parsed = parseSetCookie(login.setCookie);
      if (parsed) {
        const c = await cookies();
        c.set({
          name: parsed.name,
          value: parsed.value,
          path: parsed.path ?? "/",
          domain: parsed.domain,
          httpOnly: parsed.httpOnly,
          secure: parsed.secure,
          sameSite: "lax",
          maxAge: parsed.maxAge,
        });
      }
    }

    // 3. The auto-login already set the session to accepted.tenant_id
    // (we passed workspace_tenant). No need to call switch-tenant.
    return { ok: true, tenantId: accepted.tenant_id };
  } catch (err) {
    return fail(err);
  }
}

// Local duplicate of the parseSetCookie helper shared across login
// and pick-tenant. Will hoist to lib/ once a fourth caller shows up.
function parseSetCookie(raw: string): {
  name: string;
  value: string;
  path?: string;
  domain?: string;
  httpOnly: boolean;
  secure: boolean;
  maxAge?: number;
} | null {
  const parts = raw.split(";").map((p) => p.trim());
  const [first, ...attrs] = parts;
  if (!first || !first.includes("=")) return null;
  const eq = first.indexOf("=");
  const name = first.slice(0, eq);
  const value = first.slice(eq + 1);

  const out = {
    name,
    value,
    httpOnly: false,
    secure: false,
  } as {
    name: string;
    value: string;
    path?: string;
    domain?: string;
    httpOnly: boolean;
    secure: boolean;
    maxAge?: number;
  };
  for (const attr of attrs) {
    const lower = attr.toLowerCase();
    if (lower === "httponly") out.httpOnly = true;
    else if (lower === "secure") out.secure = true;
    else if (lower.startsWith("path=")) out.path = attr.slice(5);
    else if (lower.startsWith("domain=")) out.domain = attr.slice(7);
    else if (lower.startsWith("max-age="))
      out.maxAge = parseInt(attr.slice(8), 10);
  }
  return out;
}
