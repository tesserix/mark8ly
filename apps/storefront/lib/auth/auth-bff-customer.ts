// Server-side client for auth-bff's storefront customer credential
// endpoints (POST /auth/customer/login and POST /auth/customer/totp).
//
// SERVER-SIDE ONLY. AUTH_BFF_URL is server config (never a NEXT_PUBLIC_*
// variable) because the customer endpoint sits behind auth-bff's own
// Zitadel login-client PAT (see services/auth-bff/pkg/config/config.go's
// ZITADEL_LOGIN_CLIENT_TOKEN) — that token is used server-to-server between
// auth-bff and Zitadel and must never be reachable from a value the browser
// bundle can read. This module must only be imported from a server action
// or route handler (see apps/storefront/app/sign-in/actions.ts, the eventual
// caller), never from client components.
//
// Shape reference: services/auth-bff/internal/zitadellogin/customer_handler.go
//   POST /auth/customer/login  <- {login_name, password}
//   POST /auth/customer/totp   <- {session_id, session_token, code}
//   200 complete   -> {"data": {"uid": string, "email": string}}
//   200 factor-req -> {"totp_required": true, "session_id": string, "session_token": string}
//   200 handoff    -> {"handoff_url": string}
//   401 rejected   -> {"error": "invalid_credentials"} or {"error": "invalid_totp"}
//   503/5xx        -> {"error": "zitadel_unavailable" | "internal_error" | ...}
//
// Neither endpoint takes an auth_request_id: the customer path makes a
// sufficiency decision and returns an identity — it never finalizes, so it
// never obtains (or needs) an OIDC authorization code. See
// services/auth-bff/internal/zitadellogin/sufficiency.go's DecideSufficiency
// / DecideAfterFactor and the file comment on customer_handler.go.
//
// The 401 case is deliberately collapsed to ONE outcome on the wire —
// CustomerHandler.respondSessionCreateError returns the identical
// {"error":"invalid_credentials"} whether the password was wrong or the
// account doesn't exist, because a different answer would be an
// account-enumeration oracle on a public storefront. This client must not
// reintroduce that distinction: every 401 maps to the same `rejected`
// outcome below, with no further detail. A 5xx or a transport failure is a
// DIFFERENT, distinguishable case (AuthBffCustomerError, thrown rather than
// returned) so the UI can tell "wrong credentials" from "auth is down".

const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8087";

/**
 * Outcome union mirroring the endpoint's four possible shapes. Modeled on
 * apps/admin/lib/auth/login-response.ts's LoginOutcome so the merchant and
 * customer login paths read alike, but intentionally NOT shared code —
 * these are separate apps and the customer path has one fewer step-up type
 * (no usermfa/email-OTP gate; Storefront customers never go through that
 * gauntlet, see the file comment on customer_handler.go) and no session
 * cookie / tenant id (the customer endpoint mints no session; the caller,
 * apps/storefront/app/sign-in/actions.ts, does that itself).
 */
export type CustomerAuthOutcome =
  | { kind: "complete"; uid: string; email: string }
  | { kind: "totp_required"; sessionId: string; sessionToken: string }
  | { kind: "handoff"; handoffUrl: string }
  | { kind: "rejected" };

/**
 * Thrown for anything that is NOT one of the endpoint's normal outcomes:
 * a 5xx from auth-bff, a malformed 2xx body, or a transport-level failure
 * (fetch rejecting outright). Deliberately distinct from the `rejected`
 * union member above so callers can tell "the credential was wrong" from
 * "we couldn't find out" and avoid, e.g., telling a customer their
 * password is wrong when auth-bff is actually down.
 *
 * `message` is always a fixed, static string — never built from the
 * request body or the raw response body — so a password can never end up
 * in a thrown error.
 */
export class AuthBffCustomerError extends Error {
  constructor(
    public status: number,
    public code: string,
  ) {
    super(`auth-bff customer endpoint error: ${code} (status ${status})`);
    this.name = "AuthBffCustomerError";
  }
}

