// The `Result` type returned by both `customerSignIn` and
// `confirmCustomerTotp` (apps/storefront/app/sign-in/actions.ts), plus the
// `isTotpRequiredResult` type guard for it.
//
// This lives OUTSIDE actions.ts deliberately. actions.ts is a `"use server"`
// module, and Next.js requires every runtime export of a `"use server"`
// file to be an async function — a plain synchronous function like
// `isTotpRequiredResult` gets stripped from the built server-actions
// module, so importing it from a client component (CustomerSignInForm.tsx)
// resolves to `undefined` at runtime even though it type-checks and its
// unit tests pass. `type`/`interface` exports are erased at compile time
// and are exempt from this rule, so `Result`/`TotpRequiredResult` could in
// principle stay in actions.ts — they're re-exported from here anyway so
// both modules share one definition instead of duplicating the shape.
//
// See: https://nextjs.org/docs/app/api-reference/directives/use-server

export type TotpRequiredResult = {
  ok: false;
  code: "totp_required";
  message: string;
  sessionId: string;
  sessionToken: string;
};

export type CustomerSignInResult =
  | { ok: true }
  | { ok: false; code: string; message: string }
  | TotpRequiredResult;

/**
 * isTotpRequiredResult narrows a `CustomerSignInResult` to the
 * totp_required variant.
 *
 * `code` can't be used as an automatic discriminant here — the plain
 * failure variant types `code` as `string`, not a set of literals, which
 * disqualifies it from TypeScript's discriminated-union narrowing (an
 * equality check like `result.code === "totp_required"` type-checks but
 * does not narrow `result` itself). Callers (the sign-in form,
 * corresponding tests) use this instead of that equality check.
 */
export function isTotpRequiredResult(
  r: CustomerSignInResult,
): r is TotpRequiredResult {
  return !r.ok && r.code === "totp_required";
}
