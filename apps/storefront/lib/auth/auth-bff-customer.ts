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
//   401 unverified -> {"error": "email_not_verified"} (login only — see below)
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
// reintroduce that distinction: every 401 from that error path maps to the
// same `rejected` outcome below, with no further detail. A 5xx or a
// transport failure is a DIFFERENT, distinguishable case
// (AuthBffCustomerError, thrown rather than returned) so the UI can tell
// "wrong credentials" from "auth is down".
//
// One 401 code is the deliberate exception: {"error":"email_not_verified"}
// on the login endpoint is surfaced as its own `email_not_verified`
// outcome, not collapsed. See parseCustomerOutcome's handling of it below
// for why this does not reopen the enumeration oracle.

const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8087";

/**
 * The shared service-to-service secret auth-bff's Zitadel credential
 * endpoints require in the X-Internal-Auth header — the same scheme and the
 * same env var this app already uses for marketplace-api's internal routes
 * (see app/api/internal/orders/[id]/invoice/route.ts).
 *
 * auth-bff is publicly reachable (auth.mark8ly.com routes to it on any
 * path) and /auth/customer/login answers whether a {login_name, password}
 * pair is valid, so without this header those routes are a credential
 * oracle over every user in the Zitadel instance. This module is
 * server-side only, so the header costs one line — read from server config,
 * never a NEXT_PUBLIC_* variable, which would ship the secret to the
 * browser bundle.
 *
 * Read at call time rather than module scope so a value injected after
 * module evaluation (and a test's stubbed env) is still seen.
 */
function internalAuthHeader(): Record<string, string> {
  const secret = process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";
  return secret ? { "X-Internal-Auth": secret } : {};
}

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
  | { kind: "rejected" }
  | { kind: "email_not_verified" };

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
      headers: { "Content-Type": "application/json", ...internalAuthHeader() },
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
 *  `rejected` outcome (with one explicit exception, see below),
 *  other non-2xx -> AuthBffCustomerError, 2xx -> the matching outcome
 *  by shape. */
async function parseCustomerOutcome(
  res: Response,
): Promise<CustomerAuthOutcome> {
  if (res.status === 401) {
    // Both ErrBadCredentials and ErrUserNotFound (login) and both a wrong
    // TOTP code and a vanished session (totp) arrive here as the identical
    // {"error": "invalid_credentials"} / {"error": "invalid_totp"} body.
    // Collapse those to one outcome with no further detail — do not read
    // the `error` field for anything but the one allowlisted exception
    // below, and do not distinguish by any other value it carries.
    //
    // The ONE exception: {"error": "email_not_verified"}. This is safe to
    // surface distinctly, unlike every other 401, because of WHERE it sits
    // in auth-bff's login handler (services/auth-bff/internal/
    // zitadellogin/customer_handler.go:363-369): it is only ever returned
    // AFTER CreatePasswordSession has already SUCCEEDED, i.e. the caller
    // already holds this account's correct password. Revealing
    // "email_not_verified" instead of the generic rejection tells such a
    // caller nothing they didn't already prove they knew — it is not a new
    // account-enumeration oracle for someone who does NOT hold a valid
    // credential, because they can never reach this branch in the first
    // place (they get collapsed `rejected`, like everyone else without the
    // password). Do not "helpfully" fold this back into `rejected` — that
    // would silently reintroduce the storefront bug where a customer with
    // the CORRECT password is told their password is wrong, with no path
    // to recovery (see the phase brief / customer-signup-messages.ts's
    // already-written, previously-unreachable copy for this exact case).
    //
    // Read the body defensively: anything other than exactly
    // {"error":"email_not_verified"} — an unknown code, a missing field, a
    // malformed body — falls through to the collapsed `rejected` outcome
    // below. This must stay an explicit allowlist of one literal string,
    // never a passthrough of whatever the body says.
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error === "email_not_verified") {
        return { kind: "email_not_verified" };
      }
    } catch {
      // Non-JSON or unreadable body — fall through to `rejected`.
    }
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

// ---------------------------------------------------------------------------
// Google-through-Zitadel: POST /auth/customer/idp/start and
// POST /auth/customer/idp/finish.
//
// Unlike verifyCustomerCredential/verifyCustomerTotp above, idp/finish's
// non-2xx responses are NOT collapsed to one outcome — there is no
// account-enumeration concern here (an intent id/token pair is not a
// guessable credential the way a password is), and each business outcome
// (email_not_verified, email_taken, email_ambiguous, unexpected_idp,
// invalid_intent, ...) needs to stay distinguishable so the caller can
// show a truthful, distinct message for each — see
// apps/storefront/app/auth/idp/finish/route.ts and the phase brief's
// constraint that email_taken (permanent lockout) must never be worded
// like email_not_verified or a transient failure.
//
// A genuinely unreadable failure (malformed body, transport error) still
// throws AuthBffCustomerError, exactly like the password path, so a
// caller can tell "auth-bff told us no" from "we couldn't find out".

