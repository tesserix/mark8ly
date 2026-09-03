"use server";

// Server action for storefront customer sign-in.
//
// Customer auth skips auth-bff's merchant gauntlet entirely — auth-bff's
// auto-login checks OpenFGA tenant membership which customers don't have.
// Instead we:
//   1. Verify the credential — under GIP, the Identity Toolkit id_token
//      (signature, project, tenant, expiry); under Zitadel, the login
//      name + password via auth-bff's storefront-customer endpoint
//      (see @/lib/auth/auth-bff-customer). This is the ONLY step that
//      differs between providers — see the comment on AUTH_PROVIDER below.
//   2. Set an HMAC-signed `mp_customer_session` cookie.
//   3. Ensure the customer profile exists in marketplace-api.

import { cookies, headers } from "next/headers";
import { encodeSession } from "@/lib/session";
import {
  GIPTokenVerificationError,
  verifyGIPIdToken,
} from "@/lib/gip/verify-id-token";
import {
  verifyCustomerCredential,
  verifyCustomerTotp,
} from "@/lib/auth/auth-bff-customer";
import { sanitizeHost } from "@/lib/host";
import { platformInternalFetch } from "@/lib/api/server/platformInternal";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";
const GIP_PROJECT_ID = process.env.GIP_PROJECT_ID ?? "";
const GIP_CUSTOMER_TENANT_ID = process.env.GIP_CUSTOMER_TENANT_ID ?? "";

// Which identity provider verifies the credential in customerSignIn below.
// Read defensively, matching apps/admin/lib/config.ts's publicConfig.authProvider
// rule exactly: only the literal string "zitadel" switches the path — unset,
// empty, or any other value (including "Zitadel" or "true") stays on GIP.
// NEXT_PUBLIC_-prefixed (not server-only) because CustomerSignInForm, a client
// component, must branch on the identical value to decide whether to call
// Identity Toolkit from the browser at all.
const AUTH_PROVIDER: "gip" | "zitadel" =
  process.env.NEXT_PUBLIC_AUTH_PROVIDER === "zitadel" ? "zitadel" : "gip";

type TotpRequiredResult = {
  ok: false;
  code: "totp_required";
  message: string;
  sessionId: string;
  sessionToken: string;
};

type Result =
  | { ok: true }
  | { ok: false; code: string; message: string }
  | TotpRequiredResult;

/**
 * isTotpRequiredResult narrows a `Result` to the totp_required variant.
 *
 * `code` can't be used as an automatic discriminant here — the plain
 * failure variant types `code` as `string`, not a set of literals, which
 * disqualifies it from TypeScript's discriminated-union narrowing (an
 * equality check like `result.code === "totp_required"` type-checks but
 * does not narrow `result` itself). Callers (the sign-in form,
 * corresponding tests) use this instead of that equality check.
 */
export function isTotpRequiredResult(r: Result): r is TotpRequiredResult {
  return !r.ok && r.code === "totp_required";
}

interface CustomerSignInInput {
  storeSlug: string;
  /** GIP path only: the verified Identity Toolkit id_token. */
  idToken?: string;
  /** Deprecated: ignored. The trusted UID comes from the verified credential. */
  uid?: string;
  /** Deprecated: ignored. The trusted email comes from the verified credential. */
  email?: string;
  /** Zitadel path only: the login name (email) collected by the form. */
  loginName?: string;
  /** Zitadel path only: the password collected by the form. Never logged. */
  password?: string;
}

async function resolveStore(
  storeSlug: string,
): Promise<{ tenant_id: string; store_id: string } | null> {
  try {
    const res = await platformInternalFetch(
      `/internal/stores/by-slug/${encodeURIComponent(storeSlug)}`,
      { cache: "no-store" },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as {
      data?: { tenant_id?: string; id?: string };
    };
    if (!body.data?.tenant_id || !body.data?.id) return null;
    return { tenant_id: body.data.tenant_id, store_id: body.data.id };
  } catch {
    return null;
  }
}

// Best-effort: register the customer profile in marketplace-api so
// they show up in the merchant's customer list and can place orders.
// We call the web storefront /account endpoint (GET) with the session
// cookie — the OptionalCustomerAuth middleware automatically calls
// EnsureProfile when it sees a valid session cookie.
async function ensureCustomerProfile(
  storeSlug: string,
  cookieValue: string,
): Promise<void> {
  try {
    const headers: Record<string, string> = {
      Accept: "application/json",
      Cookie: `mp_customer_session=${cookieValue}`,
    };
    if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;

    await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/account`,
      {
        method: "GET",
        headers,
        cache: "no-store",
      },
    );
    // The GET /account call triggers OptionalCustomerAuth middleware
    // which calls EnsureProfile. We don't care about the response —
    // the side effect (profile creation) is what matters.
  } catch {
    // silent — customer can still browse
  }
}

// Best-effort: auto-enroll the customer in the store's loyalty program.
// Backend is idempotent — returns the existing record on repeat calls.
// If a referral code was captured in the mp_referral cookie, pass it
// through so the referrer and referee bonuses are awarded atomically.
// Silent on failure — loyalty is a growth feature, not a blocker.
async function ensureLoyaltyEnrollment(
  storeSlug: string,
  email: string,
  referralCode: string | undefined,
): Promise<void> {
  if (!email) return;
  try {
    const headers: Record<string, string> = {
      Accept: "application/json",
      "Content-Type": "application/json",
    };
    if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;

    const body: Record<string, unknown> = { email };
    if (referralCode) body.referral_code = referralCode;

    await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/loyalty/enroll`,
      {
        method: "POST",
        headers,
        body: JSON.stringify(body),
        cache: "no-store",
      },
    );
  } catch {
    // silent — program may be inactive or network blip
  }
}

