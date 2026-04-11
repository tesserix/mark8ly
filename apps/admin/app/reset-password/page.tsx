import type { Metadata } from "next";
import { BrandBar } from "@repo/ui/brand-bar";

import { ResetPasswordForm } from "@/components/auth/ResetPasswordForm";

export const metadata: Metadata = {
  title: "Reset password",
  robots: { index: false, follow: false },
};

interface ResetPasswordPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

/**
 * /reset-password — branded landing page for the password reset link
 * sent by platform-api. The previous flow sent users to Firebase's
 * hosted auth action URL, exposing the GCP project. Now the email
 * lands users here with the oobCode in the query string, and our own
 * form drives `accounts:resetPassword` via platform-api.
 */
export default async function ResetPasswordPage({
  searchParams,
}: ResetPasswordPageProps) {
  const params = await searchParams;
  const raw = params.oobCode ?? params.oob_code;
  const oobCode = typeof raw === "string" ? raw : "";

  return (
    <>
      <BrandBar />
      <main id="main" className="px-6 py-16 sm:py-24">
        <div className="mx-auto w-full max-w-md">
          <ResetPasswordForm oobCode={oobCode} />
        </div>
      </main>
    </>
  );
}
