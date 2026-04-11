import type { Metadata } from "next";
import { BrandBar } from "@repo/ui/brand-bar";

import { ForgotPasswordForm } from "@/components/auth/ForgotPasswordForm";

export const metadata: Metadata = {
  title: "Reset password",
  robots: { index: false, follow: false },
};

/**
 * /forgot-password — password reset funnel.
 *
 * Collects the merchant's email and calls GIP's accounts:sendOobCode to
 * send a reset link. GIP hosts the reset UI at its configured action URL
 * so we don't need a separate /reset-password page.
 */
export default function ForgotPasswordPage() {
  return (
    <>
      <BrandBar />
      <main id="main" className="px-6 py-16 sm:py-24">
        <div className="mx-auto w-full max-w-md">
          <ForgotPasswordForm />
        </div>
      </main>
    </>
  );
}
