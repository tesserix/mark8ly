import { AdminShell } from "@/components/shell/AdminShell";
import { StorefrontThemeForm } from "@/components/settings/StorefrontThemeForm";
import { BrandingSettingsClient } from "@/components/settings/BrandingSettingsClient";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getBranding, type SessionHeaders } from "@/lib/api/marketplace-api";

export default async function StorefrontSettingsPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    currentStore,
  } = await getServerSessionContext();
  const editable = canEditSettings(role);

  // Fetch branding from marketplace-api (B1).
  let branding = null;
  if (currentStore) {
    const session: SessionHeaders = {
      userId: email,
      tenantId,
    };
    branding = await getBranding(currentStore.id, session);
  }

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-6xl space-y-12">
        <header className="space-y-3">
          <p className="eyebrow">Themes</p>
          <h1 className="font-serif text-5xl font-medium tracking-tight text-foreground">
            Themes &amp; branding
          </h1>
          <p className="max-w-3xl text-base leading-7 text-foreground-secondary">
            Define your store identity, color palette, typography, layout, and
            footer. Changes are reflected on your live storefront immediately.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view storefront settings but
              cannot publish changes.
            </p>
          )}
        </header>

        {currentStore && branding ? (
          <BrandingSettingsClient branding={branding} editable={editable} />
        ) : currentStore ? (
          <StorefrontThemeForm store={currentStore} editable={editable} />
        ) : (
          <p className="text-sm text-danger">
            We couldn&apos;t load the current store. Please refresh, or contact
            support if the problem persists.
          </p>
        )}
      </div>
    </AdminShell>
  );
}