/**
 * completeCustomerSignIn is the tail shared by every path that ends in a
 * verified {uid, email}: minting the HMAC-signed session cookie scoped to
 * the resolved host, the best-effort profile and loyalty side effects, and
 * burning the referral cookie. `customerSignIn` (password/id-token path)
 * and `confirmCustomerTotp` (authenticator-code path) both call this
 * instead of each carrying their own copy — a second copy of this tail
 * would drift the moment one path changed and the other didn't.
 */
async function completeCustomerSignIn(
  store: { tenant_id: string; store_id: string },
  cookieHost: string,
  storeSlug: string,
  verified: { uid: string; email: string },
): Promise<Result> {
  // Set the HMAC-signed customer session cookie. The storefront
  // layout decodes and verifies this on every request to hydrate
  // the authenticated state.
  const cookieValue = encodeSession({
    uid: verified.uid,
    email: verified.email,
    store_slug: storeSlug,
    store_id: store.store_id,
    tenant_id: store.tenant_id,
  });

  const c = await cookies();
  c.set({
    name: "mp_customer_session",
    value: cookieValue,
    path: "/",
    domain: cookieHost, // scoped to exact host so store-a's session can't be sent to store-b
    httpOnly: true,
    secure: true,
    sameSite: "lax",
    maxAge: 60 * 60 * 24 * 30, // 30 days
  });

  // Best-effort profile registration — pass the freshly minted cookie
  // so marketplace-api's OptionalCustomerAuth can validate it and call
  // EnsureProfile.
  await ensureCustomerProfile(storeSlug, cookieValue);

  // Auto-enroll in loyalty (idempotent — signup bonus awarded once).
  // Reads the mp_referral cookie captured by middleware on a prior
  // page hit, so referral attribution survives the GIP signup dance.
  const referralCode = c.get("mp_referral")?.value;
  await ensureLoyaltyEnrollment(storeSlug, verified.email, referralCode);
  if (referralCode) {
    // Burn the cookie so the same invite link can't be replayed by
    // the same browser for another account.
    c.delete("mp_referral");
  }

  return { ok: true };
}

/**
 * handleSignInError maps a thrown error to the shared, user-facing
 * `Result` shape. Shared by `customerSignIn` and `confirmCustomerTotp` so
 * neither path can accidentally leak an internal error string (e.g.
 * AuthBffCustomerError's `.message`, which is literally
 * "auth-bff customer endpoint error: <code> (status <n>)") to the
 * shopper. The detail is logged server-side only.
 */
function handleSignInError(err: unknown, logLabel: string): Result {
  if (err instanceof GIPTokenVerificationError) {
    return {
      ok: false,
      code: "invalid_token",
      message: "Your sign-in session could not be verified. Please sign in again.",
    };
  }
  console.error(logLabel, err);
  return {
    ok: false,
    code: "unknown",
    message: "Sign-in is temporarily unavailable. Please try again shortly.",
  };
}

