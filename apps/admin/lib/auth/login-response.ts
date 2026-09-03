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

/** True when `key` is exactly `true` at EITHER nesting level.
 *
 * auth-bff's envelopes are inconsistent by endpoint — /auth/auto-login nests under
 * `data`, /auth/otp/verify does not, and /auth/zitadel/login mixes both in one body.
 * Checking only the level we expect is what makes a nesting change silently complete
 * a login with a factor outstanding, which is the defect class this module exists to
 * stop. We act only on `=== true`, so an unexpected placement can at worst prompt for
 * a factor that was not needed — never skip one. */
function flagAtEitherLevel(top: Record<string, unknown>, data: Record<string, unknown>, key: string): boolean {
  return top[key] === true || data[key] === true;
}

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
  if (flagAtEitherLevel(top, data, "totp_required")) {
    const sessionId =
      typeof top.session_id === "string" ? top.session_id
      : typeof data.session_id === "string" ? data.session_id : "";
    const sessionToken =
      typeof top.session_token === "string" ? top.session_token
      : typeof data.session_token === "string" ? data.session_token : "";
    if (!sessionId || !sessionToken) {
      throw new LoginResponseError("totp_required without a session to continue");
    }
    return { kind: "totp_required", sessionId, sessionToken };
  }
  if (flagAtEitherLevel(top, data, "mfa_required")) return { kind: "mfa_required" };
  if (flagAtEitherLevel(top, data, "email_otp_required")) return { kind: "email_otp_required" };

  const handoffUrl = typeof top.handoff_url === "string" ? top.handoff_url
    : typeof data.handoff_url === "string" ? data.handoff_url : "";
  if (handoffUrl) {
    return { kind: "handoff", handoffUrl };
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