/** Wire shape for POST /auth/customer/idp/start. */
interface StartCustomerIDPStartBody {
  auth_url?: string;
}

/**
 * startCustomerIDPIntent submits {return_url} to
 * POST /auth/customer/idp/start and returns the authUrl the browser must
 * be redirected to. returnUrl must already be this store's own
 * /auth/idp/finish route (validated server-side against auth-bff's
 * storefront return-url allowlist) — see idpintent.go's StartIDPIntent
 * doc: Zitadel does not validate it at all, so a bad value here is an
 * open redirect waiting to happen.
 *
 * Never call this from a client component — see the file header.
 */
export async function startCustomerIDPIntent(returnUrl: string): Promise<string> {
  const res = await postToCustomerEndpoint("/auth/customer/idp/start", {
    return_url: returnUrl,
  });

  if (!res.ok) {
    throw new AuthBffCustomerError(res.status, await readErrorCode(res));
  }

  let body: StartCustomerIDPStartBody;
  try {
    body = (await res.json()) as StartCustomerIDPStartBody;
  } catch {
    throw new AuthBffCustomerError(res.status, "invalid_response_body");
  }
  if (!body.auth_url) {
    throw new AuthBffCustomerError(res.status, "unrecognised_response_shape");
  }
  return body.auth_url;
}

/** Outcome of POST /auth/customer/idp/finish. */
export type CustomerIDPFinishOutcome =
  | { kind: "complete"; uid: string; email: string }
  /**
   * `code` is one of auth-bff's documented idp/finish outcomes
   * (email_not_verified, email_taken, email_ambiguous, unexpected_idp,
   * invalid_intent, zitadel_unavailable, internal_error, ...) or an
   * `http_<status>`/`*_response_*` fallback for anything unrecognised.
   * Never rendered to the shopper verbatim — the caller maps each known
   * code to its own truthful copy and falls back to a generic message for
   * anything else. See the file-header doc above for why these are kept
   * distinguishable instead of collapsed the way a 401 is on the
   * credential path.
   */
  | { kind: "failed"; code: string };

interface FinishCustomerIDPArgs {
  intentId: string;
  intentToken: string;
}

/** Wire shape for POST /auth/customer/idp/finish. */
type CustomerIDPFinishBody = { data: { uid: string; email: string } };

/**
 * finishCustomerIDPIntent submits {intent_id, intent_token} to
 * POST /auth/customer/idp/finish and returns the resulting outcome.
 *
 * Deliberately takes ONLY intentId/intentToken — never a `user` value.
 * Zitadel's redirect back to the browser can carry a `user` query param,
 * but it rides in a URL the browser followed and is attacker-controlled;
 * the caller (apps/storefront/app/auth/idp/finish/route.ts) must never
 * read it for anything, and this function has no parameter for it at all
 * so there is nothing to accidentally forward.
 *
 * Mints no session/cookie — same as verifyCustomerCredential/
 * verifyCustomerTotp, the caller does that via completeCustomerSignIn.
 *
 * Never call this from a client component — see the file header.
 */
export async function finishCustomerIDPIntent(
  args: FinishCustomerIDPArgs,
): Promise<CustomerIDPFinishOutcome> {
  const res = await postToCustomerEndpoint("/auth/customer/idp/finish", {
    intent_id: args.intentId,
    intent_token: args.intentToken,
  });

  if (!res.ok) {
    return { kind: "failed", code: await readErrorCode(res) };
  }

  let body: CustomerIDPFinishBody;
  try {
    body = (await res.json()) as CustomerIDPFinishBody;
  } catch {
    return { kind: "failed", code: "invalid_response_body" };
  }
  if (body.data && typeof body.data.uid === "string") {
    return { kind: "complete", uid: body.data.uid, email: body.data.email };
  }
  return { kind: "failed", code: "unrecognised_response_shape" };
}

