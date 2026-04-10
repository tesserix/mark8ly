import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listCampaigns, type ListCampaignsQuery } from "@/lib/api/campaigns-api";
import { CampaignsList } from "@/components/marketing/campaigns/CampaignsList";
import { CampaignsListEmpty } from "@/components/marketing/campaigns/CampaignsListEmpty";
import Link from "next/link";

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
      <AdminShell
        tenantName={tenantName}
        userEmail={email}
        role={role}
        memberships={memberships}
        currentTenantId={tenantId}
      >
        <main className="mx-auto w-full max-w-5xl space-y-10 px-8 py-8">
          <header className="space-y-3">
            <p className="eyebrow">Marketing</p>
            <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
              Campaigns
            </h1>
          </header>
          <CampaignsListEmpty variant="no-store" />
        </main>
      </AdminShell>
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
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <main className="mx-auto w-full max-w-5xl space-y-10 px-8 py-8">
        <header className="flex items-center justify-between">
          <div className="space-y-3">
            <p className="eyebrow">Marketing</p>
            <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
              Campaigns
            </h1>
            <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
              Reach your customers with targeted email campaigns.
            </p>
          </div>
          {canCreate && (
            <Link
              href="/marketing/campaigns/new"
              className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800"
            >
              Create campaign
            </Link>
          )}
        </header>

        <hr className="border-ink-200" />

        {campaigns.length === 0 && !query.status ? (
          <CampaignsListEmpty variant="no-campaigns" />
        ) : (
          <CampaignsList
            campaigns={campaigns}
            meta={meta}
            storeId={currentStore.id}
          />
        )}
      </main>
    </AdminShell>
  );
}
