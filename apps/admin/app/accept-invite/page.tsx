import { verifyInvitationToken, PlatformApiError } from "@/lib/api/platform-api";
import { AcceptInviteForm } from "@/components/auth/AcceptInviteForm";

interface PageProps {
  searchParams: Promise<{ token?: string }>;
}

/**
 * Phase P — /accept-invite?token=…
 *
 * Public page (no middleware gate). Verifies the invitation token
 * server-side and renders:
 *   - An error panel if the token is missing/invalid/expired/revoked
 *   - The AcceptInviteForm with the verified invitation context if OK
 *
 * The form handles both new-account (sign-up with email+password) and
 * existing-account (sign-in with email+password OR Google) flows, then
 * calls the accept server action which writes the FGA role tuple via
 * platform-api and switches the session onto the new tenant.
 */
export default async function AcceptInvitePage({ searchParams }: PageProps) {
  const { token = "" } = await searchParams;

  if (!token) {
    return <ErrorCard title="Missing token" message="The invite link is malformed. Ask the person who invited you to resend it." />;
  }

  let invitation;
  try {
    invitation = await verifyInvitationToken(token);
  } catch (err) {
    const { title, message } = describeError(err);
    return <ErrorCard title={title} message={message} />;
  }

  return (
    <main className="min-h-screen px-4 py-10 sm:px-6 sm:py-14">
      <div className="mx-auto grid min-h-[calc(100vh-5rem)] max-w-6xl items-center gap-8 lg:grid-cols-[minmax(0,1.05fr)_minmax(24rem,0.95fr)]">
        <section className="hidden lg:block">
          <p className="admin-eyebrow">Team invite</p>
          <h1 className="mt-4 max-w-xl font-serif text-6xl font-medium leading-none tracking-tight text-foreground">
            You&apos;re invited to join {invitation.tenant_name}.
          </h1>
          <p className="mt-6 max-w-xl text-lg leading-8 text-muted-foreground">
            Accept the invite with <strong className="font-medium text-foreground">{invitation.email}</strong> and
            we&apos;ll connect your account to the store as a {invitation.role}.
          </p>

          <div className="mt-10 grid max-w-2xl gap-4 sm:grid-cols-2">
            <div className="admin-panel rounded-[1.5rem] p-5">
              <p className="text-sm font-semibold text-foreground">Right access, right store</p>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                The invite links your account to this specific workspace without affecting any others you already use.
              </p>
            </div>
            <div className="admin-panel rounded-[1.5rem] p-5">
              <p className="text-sm font-semibold text-foreground">Fastest path in</p>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                Existing accounts can sign in immediately, and new users can create an account right from this page.
              </p>
            </div>
          </div>
        </section>

        <div className="mx-auto w-full max-w-xl">
          <header className="mb-6 text-center lg:text-left">
            <p className="admin-eyebrow lg:hidden">Team invite</p>
            <h1 className="mt-2 font-serif text-3xl font-medium tracking-tight text-foreground">
              Join {invitation.tenant_name}
            </h1>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              Accept your {invitation.role} invite with {invitation.email}.
            </p>
          </header>
          <AcceptInviteForm token={token} invitation={invitation} />
        </div>
      </div>
    </main>
  );
}

interface ErrorCardProps {
  title: string;
  message: string;
}

function ErrorCard({ title, message }: ErrorCardProps) {
  return (
    <main className="min-h-screen px-4 py-16">
      <div className="mx-auto w-full max-w-xl">
        <div className="admin-surface rounded-[2rem] border border-destructive/20 bg-destructive/5 p-6 text-center">
          <p className="admin-eyebrow">Invite status</p>
          <h1 className="mt-3 font-serif text-2xl font-medium text-foreground">
            {title}
          </h1>
          <p className="mt-3 text-sm text-muted-foreground">{message}</p>
        </div>
      </div>
    </main>
  );
}

function describeError(err: unknown): { title: string; message: string } {
  if (err instanceof PlatformApiError) {
    switch (err.code) {
      case "invitation_not_found":
        return {
          title: "Invite not found",
          message: "This invite link is no longer valid. Ask the person who invited you to resend it.",
        };
      case "invitation_expired":
        return {
          title: "Invite expired",
          message: "This invitation expired. Ask the person who invited you to resend it.",
        };
      case "invitation_accepted":
        return {
          title: "Already accepted",
          message: "This invitation has already been accepted. Sign in to continue.",
        };
      case "invitation_revoked":
        return {
          title: "Invite revoked",
          message: "This invitation was revoked. Contact the inviter if this is unexpected.",
        };
    }
  }
  return {
    title: "Something went wrong",
    message: "We couldn't verify your invitation. Please try again or contact support.",
  };
}
