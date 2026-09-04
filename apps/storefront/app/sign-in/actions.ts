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

import { headers } from "next/headers";
import {
  GIPTokenVerificationError,
  verifyGIPIdToken,
} from "@/lib/gip/verify-id-token";
import {
  verifyCustomerCredential,
  verifyCustomerTotp,
} from "@/lib/auth/auth-bff-customer";
import { sanitizeHost } from "@/lib/host";
import { completeCustomerSignIn, resolveStore } from "@/lib/auth/customer-session";
import type { CustomerSignInResult as Result } from "@/lib/auth/customer-sign-in-result";

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

// `Result` (aliased above from `CustomerSignInResult`) and the
// `isTotpRequiredResult` guard for it live in
// @/lib/auth/customer-sign-in-result, NOT here — this file is a
// `"use server"` module, and Next.js strips any runtime export from such a
// module that isn't an async function. `isTotpRequiredResult` is a plain
// synchronous function, so it can't be exported from this file (see the
// comment in that module for the full explanation). `type`/`interface`
// exports are erased at compile time and are exempt, but the type is kept
// in the same shared module as the guard rather than split, so both
// modules can never define the shape differently.

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

/**
 * handleSignInError maps a thrown error to the shared, user-facing
 * `Result` shape. Shared by `customerSignIn` and `confirmCustomerTotp` so
 * neither path can accidentally leak an internal error string (e.g.
 * AuthBffCustomerError's `.message`, which is literally
 * "auth-bff customer endpoint error: <code> (status <n>)") to the
 * shopper. The detail is logged server-side only.
 */
function handleSignInError(err: unknown, logLabel: string): Result {
  // GIPTokenVerificationError can only be thrown by customerSignIn's GIP
  // branch (verifyGIPIdToken) — confirmCustomerTotp is Zitadel-only and
  // never throws this. Kept in the shared helper anyway rather than
  // split into two near-identical functions: one caller needing a case
  // the other never hits isn't worth forking the error-mapping logic in
  // two, and a stray GIPTokenVerificationError from confirmCustomerTotp
  // (there shouldn't ever be one) still gets a sane, non-leaking message.
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
        case "email_not_verified":
          // The password was CORRECT — auth-bff only returns this after
          // CreatePasswordSession already succeeded (see
          // auth-bff-customer.ts's parseCustomerOutcome and
          // customer_handler.go:363-369) — but the account was registered
          // and never had its email confirmed. Telling this shopper
          // "Email or password is incorrect" would be false and send them
          // into a retry loop that can never succeed: no amount of
          // password resets fixes an unverified email, and this endpoint
          // never sends a new code on its own. Their only real recovery is
          // re-running create-account, which deletes and recreates the
          // unverified registration with a fresh code — so point them
          // there explicitly instead of leaving them stuck.
          return {
            ok: false,
            code: "email_not_verified",
            message:
              "This email address hasn't been verified yet. Go to Create account to get a new verification code.",
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
        // A FRESH challenge, not a wrong code — Zitadel wants another
        // code and handed back a new sessionId/sessionToken pair.
        // Mirrors apps/admin/app/login/actions.ts's mapZitadelOutcome,
        // which returns fresh zitadelSessionId/zitadelSessionToken on a
        // repeat challenge for the same reason: the caller's original
        // pair is now stale, and silently discarding the new one here
        // (returning the old "That code is incorrect" wording) would
        // make every subsequent retry submit stale credentials that can
        // never succeed, while blaming the code the customer typed.
        return {
          ok: false,
          code: "totp_required",
          message:
            "Enter the new 6-digit code from your authenticator app to finish signing in.",
          sessionId: outcome.sessionId,
          sessionToken: outcome.sessionToken,
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
      case "email_not_verified":
        // POST /auth/customer/totp never emits this — the check that
        // produces it (customer_handler.go:363-369) runs on the login
        // path before a session/factor challenge exists at all, and this
        // function only ever runs after customerSignIn already returned
        // totp_required for a session that passed that check. Handled
        // here only so this switch stays exhaustive against the shared
        // CustomerAuthOutcome union; reuse customerSignIn's copy and
        // pointer to create-account rather than inventing a second
        // wording for a branch that can't be reached.
        return {
          ok: false,
          code: "email_not_verified",
          message:
            "This email address hasn't been verified yet. Go to Create account to get a new verification code.",
        };
    }

    return await completeCustomerSignIn(store, cookieHost, input.storeSlug, verified);
  } catch (err) {
    return handleSignInError(err, "confirmCustomerTotp failed with an unexpected error");
  }
}
