import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { listDomains } from "@/lib/api/settings-tier2-api";
import { DomainsSettingsClient } from "@/components/settings/DomainsSettingsClient";

/**
 * /settings/domains — custom domain management.
 *
 * Server component fetches domain list from marketplace-api and renders
 * the interactive client component for add, verify, and remove actions.
 */
export default async function DomainsSettingsPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = await getServerSessionContext();

  const editable = canEditSettings(role);

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-5xl space-y-10">
        <header className="space-y-3">
          <p className="eyebrow">Settings</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            Custom domains
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Connect your own domain to your storefront. Your customers will see
            your brand, not ours.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view domain settings but cannot
              edit them.
            </p>
          )}
        </header>

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
      </div>
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
