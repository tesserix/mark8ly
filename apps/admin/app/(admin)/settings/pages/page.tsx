import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import { PagesList } from "@/components/settings/PagesList";
import {
  canEditSettings,
  getServerSessionContext,
  resolveStoreId,
} from "@/lib/auth/serverSession";
import { listPages, type SessionHeaders } from "@/lib/api/marketplace-api";

export const dynamic = "force-dynamic";

export default async function PagesIndexPage() {
  const { role, email, tenantId } = await getServerSessionContext();
  const canManage = canEditSettings(role);

  const storeId = await resolveStoreId();
  const session: SessionHeaders = { userId: email, tenantId };
  const pages = storeId ? await listPages(storeId, session).catch(() => []) : [];

  return (
    <AdminPage
      eyebrow="Store"
      title="Pages"
      description={
        <>
          Author About, Terms, Privacy, and other static pages that live at
          your storefront&apos;s <code>/pages/</code> URLs. Add them to your
          footer from the Branding tab.
        </>
      }
      readOnlyNotice={!canManage && role ? <ReadOnlyNotice role={role} /> : undefined}
    >
      <PagesList pages={pages} canManage={canManage} />
    </AdminPage>
  );
}
