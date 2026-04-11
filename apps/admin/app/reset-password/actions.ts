"use server";

// Server action bridge for the branded reset-password page. Reads the
// oob code from the form and proxies to platform-api's
// /internal/auth/password-reset/confirm endpoint, which calls GIP's
// public resetPassword endpoint on our behalf.

import { confirmPasswordReset } from "@/lib/api/platform-api";

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
  if (trimmedPassword.length < 8) {
    return {
      ok: false,
      code: "weak_password",
      message: "Password must be at least 8 characters.",
    };
  }

  const result = await confirmPasswordReset(trimmedCode, trimmedPassword);
  if (result.ok) {
    return { ok: true };
  }
  return { ok: false, code: result.code, message: result.message };
}
