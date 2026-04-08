import type { Metadata } from "next";
import { BrandBar } from "@repo/ui/brand-bar";

import { SignInForm } from "@/components/auth/SignInForm";

export const metadata: Metadata = { title: "Sign in" };

/**
 * Admin /login — returning-user funnel.
 *
 * Two paths to a session:
 *   1. Email + password — Identity Toolkit signInWithPassword + the
 *      `signIn` server action which looks up workspace_tenant by GIP
 *      UID and calls auth-bff /auth/auto-login.
 *   2. Continue with Google — gsi/client popup, exchanged via Identity
 *      Toolkit signInWithIdp, then the same server action.
 *
 * Wrapped in the slim BrandBar shell — no marketing rail, no
 * highlights grid, just the form. Trust the form, trust the brand.
 */
export default function LoginPage() {
  return (
    <>
      <BrandBar />
      <main id="main" className="px-6 py-16 sm:py-24">
        <div className="mx-auto w-full max-w-md">
          <SignInForm />
        </div>
      </main>
    </>
  );
}
