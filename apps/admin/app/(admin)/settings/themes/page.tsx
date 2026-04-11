import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import { StorefrontThemeForm } from "@/components/settings/StorefrontThemeForm";
import { BrandingSettingsClient } from "@/components/settings/BrandingSettingsClient";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getBranding, type SessionHeaders } from "@/lib/api/marketplace-api";

export default async function ThemesSettingsPage() {
  const { tenantName, email, role, memberships, tenantId, currentStore } =
    await getServerSessionContext();
  const editable = canEditSettings(role);

  // Fetch branding from marketplace-api (B1).
  let branding = null;
  if (currentStore) {
    const session: SessionHeaders = { userId: email, tenantId };
    branding = await getBranding(currentStore.id, session);
  }

  return (
          <AdminPage
        eyebrow="Store"
        title="Themes & branding"
        description="Define your store identity, color palette, typography, layout, and footer. Changes are reflected on your live storefront immediately."
        maxWidth="lg"
        readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
      >
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
      </AdminPage>
  );
}
