import { SignInForm } from "@/components/auth/SignInForm";

export const metadata = { title: "Sign in — Mark8ly" };

/**
 * /login — returning-user sign-in. Two paths to a session:
 *
 *   1. Email + password — uses Identity Toolkit signInWithPassword and the
 *      `signIn` server action which looks up the workspace tenant by GIP
 *      UID and calls auth-bff /auth/auto-login.
 *   2. Continue with Google — Google Identity Services popup, exchanged
 *      via Identity Toolkit signInWithIdp, then the same server action.
 *
 * Phase M.
 */
export default function LoginPage() {
  return (
    <main className="min-h-screen bg-gradient-to-br from-warm-50 via-white to-warm-100 px-4 py-16">
      <SignInForm />
    </main>
  );
}
