"use server";

// Server action bridge for the branded reset-password page. Reads the
// oob code from the form and proxies to platform-api's
// /internal/auth/password-reset/confirm endpoint, which calls GIP's
// public resetPassword endpoint on our behalf.

import { confirmPasswordReset } from "@/lib/api/platform-api";
import { publicConfig } from "@/lib/config";
import { validateNewPassword } from "@/lib/auth/password-policy";

export type ResetPasswordResult =
  | { ok: true }
  | { ok: false; message: string; code?: string };

export async function confirmPasswordResetAction(
  oobCode: string,
  newPassword: string,
): Promise<ResetPasswordResult> {
  const trimmedCode = oobCode.trim();
  const trimmedPassword = newPassword.trim();
  if (!trimmedCode) {
    return {
      ok: false,
      code: "missing_code",
      message:
        "This reset link is missing its code. Request a new one from the forgot-password page.",
    };
  }
  // Validate against the provider's ACTUAL policy. Zitadel requires 12
  // chars plus upper/lower/number/symbol; claiming 8 here produced the
  // dead end in #695 — the server rejected an 8-character password and
  // this action answered "must be at least 8 characters", i.e. telling
  // the user to do what they had just done. GIP's own minimum is 8, so
  // the old bound stays correct on that path.
  const policyError = publicConfig.authProvider === "zitadel"
    ? validateNewPassword(trimmedPassword)
    : trimmedPassword.length < 8
      ? "Password must be at least 8 characters."
      : null;
  if (policyError) {
    return { ok: false, code: "weak_password", message: policyError };
  }

  const result = await confirmPasswordReset(trimmedCode, trimmedPassword);
  if (result.ok) {
    return { ok: true };
  }
  return { ok: false, code: result.code, message: result.message };
}
