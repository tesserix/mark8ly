import type { Metadata } from "next";
import { BrandBar } from "@repo/ui/brand-bar";

import { verifyInvitationToken, PlatformApiError } from "@/lib/api/platform-api";
import { AcceptInviteForm } from "@/components/auth/AcceptInviteForm";
import { publicConfig } from "@/lib/config";

export const metadata: Metadata = { title: "Accept invite" };

interface PageProps {
  searchParams: Promise<{ token?: string }>;
}

/**
 * /accept-invite?token=… — public page (no middleware gate).
 *
 * Verifies the invitation token server-side and renders either an
 * error message or the AcceptInviteForm with the verified context.
 * Under GIP the form handles new-account (sign-up) and existing-account
 * (sign-in OR Google) flows then calls the accept server action which
 * writes the FGA role tuple via platform-api and switches the session
 * onto the new tenant.
 *
 * Under Zitadel (`publicConfig.authProvider === "zitadel"`) the form
 * collects a password and hands it to platform-api, which provisions
 * the Zitadel user, the admin project grant and the FGA tuples, then
 * sends the invitee into the normal Zitadel login flow. See issue #679.
 */
export default async function AcceptInvitePage({ searchParams }: PageProps) {
  const { token = "" } = await searchParams;

  if (!token) {
    return (
      <ErrorShell
        title="Missing token"
        message="The invite link is malformed. Ask the person who invited you to resend it."
      />
    );
  }

  let invitation;
  try {
    invitation = await verifyInvitationToken(token);
  } catch (err) {
    const { title, message } = describeError(err);
    return <ErrorShell title={title} message={message} />;
  }

  return (
    <>
      <BrandBar />
      <main id="main" className="px-6 py-16 sm:py-24">
        <div className="mx-auto w-full max-w-md space-y-10">
          <header className="space-y-2">
            <p className="eyebrow">Team invite</p>
            <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground">
              Join {invitation.tenant_name}
            </h1>
            <p className="text-base leading-7 text-foreground-secondary">
              Accept your {invitation.role} invite with {invitation.email}.
            </p>
          </header>
          <AcceptInviteForm
            token={token}
            invitation={invitation}
            provider={publicConfig.authProvider}
          />
        </div>
      </main>
    </>
  );
}

function ErrorShell({ title, message }: { title: string; message: string }) {
  return (
    <>
      <BrandBar />
      <main id="main" className="px-6 py-16 sm:py-24">
        <div className="mx-auto w-full max-w-md space-y-3">
          <p className="eyebrow">Invite status</p>
          <h1 className="font-serif text-3xl font-medium tracking-tight text-foreground">
            {title}
          </h1>
          <p className="text-base leading-7 text-foreground-secondary">
            {message}
          </p>
        </div>
      </main>
    </>
  );
}

function describeError(err: unknown): { title: string; message: string } {
  if (err instanceof PlatformApiError) {
    switch (err.code) {
      case "invitation_not_found":
        return {
          title: "Invite not found",
          message:
            "This invite link is no longer valid. Ask the person who invited you to resend it.",
        };
      case "invitation_expired":
        return {
          title: "Invite expired",
          message:
            "This invitation expired. Ask the person who invited you to resend it.",
        };
      case "invitation_accepted":
        return {
          title: "Already accepted",
          message:
            "This invitation has already been accepted. Sign in to continue.",
        };
      case "invitation_revoked":
        return {
          title: "Invite revoked",
          message:
            "This invitation was revoked. Contact the inviter if this is unexpected.",
        };
    }
  }
  return {
    title: "Something went wrong",
    message:
      "We couldn't verify your invitation. Please try again or contact support.",
  };
}
