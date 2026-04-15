import { notFound } from "next/navigation";

import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import { PageEditor } from "@/components/settings/PageEditor";
import {
  canEditSettings,
  getServerSessionContext,
  resolveStoreId,
} from "@/lib/auth/serverSession";
import { getPage, type SessionHeaders } from "@/lib/api/marketplace-api";

export const dynamic = "force-dynamic";

interface Props {
  params: Promise<{ id: string }>;
}

export default async function PageEditorRoute({ params }: Props) {
  const { id } = await params;
  const { role, userId, tenantId } = await getServerSessionContext();
  const canManage = canEditSettings(role);

  const storeId = await resolveStoreId();
  if (!storeId) notFound();

  // FGA tuples key on the Firebase UID (x-session-user-id), not email.
  const session: SessionHeaders = { userId, tenantId };
  const page = await getPage(storeId, id, session).catch(() => null);
  if (!page) notFound();

  return (
    <AdminPage
      eyebrow="Store → Pages"
      title={page.title || "Untitled page"}
      description={
        <>
          Edit the page body, slug, and SEO metadata. Unpublished drafts
          aren&apos;t visible on your storefront.
        </>
      }
      readOnlyNotice={
        !canManage && role ? <ReadOnlyNotice role={role} /> : undefined
      }
    >
      <PageEditor page={page} canManage={canManage} />
    </AdminPage>
  );
}
