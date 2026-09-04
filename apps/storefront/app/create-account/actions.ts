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
import { headers } from "next/headers";
import { sanitizeHost } from "@/lib/host";

// Re-exported so callers that only import from this module (the form)
// don't also need to reach into lib/auth/create-account-flow for the
// result shape registerCustomer returns. `type` exports are erased at
// compile time and are exempt from the async-only rule above.
export type { RegisterCustomerResult };

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
  try {
    const outcome = await registerCustomerAccount({
      email: input.email,
      password: input.password,
    });
    if (outcome.kind === "created") {
      return { ok: true, uid: outcome.uid, email: outcome.email };
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
   * Not independently re-verified against Zitadel here: the account
   * behind `uid` was created moments earlier by THIS SAME client's
   * registerCustomer call using exactly this address, so trusting it back
   * grants no capability the client didn't already have at register time
   * — it is used only to attach an email to the session cookie and to
   * key the best-effort loyalty enrollment inside completeCustomerSignIn,
   * never to decide whether verification succeeded (that is `code`'s
   * job, checked against `uid` by Zitadel itself).
   */
  email: string;
  /** The 6-character code the shopper read out of their verification
   *  email. Never logged — see verifyCustomerEmailCode's doc. */
  code: string;
  storeSlug: string;
}

/**
 * verifyCustomerEmail is step 2: it submits the code, and ONLY on a
 * verified outcome mints the session via completeCustomerSignIn — the
 * same tail every other sign-in path in this app shares. No cookie is
 * set on any failure branch below, including a wrong/expired code, an
 * unresolvable host/store, or an upstream failure.
 */
export async function verifyCustomerEmail(
  input: VerifyCustomerEmailInput,
): Promise<Result> {
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
