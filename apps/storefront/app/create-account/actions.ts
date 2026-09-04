"use server";

// Server actions for storefront customer account creation.
//
// GIP path (flag unset): UNCHANGED, byte-identical to before phase 6a
// task 3. The browser calls Identity Toolkit's accounts:signUp directly
// (components/auth/CreateAccountForm.tsx) and customerSignUp below still
// just delegates to customerSignIn — by the time this runs the GIP
// account already exists, so signing in and signing up are the same
// operation once the account is there.
//
// Zitadel path (NEXT_PUBLIC_AUTH_PROVIDER === "zitadel"): a real two-step
// sign-up, because an email/password account created against Zitadel
// starts life UNVERIFIED (see services/auth-bff's customer_handler.go).
// Why that verification step exists at all — and why it is not
// decorative — matters for the copy this file returns: the storefront's
// Google-through-Zitadel path (auth/idp/finish/route.ts) deliberately
// refuses to link a Google identity to an account whose email is
// unverified. Skip verification here and every shopper who later clicks
// "Continue with Google" would be permanently locked out of linking that
// address. So:
//
//   1. registerCustomer  -> POST /auth/customer/register (creates the
//      account, emails a 6-character code; the code itself never leaves
//      auth-bff — see lib/auth/auth-bff-customer.ts's
//      registerCustomerAccount doc).
//   2. The form shows a "check your email" step.
//   3. verifyCustomerEmail -> POST /auth/customer/verify-email (submits
//      the code) and, ONLY on success, mints the session via the shared
//      completeCustomerSignIn tail — the SAME function customerSignIn,
//      confirmCustomerTotp, and the Google idp/finish route all call.
//      There is exactly one cookie-minting path in this app; this file
//      does not add a second.
//
// Every export below is async — this is a `"use server"` module, and
// Next.js silently strips any non-async runtime export from one (see
// lib/auth/customer-sign-in-result.ts's file header for the full
// explanation of why that rule bit this repo before). The
// CreateAccountStep/CreateAccountFormState/RegisterCustomerResult types
// and the pure applyRegisterResult/applyVerifyResult state-transition
// functions the form uses accordingly live in
// @/lib/auth/create-account-flow, not here.

import { customerSignIn } from "@/app/sign-in/actions";
import {
  AuthBffCustomerError,
  registerCustomerAccount,
  verifyCustomerEmailCode,
} from "@/lib/auth/auth-bff-customer";
import { customerSignupErrorMessage } from "@/lib/auth/customer-signup-messages";
import type { RegisterCustomerResult } from "@/lib/auth/create-account-flow";
import { completeCustomerSignIn, resolveStore } from "@/lib/auth/customer-session";
import type { CustomerSignInResult as Result } from "@/lib/auth/customer-sign-in-result";
import { signPendingSignup, verifyPendingSignup } from "@/lib/auth/pending-signup-token";
import { headers } from "next/headers";
import { sanitizeHost } from "@/lib/host";

// Re-exported so callers that only import from this module (the form)
// don't also need to reach into lib/auth/create-account-flow for the
// result shape registerCustomer returns. `type` exports are erased at
// compile time and are exempt from the async-only rule above.
export type { RegisterCustomerResult };

// Matches apps/storefront/app/sign-in/actions.ts's AUTH_PROVIDER rule
// exactly — see that file's comment. registerCustomer/verifyCustomerEmail
// below refuse to do anything under any other value, mirroring
// app/auth/idp/finish/route.ts's flag guard (that route 404s outright;
// these are server actions rather than a route, so they return a plain
// failure result instead). This is defense in depth, not the fix for the
// email-trust issue below — Next.js still registers both actions as
// callable server actions regardless of this flag, and today the ONLY
// thing standing between an unflagged storefront and a live register/
// verify-email call is auth-bff's own ZITADEL_ENABLED gate. The guard
// just means a storefront left on GIP never forwards a call it has no
// business making, independent of what the backend happens to have
// mounted.
const AUTH_PROVIDER: "gip" | "zitadel" =
  process.env.NEXT_PUBLIC_AUTH_PROVIDER === "zitadel" ? "zitadel" : "gip";

const NOT_AVAILABLE_MESSAGE = "Account creation is not available right now. Please try again later.";

interface CustomerSignUpInput {
  idToken: string;
  uid: string;
  storeSlug: string;
}

/**
 * customerSignUp is the GIP-path action, called only when
 * NEXT_PUBLIC_AUTH_PROVIDER !== "zitadel". Delegates to the sign-in
 * action — the flow is identical once the GIP account exists. The
 * sign-in action sets the cookie and registers the customer profile in
 * marketplace-api. UNCHANGED from before phase 6a task 3.
 */
export async function customerSignUp(input: CustomerSignUpInput): Promise<Result> {
  return customerSignIn(input);
}

interface RegisterCustomerInput {
  email: string;
  password: string;
}