export async function customerSignIn(
  input: CustomerSignInInput,
): Promise<Result> {
  try {
    // Resolve and validate the customer-facing host first — it gates
    // the cookie Domain, so an invalid host means we cannot complete
    // sign-in regardless of token validity.
    const h = await headers();
    const rawHost = h.get("x-forwarded-host") ?? h.get("host");
    const cookieHost = sanitizeHost(rawHost);
    if (!cookieHost) {
      return {
        ok: false,
        code: "invalid_host",
        message: "Could not validate the host for sign-in. Please try again.",
      };
    }

    const store = await resolveStore(input.storeSlug);
    if (!store) {
      return {
        ok: false,
        code: "store_not_found",
        message: "Could not resolve this store. Please try again.",
      };
    }

    // The ONLY step that differs between providers: how the credential is
    // verified and where {uid, email} come from. Everything below this
    // block — cookie minting, domain scoping, profile/loyalty side
    // effects — is identical on both paths.
    let verified: { uid: string; email: string };
    if (AUTH_PROVIDER === "zitadel") {
      if (!input.loginName || !input.password) {
        return {
          ok: false,
          code: "invalid_request",
          message: "Email and password are required.",
        };
      }
      const outcome = await verifyCustomerCredential({
        loginName: input.loginName,
        password: input.password,
      });
      // Each non-"complete" outcome gets its own truthful message. None of
      // them may render as "email or password is incorrect" — that would
      // be false for a customer who typed the right password but hit a
      // factor this endpoint can't finish (totp_required) or can't collect
      // at all (handoff), and would send them into a retry loop that can
      // never succeed. No cookie is set for any outcome below.
      switch (outcome.kind) {
        case "complete":
          verified = { uid: outcome.uid, email: outcome.email };
          break;
        case "rejected":
          // The one case this message IS true for: a wrong password or an
          // unknown login name, collapsed to a single outcome upstream
          // (see auth-bff-customer.ts) so this response can't be used to
          // enumerate accounts.
          return {
            ok: false,
            code: "invalid_credentials",
            message: "Email or password is incorrect.",
          };
        case "totp_required":
          // The account has an authenticator app enrolled. No cookie is
          // set yet — the client must collect the 6-digit code and call
          // confirmCustomerTotp with these carried-through session
          // fields to finish sign-in (mirrors the admin app's Zitadel
          // TOTP step-up).
          return {
            ok: false,
            code: "totp_required",
            message:
              "Enter the 6-digit code from your authenticator app to finish signing in.",
            sessionId: outcome.sessionId,
            sessionToken: outcome.sessionToken,
          };
        case "handoff":
          // A real, uncollectible factor (passkey, U2F, SMS OTP, recovery
          // code, ...). The handoff URL is deliberately never surfaced —
          // it lands the customer in Zitadel's own hosted UI, which mints
          // no `mp_customer_session`, so following it would not complete
          // sign-in on this storefront either.
          return {
            ok: false,
            code: "signin_method_unsupported",
            message:
              "This account uses a sign-in method this storefront can't complete yet. Please contact support for help signing in.",
          };
      }
    } else {
      verified = await verifyGIPIdToken(
        input.idToken ?? "",
        GIP_PROJECT_ID,
        GIP_CUSTOMER_TENANT_ID,
      );
    }

    return await completeCustomerSignIn(store, cookieHost, input.storeSlug, verified);
  } catch (err) {
    return handleSignInError(err, "customerSignIn failed with an unexpected error");
  }
}

/**
 * confirmCustomerTotp finishes a customer sign-in that
 * `verifyCustomerCredential` step-up'd with a `totp_required` outcome
 * (Zitadel itself demanding a verified authenticator code before the
 * login can complete — distinct from any auth-bff-side gate). Mirrors
 * `apps/admin/app/login/actions.ts`'s `confirmZitadelTotp`: the client
 * carries `sessionId`/`sessionToken` from the first call unchanged into
 * this one, because minting the session server-side (via
 * `PATCH /v2/sessions/{id}`) requires the instance login-client PAT that
 * only auth-bff holds — there is no pending-cookie mechanism to recover
 * these from the server side instead.
 *
 * Only reachable on the Zitadel path: under GIP, `verifyGIPIdToken` never
 * produces a `totp_required` outcome, so the client can never obtain a
 * sessionId/sessionToken to call this with.
 */
export async function confirmCustomerTotp(input: {
  storeSlug: string;
  sessionId: string;
  sessionToken: string;
  code: string;
}): Promise<Result> {
  try {
    const h = await headers();
    const rawHost = h.get("x-forwarded-host") ?? h.get("host");
    const cookieHost = sanitizeHost(rawHost);
    if (!cookieHost) {
      return {
        ok: false,
        code: "invalid_host",
        message: "Could not validate the host for sign-in. Please try again.",
      };
    }

    const store = await resolveStore(input.storeSlug);
    if (!store) {
      return {
        ok: false,
        code: "store_not_found",
        message: "Could not resolve this store. Please try again.",
      };
    }

    const trimmed = input.code.trim();
    if (!trimmed) {
      return {
        ok: false,
        code: "invalid_code",
        message: "Enter the 6-digit code from your authenticator app.",
      };
    }

    const outcome = await verifyCustomerTotp({
      sessionId: input.sessionId,
      sessionToken: input.sessionToken,
      code: trimmed,
    });

    let verified: { uid: string; email: string };
    switch (outcome.kind) {
      case "complete":
        verified = { uid: outcome.uid, email: outcome.email };
        break;
      case "rejected":
        return {
          ok: false,
          code: "invalid_code",
          message: "That code is incorrect. Please try again.",
        };
      case "totp_required":
        // The endpoint asked for another code — the session id/token it
        // returned may differ from the one just used, so this can't be
        // silently retried with the caller's original values. Send the
        // shopper back to re-enter a fresh code rather than guessing.
        return {
          ok: false,
          code: "invalid_code",
          message: "That code is incorrect. Please try again.",
        };
      case "handoff":
        // Same genuine dead end customerSignIn's "handoff" case is — a
        // factor this endpoint can't collect. The handoff URL is never
        // surfaced (see the comment on that case in customerSignIn).
        return {
          ok: false,
          code: "signin_method_unsupported",
          message:
            "This account uses a sign-in method this storefront can't complete yet. Please contact support for help signing in.",
        };
    }

    return await completeCustomerSignIn(store, cookieHost, input.storeSlug, verified);
  } catch (err) {
    return handleSignInError(err, "confirmCustomerTotp failed with an unexpected error");
  }
}
