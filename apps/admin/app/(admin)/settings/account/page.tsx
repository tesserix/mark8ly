import { AppStoreBadges } from "@repo/ui/app-store-badges";

import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import {
  getAccountProfile,
  listAccountSessions,
} from "@/lib/api/settings-tier2-api";
import { AccountSettingsClient } from "@/components/settings/AccountSettingsClient";

/**
 * /settings/account — account profile, MFA, sessions, and danger zone.
 */
export default async function AccountSettingsPage() {
  const { email, role, tenantId, userId } = await getServerSessionContext();

  const editable = canEditSettings(role);

  return (
          <AdminPage
        eyebrow="Account"
        title="Account"
        description="Manage your personal profile, security settings, and active sessions."
        readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
      >
        <AccountSettingsContent
          userId={userId}
          tenantId={tenantId}
          sessionEmail={email}
          editable={editable}
          isOwner={role === "owner"}
        />

        {/* Permanent, quiet entry point — the mobile app is a personal tool,
            so it belongs with the merchant's own settings rather than the
            store's. No dismissal state: this one is meant to stay findable. */}
        <section className="border-t border-border-subtle pt-10">
          <p className="eyebrow mb-3">On your phone</p>
          <h2 className="font-serif text-lg font-medium text-foreground">
            Mark8ly Admin app
          </h2>
          <p className="mt-2 max-w-prose text-sm leading-relaxed text-foreground-secondary">
            Confirm orders, check stock, and answer customers on the go. Sign in
            with this same account.
          </p>
          <AppStoreBadges className="mt-5" height={36} />
        </section>
      </AdminPage>
  );
}

async function AccountSettingsContent({
  userId,
  tenantId,
  sessionEmail,
  editable,
  isOwner,
}: {
  userId: string;
  tenantId: string;
  sessionEmail: string;
  editable: boolean;
  isOwner: boolean;
}) {
  const session = { userId, tenantId };
  const [profile, sessions] = await Promise.all([
    getAccountProfile(session),
    listAccountSessions(session),
  ]);

  return (
    <AccountSettingsClient
      profile={profile}
      sessionEmail={sessionEmail}
      sessions={sessions}
      editable={editable}
      isOwner={isOwner}
    />
  );
}
