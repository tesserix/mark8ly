import { z } from "zod";

/**
 * The password rules Zitadel actually enforces, mirrored here so the
 * browser can say "no" before the network does.
 *
 * # Why this file exists
 *
 * A merchant accepted a staff invitation with an eleven-character
 * password. That form's schema said `min(8)`, so it submitted happily;
 * Zitadel's policy requires twelve, so provisioning failed, and the
 * invitee was shown an opaque "we couldn't finish setting up your
 * account". They retried the same password. Onboarding's set-password
 * form had the identical `min(8)` and the identical failure ahead of it
 * (#685) — with no fallback account, since this is the merchant's first.
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
 * Schema for a password the merchant is CHOOSING (it will create, or be
 * checked against, a Zitadel account).
 *
 * Not for the GIP path, whose own minimum is 8 and which this file must
 * not tighten: the GIP flow is the fallback and stays byte-identical.
 * See SetPasswordForm for where that branch is made.
 *
 * A verbatim copy of apps/admin/lib/auth/password-policy.ts. The two
 * apps cannot share a module (separate Next builds, no shared auth
 * package) and the alternative — onboarding silently using a looser
 * rule than accept-invite — is the exact class of bug this file was
 * written for. Its Go counterpart is
 * services/platform-api/internal/zitadeladmin/password_policy.go; all
 * three move together.
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
