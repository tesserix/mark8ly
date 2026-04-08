import { SignInForm } from "@/components/auth/SignInForm";

export const metadata = { title: "Sign in — Mark8ly" };

/**
 * Admin /login — Phase M.
 *
 * The returning-user funnel lives here, not on the marketing site.
 * Two paths to a session:
 *   1. Email + password — Identity Toolkit signInWithPassword + the
 *      `signIn` server action which looks up workspace_tenant by GIP
 *      UID and calls auth-bff /auth/auto-login.
 *   2. Continue with Google — gsi/client popup, exchanged via Identity
 *      Toolkit signInWithIdp, then the same server action.
 */
export default function LoginPage() {
  return (
    <main className="min-h-screen bg-gradient-to-br from-warm-50 via-white to-warm-100 px-4 py-16">
      <SignInForm />
    </main>
  );
}
