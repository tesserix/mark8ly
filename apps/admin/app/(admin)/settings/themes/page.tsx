import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import { StorefrontThemeForm } from "@/components/settings/StorefrontThemeForm";
import { BrandingSettingsClient } from "@/components/settings/BrandingSettingsClient";
import { PagesList } from "@/components/settings/PagesList";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getBranding, listPages, listCategories, type AdminCategory, type SessionHeaders } from "@/lib/api/marketplace-api";

export default async function ThemesSettingsPage() {
  const { tenantName, userId, role, memberships, tenantId, currentStore } =
    await getServerSessionContext();
  const editable = canEditSettings(role);

  // Fetch branding from marketplace-api (B1). FGA tuples are keyed by the
  // Firebase UID (x-session-user-id), NOT email — passing email to
  // SessionHeaders makes the Check fail and the endpoint 404s.
  let branding = null;
  let pages: Awaited<ReturnType<typeof listPages>> = [];
  let categories: AdminCategory[] = [];
  if (currentStore) {
    const session: SessionHeaders = { userId, tenantId };
    [branding, pages, categories] = await Promise.all([
      getBranding(currentStore.id, session),
      listPages(currentStore.id, session).catch(() => []),
      listCategories(currentStore.id, session).catch(() => []),
    ]);
  }

  return (
          <AdminPage
        eyebrow="Store"
        title="Branding"
        description="Identity, theme, homepage, pages, footer, and advanced options for your storefront. Changes are reflected on your live storefront immediately."
        readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
      >
        {currentStore && branding ? (
          <BrandingSettingsClient
            branding={branding}
            editable={editable}
            pages={pages}
            categories={categories}
            store={{
              name: currentStore.name,
              slug: currentStore.slug,
              countryCode: currentStore.country_code,
            }}
            themeContent={
              <StorefrontThemeForm store={currentStore} editable={editable} />
            }
            pagesContent={<PagesList pages={pages} canManage={editable} />}
          />
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
