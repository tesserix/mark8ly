/**
 * Normalises every shape auth-bff returns on a login path into one union.
 *
 * Four shapes exist, for historical reasons rather than good ones:
 *   - /auth/auto-login success  -> { data: { uid, email, tenant_id, mfa_required?, email_otp_required? } }
 *   - /auth/otp/verify success  -> { uid, tenant_id }            (top level, no data)
 *   - any error                 -> { error, message }            (flat)
 *   - /auth/zitadel/login       -> { totp_required, session_id, session_token } or
 *                                  { handoff_url, auth_request_id } or
 *                                  { callback_url, data: { ... } }
 *
 * Two DIFFERENT second factors are represented here. Zitadel's own TOTP arrives
 * as top-level `totp_required`; auth-bff's usermfa gate arrives as nested
 * `data.mfa_required`. Handling one and not the other is exactly the defect that
 * took merchant login down in #493/#502 — auth-bff said email_otp_required, the
 * client read only mfa_required, and the code-entry screen never rendered.
 */
export type LoginOutcome =
  | { kind: "complete"; uid: string; email: string; tenantId: string; callbackUrl?: string }
  | { kind: "totp_required"; sessionId: string; sessionToken: string }
  | { kind: "mfa_required" }
  | { kind: "email_otp_required" }
  | { kind: "handoff"; handoffUrl: string };

export class LoginResponseError extends Error {}

export function parseLoginResponse(body: unknown): LoginOutcome {
  if (typeof body !== "object" || body === null) {
    throw new LoginResponseError("login response was not an object");
  }
  const top = body as Record<string, unknown>;
  const data = (typeof top.data === "object" && top.data !== null
    ? (top.data as Record<string, unknown>)
    : {}) as Record<string, unknown>;

  // Step-ups first: a body carrying both a factor requirement and a completion
  // must never be read as complete.
  if (top.totp_required === true) {
    const sessionId = typeof top.session_id === "string" ? top.session_id : "";
    const sessionToken = typeof top.session_token === "string" ? top.session_token : "";
    if (!sessionId || !sessionToken) {
      throw new LoginResponseError("totp_required without a session to continue");
    }
    return { kind: "totp_required", sessionId, sessionToken };
  }
  if (data.mfa_required === true) return { kind: "mfa_required" };
  if (data.email_otp_required === true) return { kind: "email_otp_required" };

  if (typeof top.handoff_url === "string" && top.handoff_url) {
    return { kind: "handoff", handoffUrl: top.handoff_url };
  }

  const uid = typeof data.uid === "string" ? data.uid : typeof top.uid === "string" ? top.uid : "";
  const tenantId =
    typeof data.tenant_id === "string" ? data.tenant_id
    : typeof top.tenant_id === "string" ? top.tenant_id : "";
  const email = typeof data.email === "string" ? data.email : "";
  if (!uid || !tenantId) {
    throw new LoginResponseError("login response carried neither a step-up nor an identity");
  }
  const callbackUrl = typeof top.callback_url === "string" ? top.callback_url : undefined;
  return { kind: "complete", uid, email, tenantId, ...(callbackUrl ? { callbackUrl } : {}) };
}
