// Server component shell for /onboarding/set-password.
//
// Reads the session id from the query string, fetches the session from
// platform-api server-side (so we never trust client state for the email),
// and renders the SetPasswordForm with the verified email + business name.
// If the session is missing, expired, or not yet verified we bounce the
// user back to /onboarding to start over.

import { redirect } from "next/navigation";

import { onboarding } from "@/lib/api/platform-api";
import { SetPasswordForm } from "@/components/onboarding/SetPasswordForm";

export const metadata = { title: "Finish setup — Mark8ly" };

interface PageProps {
  searchParams: Promise<{ session?: string }>;
}

export default async function SetPasswordPage({ searchParams }: PageProps) {
  const params = await searchParams;
  const sessionId = params.session ?? "";

  if (!sessionId) {
    redirect("/onboarding");
  }

  let email = "";
  let businessName = "";
  try {
    const sess = await onboarding.getSession(sessionId);
    if (!sess.email_verified_at || sess.status === "completed") {
      redirect("/onboarding");
    }
    email = sess.email;
    businessName = sess.draft?.business_name ?? "";
  } catch {
    redirect("/onboarding");
  }

  return (
    <main className="min-h-screen bg-gradient-to-br from-warm-50 via-white to-warm-100 px-4 py-16">
      <SetPasswordForm
        sessionId={sessionId}
        email={email}
        businessName={businessName}
      />
    </main>
  );
}