// ---------------------------------------------------------------------------
// Customer sign-up (phase 6a task 3): POST /auth/customer/register and
// POST /auth/customer/verify-email.
//
// Same discipline as idp/start + idp/finish above: outcomes are NOT
// collapsed to one value the way the 401 password path is. There is no
// account-enumeration concern here — a sign-up caller chooses the email
// being registered, so a distinct code tells them nothing they did not
// already control — and each of register's failure codes (email_taken,
// email_ambiguous, weak_password, verification_email_failed,
// zitadel_unavailable) needs its own truthful, distinct copy. See
// lib/auth/customer-signup-messages.ts and the phase brief's constraint
// that email_taken (a permanent state for that address) must never read
// like verification_email_failed (the account was rolled back, so
// retrying genuinely works).
//
// A genuinely unreadable failure (malformed body, transport error) still
// throws AuthBffCustomerError, exactly like every other client in this
// file, so a caller can tell "auth-bff told us no" from "we couldn't find
// out".

interface RegisterCustomerAccountArgs {
  email: string;
  password: string;
  givenName?: string;
  familyName?: string;
}

/** Outcome of POST /auth/customer/register. */
export type CustomerRegisterOutcome =
  | { kind: "created"; uid: string; email: string }
  /**
   * `code` is one of auth-bff's documented register outcomes (email_taken,
   * email_ambiguous, weak_password, verification_email_failed,
   * zitadel_unavailable, invalid_request, ...) or an `http_<status>`/
   * `*_response_*` fallback for anything unrecognised. Never rendered to
   * the shopper verbatim — see customer-signup-messages.ts.
   */
  | { kind: "failed"; code: string };

/** Wire shape for POST /auth/customer/register's 2xx body. */
type CustomerRegisterBody = { data: { uid: string; email: string } };

/**
 * registerCustomerAccount submits {email, password[, given_name,
 * family_name]} to POST /auth/customer/register and returns the resulting
 * outcome. The account it creates is UNVERIFIED until a follow-up
 * verifyCustomerEmailCode call succeeds — see that function's doc.
 *
 * Never call this from a client component — see the file header.
 */
export async function registerCustomerAccount(
  args: RegisterCustomerAccountArgs,
): Promise<CustomerRegisterOutcome> {
  const body: Record<string, string> = {
    email: args.email,
    password: args.password,
  };
  if (args.givenName) body.given_name = args.givenName;
  if (args.familyName) body.family_name = args.familyName;

  const res = await postToCustomerEndpoint("/auth/customer/register", body);

  if (!res.ok) {
    return { kind: "failed", code: await readErrorCode(res) };
  }

  let parsed: CustomerRegisterBody;
  try {
    parsed = (await res.json()) as CustomerRegisterBody;
  } catch {
    return { kind: "failed", code: "invalid_response_body" };
  }
  if (
    parsed.data &&
    typeof parsed.data.uid === "string" &&
    typeof parsed.data.email === "string"
  ) {
    return { kind: "created", uid: parsed.data.uid, email: parsed.data.email };
  }
  return { kind: "failed", code: "unrecognised_response_shape" };
}

interface VerifyCustomerEmailCodeArgs {
  uid: string;
  /** The 6-character code the shopper read out of their verification
   *  email. A live credential for the account — never logged, never
   *  echoed back in any outcome value, and never embedded in a thrown
   *  error (see AuthBffCustomerError's doc). */
  code: string;
}

/** Outcome of POST /auth/customer/verify-email. */
export type CustomerVerifyEmailOutcome =
  | { kind: "verified" }
  /**
   * `code` is one of auth-bff's documented verify-email outcomes
   * (invalid_verification_code, zitadel_unavailable, ...) or an
   * `http_<status>`/`*_response_*` fallback for anything unrecognised.
   * Never rendered to the shopper verbatim — see
   * customer-signup-messages.ts.
   */
  | { kind: "failed"; code: string };

/**
 * verifyCustomerEmailCode submits {uid, code} to
 * POST /auth/customer/verify-email and returns the resulting outcome.
 *
 * The 2xx body ({"data":{"verified":true}}) carries no field the caller
 * needs beyond "it worked" — deliberately not parsed for anything past the
 * status check, so there is nothing in it to mishandle.
 *
 * Never call this from a client component — see the file header.
 */
export async function verifyCustomerEmailCode(
  args: VerifyCustomerEmailCodeArgs,
): Promise<CustomerVerifyEmailOutcome> {
  const res = await postToCustomerEndpoint("/auth/customer/verify-email", {
    uid: args.uid,
    code: args.code,
  });

  if (!res.ok) {
    return { kind: "failed", code: await readErrorCode(res) };
  }
  return { kind: "verified" };
}

/** Shared best-effort `{error}` field extraction for a non-2xx response. */
async function readErrorCode(res: Response): Promise<string> {
  let code = `http_${res.status}`;
  try {
    const errBody = (await res.json()) as { error?: string };
    if (typeof errBody.error === "string" && errBody.error) {
      code = errBody.error;
    }
  } catch {
    // Non-JSON error body — keep the http_<status> fallback code.
  }
  return code;
}
