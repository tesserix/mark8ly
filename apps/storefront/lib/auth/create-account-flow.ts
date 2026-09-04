// Pure state-transition logic for the Zitadel-flagged create-account flow
// (components/auth/CreateAccountForm.tsx): register -> "check your email"
// step -> verify -> completeCustomerSignIn.
//
// This lives under lib/**, not in the component, for the same reason
// lib/auth/provider.ts and lib/auth/customer-sign-in-result.ts do (see
// their file headers): apps/storefront's vitest config only covers
// lib/**/*.{test,ts} and app/**/*.{test,ts} — components/** is not
// included at all — so any behaviour worth pinning with a test has to live
// here, with the component reduced to wiring these functions to React
// state and calling the two server actions.
//
// In particular: "a wrong verification code must keep the shopper on the
// verify step with their progress intact" (the phase brief) is a state
// transition, not a rendering detail, and applyVerifyResult below is what
// makes that pinnable outside a component test.

import type { CustomerSignInResult } from "./customer-sign-in-result";

/** Mirrors app/create-account/actions.ts's RegisterCustomerResult. Declared
 *  here (rather than imported from that `"use server"` module) so this
 *  file has no dependency on app/** — actions.ts imports and re-exports
 *  this type instead, the same split customer-sign-in-result.ts already
 *  uses for CustomerSignInResult. */
export type RegisterCustomerResult =
  | { ok: true; uid: string; email: string; token: string }
  | { ok: false; code: string; message: string };

export type CreateAccountStep =
  | { kind: "form" }
  | {
      kind: "verify";
      /** The trusted uid/email register's own response returned — never
       *  user-editable, carried through only so verifyCustomerEmail (and,
       *  on success, the session cookie) has an email to attach without a
       *  second round trip. */
      uid: string;
      email: string;
      /**
       * The HMAC token registerCustomer signed over exactly this
       * {uid, email} pair (see @/lib/auth/pending-signup-token).
       * verifyCustomerEmail refuses to mint a session unless this token
       * verifies against the {uid, email} it receives — see that
       * action's doc for the account-takeover this closes. Carried
       * through unmodified; the form never edits it.
       */
      token: string;
    };

export interface CreateAccountFormState {
  step: CreateAccountStep;
  error: string | null;
}

export const INITIAL_CREATE_ACCOUNT_STATE: CreateAccountFormState = {
  step: { kind: "form" },
  error: null,
};

/**
 * applyRegisterResult folds a registerCustomer result into form state: a
 * success moves to the verify step (carrying the server-trusted uid/
 * email); ANY failure — email_taken, weak_password, zitadel_unavailable,
 * whatever — stays on the form step with that outcome's own message. It
 * never partially advances: there is no uid/email to carry forward on a
 * failure to begin with.
 */
export function applyRegisterResult(
  result: RegisterCustomerResult,
): CreateAccountFormState {
  if (result.ok) {
    return {
      step: { kind: "verify", uid: result.uid, email: result.email, token: result.token },
      error: null,
    };
  }
  return { step: { kind: "form" }, error: result.message };
}

/**
 * applyVerifyResult folds a verifyCustomerEmail result into form state,
 * given the verify step it was called from. A success clears the error
 * (the caller is expected to navigate away — completeCustomerSignIn has
 * already minted the session by this point, so there is no further step to
 * render). ANY failure — a wrong/expired code, zitadel_unavailable, an
 * expired host/store lookup, whatever — returns the IDENTICAL verify step
 * object (same uid, same email) with only the error message replaced, so
 * the shopper stays exactly where they were and can simply retry the code
 * without losing their place or being sent back to the account-creation
 * form. Only a fresh applyRegisterResult can ever produce a "form" step
 * again.
 */
export function applyVerifyResult(
  step: Extract<CreateAccountStep, { kind: "verify" }>,
  result: CustomerSignInResult,
): CreateAccountFormState {
  if (result.ok) {
    return { step, error: null };
  }
  return { step, error: result.message };
}