/**
 * registerCustomer is step 1 of the Zitadel sign-up flow: it creates the
 * (unverified) account and triggers auth-bff's verification email. It
 * mints no cookie under any outcome — there is nothing to mint a session
 * for until the address is verified.
 */
export async function registerCustomer(
  input: RegisterCustomerInput,
): Promise<RegisterCustomerResult> {
  if (AUTH_PROVIDER !== "zitadel") {
    return { ok: false, code: "not_available", message: NOT_AVAILABLE_MESSAGE };
  }
  try {
    const outcome = await registerCustomerAccount({
      email: input.email,
      password: input.password,
    });
    if (outcome.kind === "created") {
      // Sign {uid, email} together NOW, from the values THIS server just
      // got back from Zitadel — never from anything the client sends.
      // verifyCustomerEmail below refuses to proceed unless this exact
      // token comes back with an exactly matching {uid, email}. See
      // @/lib/auth/pending-signup-token's file header for the
      // account-takeover this closes.
      const token = signPendingSignup(outcome.uid, outcome.email);
      return { ok: true, uid: outcome.uid, email: outcome.email, token };
    }
    return {
      ok: false,
      code: outcome.code,
      message: customerSignupErrorMessage(outcome.code),
    };
  } catch (err) {
    if (err instanceof AuthBffCustomerError) {
      console.error("registerCustomer: auth-bff call failed", err.code);
    } else {
      console.error("registerCustomer: unexpected error", err);
    }
    return {
      ok: false,
      code: "zitadel_unavailable",
      message: customerSignupErrorMessage("zitadel_unavailable"),
    };
  }
}

interface VerifyCustomerEmailInput {
  uid: string;
  /**
   * The email registerCustomer's own response returned for this uid.
   *
   * SECURITY: this value is client-supplied and, on its own, UNTRUSTED —
   * POST /auth/customer/verify-email's 2xx body carries no email for this
   * function to check it against, so without `token` below an attacker
   * could register their own address (a genuine uid + code, delivered to
   * their own inbox — Zitadel's verify checks only {uid, code}, never
   * email) and then replay this call with a victim's email instead,
   * minting a session — and upserting a marketplace-api customer profile
   * — under the victim's identity with no interaction from the victim at
   * all. `token` is what stops that: see the check below and
   * @/lib/auth/pending-signup-token's file header.
   */
  email: string;
  /**
   * The HMAC token registerCustomer returned alongside {uid, email}
   * (@/lib/auth/pending-signup-token). Proves `email` above is exactly
   * what THIS server generated it for at register time, not a value the
   * client substituted afterwards.
   */
  token: string;
  /** The 6-character code the shopper read out of their verification
   *  email. Never logged — see verifyCustomerEmailCode's doc. */
  code: string;
  storeSlug: string;
}

/**
 * verifyCustomerEmail is step 2: it submits the code, and ONLY on a
 * verified outcome mints the session via completeCustomerSignIn — the
 * same tail every other sign-in path in this app shares. No cookie is
 * set on any failure branch below, including a wrong/expired code, a
 * tampered {uid, email, token} triple, an unresolvable host/store, or an
 * upstream failure.
 */
export async function verifyCustomerEmail(
  input: VerifyCustomerEmailInput,
): Promise<Result> {
  if (AUTH_PROVIDER !== "zitadel") {
    return { ok: false, code: "not_available", message: NOT_AVAILABLE_MESSAGE };
  }
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

    // Refuse BEFORE spending a Zitadel call or looking at the code at all:
    // if {uid, email} doesn't match what registerCustomer actually signed,
    // `email` cannot be trusted for anything downstream — completeCustomerSignIn
    // would otherwise mint a session under whatever address the caller
    // put here. Logged as a tamper signal, never with the code or token
    // value itself.
    if (!verifyPendingSignup(input.uid, input.email, input.token)) {
      console.error(
        "verifyCustomerEmail: rejected — {uid, email} did not match the signed pending-signup token",
      );
      return {
        ok: false,
        code: "invalid_request",
        message: customerSignupErrorMessage("invalid_request"),
      };
    }

    const outcome = await verifyCustomerEmailCode({ uid: input.uid, code: input.code });
    if (outcome.kind !== "verified") {
      return {
        ok: false,
        code: outcome.code,
        message: customerSignupErrorMessage(outcome.code),
      };
    }

    return await completeCustomerSignIn(store, cookieHost, input.storeSlug, {
      uid: input.uid,
      email: input.email,
    });
  } catch (err) {
    if (err instanceof AuthBffCustomerError) {
      console.error("verifyCustomerEmail: auth-bff call failed", err.code);
    } else {
      console.error("verifyCustomerEmail: unexpected error", err);
    }
    return {
      ok: false,
      code: "zitadel_unavailable",
      message: customerSignupErrorMessage("zitadel_unavailable"),
    };
  }
}