interface VerifyCustomerCredentialArgs {
  loginName: string;
  password: string;
}

interface VerifyCustomerTotpArgs {
  sessionId: string;
  sessionToken: string;
  code: string;
}

/** Narrow, structurally-typed shapes for the bodies we read fields off of.
 *  Never logged or embedded in an error — see AuthBffCustomerError above. */
type CustomerLoginBody =
  | { data: { uid: string; email: string } }
  | { totp_required: true; session_id: string; session_token: string }
  | { handoff_url: string };

/**
 * verifyCustomerCredential submits {login_name, password} to
 * POST /auth/customer/login and returns the resulting outcome.
 *
 * Never call this from a client component or route that ships to the
 * browser — see the file header.
 */
export async function verifyCustomerCredential(
  args: VerifyCustomerCredentialArgs,
): Promise<CustomerAuthOutcome> {
  const res = await postToCustomerEndpoint("/auth/customer/login", {
    login_name: args.loginName,
    password: args.password,
  });
  return parseCustomerOutcome(res);
}

/**
 * verifyCustomerTotp submits {session_id, session_token, code} to
 * POST /auth/customer/totp and returns the resulting outcome.
 *
 * Never call this from a client component or route that ships to the
 * browser — see the file header.
 */
export async function verifyCustomerTotp(
  args: VerifyCustomerTotpArgs,
): Promise<CustomerAuthOutcome> {
  const res = await postToCustomerEndpoint("/auth/customer/totp", {
    session_id: args.sessionId,
    session_token: args.sessionToken,
    code: args.code,
  });
  return parseCustomerOutcome(res);
}

/** Shared POST + transport-failure handling for both endpoints. */
async function postToCustomerEndpoint(
  path: string,
  body: Record<string, string>,
): Promise<Response> {
  try {
    return await fetch(`${AUTH_BFF_URL}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
  } catch {
    // A network-level failure (fetch rejects). Deliberately no cause/detail
    // from the caught error is included — it could theoretically echo
    // request internals — and the request body (which may hold the
    // password) is never referenced here.
    throw new AuthBffCustomerError(0, "network_error");
  }
}

/** Shared response parsing for both endpoints: 401 -> the single
 *  `rejected` outcome, other non-2xx -> AuthBffCustomerError, 2xx ->
 *  the matching outcome by shape. */
async function parseCustomerOutcome(
  res: Response,
): Promise<CustomerAuthOutcome> {
  if (res.status === 401) {
    // Both ErrBadCredentials and ErrUserNotFound (login) and both a wrong
    // TOTP code and a vanished session (totp) arrive here as the identical
    // {"error": "invalid_credentials"} / {"error": "invalid_totp"} body.
    // Collapse to one outcome with no further detail — do not read the
    // `error` field and do not distinguish by it.
    return { kind: "rejected" };
  }

  if (!res.ok) {
    let code = `http_${res.status}`;
    try {
      const errBody = (await res.json()) as { error?: string };
      if (typeof errBody.error === "string" && errBody.error) {
        code = errBody.error;
      }
    } catch {
      // Non-JSON error body — keep the http_<status> fallback code.
    }
    throw new AuthBffCustomerError(res.status, code);
  }

  let body: CustomerLoginBody;
  try {
    body = (await res.json()) as CustomerLoginBody;
  } catch {
    throw new AuthBffCustomerError(res.status, "invalid_response_body");
  }

  if ("totp_required" in body && body.totp_required === true) {
    return {
      kind: "totp_required",
      sessionId: body.session_id,
      sessionToken: body.session_token,
    };
  }

  if ("handoff_url" in body && typeof body.handoff_url === "string") {
    return { kind: "handoff", handoffUrl: body.handoff_url };
  }

  if ("data" in body && body.data && typeof body.data.uid === "string") {
    return { kind: "complete", uid: body.data.uid, email: body.data.email };
  }

  throw new AuthBffCustomerError(res.status, "unrecognised_response_shape");
}
