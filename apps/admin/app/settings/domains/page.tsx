import { AdminShell } from "@/components/shell/AdminShell";
import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { listDomains } from "@/lib/api/settings-tier2-api";
import { DomainsSettingsClient } from "@/components/settings/DomainsSettingsClient";

/**
 * /settings/domains — custom domain management.
 */
export default async function DomainsSettingsPage() {
  const { tenantName, email, role, memberships, tenantId, userId, currentStore } =
    await getServerSessionContext();

  const editable = canEditSettings(role);

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <AdminPage
        eyebrow="Store"
        title="Custom domains"
        description="Connect your own domain to your storefront. Your customers will see your brand, not ours."
        readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
      >
        {currentStore ? (
          <DomainsSettingsContent
            storeId={currentStore.id}
            userId={userId}
            tenantId={tenantId}
            editable={editable}
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store before configuring domains.
          </p>
        )}
      </AdminPage>
    </AdminShell>
  );
}

async function DomainsSettingsContent({
  storeId,
  userId,
  tenantId,
  editable,
}: {
  storeId: string;
  userId: string;
  tenantId: string;
  editable: boolean;
}) {
  const domains = await listDomains(storeId, { userId, tenantId });
  return <DomainsSettingsClient domains={domains} editable={editable} />;
}
