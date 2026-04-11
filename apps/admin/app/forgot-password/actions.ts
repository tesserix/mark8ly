"use server";

// Server action bridge for the forgot-password funnel. Lives here so
// the form in components/auth/ForgotPasswordForm.tsx stays a client
// component while still calling into the in-cluster platform-api
// (which isn't exposed to the browser).

import { requestPasswordReset } from "@/lib/api/platform-api";

export interface ForgotPasswordResult {
  ok: boolean;
  message: string;
}

export async function requestPasswordResetAction(
  email: string,
): Promise<ForgotPasswordResult> {
  if (!email) {
    return { ok: false, message: "Please enter your email address." };
  }
  const result = await requestPasswordReset(email);
  if (result.ok) {
    return { ok: true, message: "" };
  }
  return { ok: false, message: result.message };
}
