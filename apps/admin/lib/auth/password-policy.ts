import { z } from "zod";

/**
 * The password rules Zitadel actually enforces, mirrored here so the
 * browser can say "no" before the network does.
 *
 * # Why this file exists
 *
 * A merchant accepted a staff invitation with `Test@123_01` — eleven
 * characters. This form's schema said `min(8)`, so it submitted happily;
 * Zitadel's policy requires twelve, so provisioning failed, and the
 * invitee was shown "we couldn't finish setting up your account — please
 * try the invitation link again". They retried the same password.
 *
 * A client rule that is LOOSER than the server's is worse than no client
 * rule at all: it promises an acceptance the server will refuse.
 *
 * # Source of truth
 *
 * The live instance policy is the authority:
 * `GET /management/v1/policies/password/complexity`, org
 * 386377229942128837 — PROBED 2026-09-05:
 *
 *     minLength: 12, hasUppercase, hasLowercase, hasNumber, hasSymbol
 *
 * These constants are a copy of it, and the ONLY copy on the TypeScript
 * side — the schema, the helper text under the field, and the tests all
 * read them from here. Its counterpart is
 * `services/platform-api/internal/zitadeladmin/password_policy.go`; the
 * two cannot share a literal across the language boundary, so they share
 * a comment instead. Change the org policy and both must move.
 *
 * The policy is deliberately NOT fetched at runtime. Zitadel remains the
 * enforcer either way, the server now returns the specific rule that was
 * broken when the two ever disagree, and an extra round trip on page
 * load to learn a number that changes approximately never is not a trade
 * worth making.
 */
export const PASSWORD_MIN_LENGTH = 12;

/**
 * The whole policy in one sentence, shown under the field BEFORE the
 * first submit. Discoverability is the point: a merchant choosing a
 * password should not have to guess and be corrected one rule at a time.
 */
export const PASSWORD_REQUIREMENTS_TEXT = `At least ${PASSWORD_MIN_LENGTH} characters, with an uppercase letter, a lowercase letter, a number, and a symbol.`;

/**
 * Every rule reports the SAME message — the full requirements — rather
 * than the first one that failed. Reporting only "needs a number" invites
 * the user to add a number and then be told about the symbol, which is
 * the drip-feed the incident was made of.
 */
export const PASSWORD_REQUIREMENTS_MESSAGE = `Password must be at least ${PASSWORD_MIN_LENGTH} characters, with an uppercase letter, a lowercase letter, a number, and a symbol.`;

/**
 * A symbol is anything that is not a letter or a digit, which is how
 * Zitadel's `hasSymbol` behaves. Kept broad on purpose: a client rule
 * narrower than the server's would reject a password Zitadel accepts.
 */
const SYMBOL = /[^A-Za-z0-9]/;

/**
 * Schema for a password the invitee is CHOOSING (it will create, or be
 * checked against, a Zitadel account).
 *
 * Not for a password being typed to sign in to an existing GIP account:
 * GIP's own minimum is 8, and holding a legacy 8-character password to
 * this standard would lock a legitimate user out of their own invite.
 * See AcceptInviteForm for where that branch is made.
 */
export const newPasswordSchema = z
  .string()
  .min(PASSWORD_MIN_LENGTH, PASSWORD_REQUIREMENTS_MESSAGE)
  .regex(/[A-Z]/, PASSWORD_REQUIREMENTS_MESSAGE)
  .regex(/[a-z]/, PASSWORD_REQUIREMENTS_MESSAGE)
  .regex(/[0-9]/, PASSWORD_REQUIREMENTS_MESSAGE)
  .regex(SYMBOL, PASSWORD_REQUIREMENTS_MESSAGE);

/**
 * Imperative form of the schema, for callers that want a message or
 * null rather than a Zod result.
 */
export function validateNewPassword(password: string): string | null {
  const result = newPasswordSchema.safeParse(password);
  return result.success
    ? null
    : (result.error.issues[0]?.message ?? PASSWORD_REQUIREMENTS_MESSAGE);
}
