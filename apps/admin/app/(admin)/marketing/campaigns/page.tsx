import Link from "next/link";

import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listCampaigns, type ListCampaignsQuery } from "@/lib/api/campaigns-api";
import { AdminPage } from "@/components/layout";
import { CampaignsList } from "@/components/marketing/campaigns/CampaignsList";
import { CampaignsListEmpty } from "@/components/marketing/campaigns/CampaignsListEmpty";

interface CampaignsPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

function parseSearchParams(
  params: Record<string, string | string[] | undefined>,
): ListCampaignsQuery {
  const first = (v: string | string[] | undefined): string | undefined =>
    Array.isArray(v) ? v[0] : v;
  return {
    status: first(params.status),
    page: first(params.page) ? Number(first(params.page)) : undefined,
    per_page: first(params.per_page) ? Number(first(params.per_page)) : undefined,
  };
}

export default async function CampaignsPage({
  searchParams,
}: CampaignsPageProps) {
  const session = await getServerSessionContext();
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = session;

  const canCreate = role === "owner" || role === "admin";

  if (!currentStore) {
    return (
      <AdminPage eyebrow="Marketing" title="Campaigns">
        <CampaignsListEmpty variant="no-store" />
      </AdminPage>
    );
  }

  const params = await searchParams;
  const query = parseSearchParams(params);
  const result = await listCampaigns(currentStore.id, query, {
    userId,
    tenantId,
  });
  const campaigns = result?.data ?? [];
  const meta = result?.meta;

  return (
    <AdminPage
      eyebrow="Marketing"
      title="Campaigns"
      description="Reach your customers with targeted email campaigns."
      actions={
        canCreate ? (
          <Link
            href="/marketing/campaigns/new"
            className="inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Create campaign
          </Link>
        ) : undefined
      }
    >
      {campaigns.length === 0 && !query.status ? (
        <CampaignsListEmpty variant="no-campaigns" />
      ) : (
        <CampaignsList
          campaigns={campaigns}
          meta={meta}
          storeId={currentStore.id}
        />
      )}
    </AdminPage>
  );
}
