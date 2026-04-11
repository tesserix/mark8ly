import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getCampaign } from "@/lib/api/campaigns-api";
import { CampaignAnalytics } from "@/components/marketing/campaigns/CampaignAnalytics";
import { notFound } from "next/navigation";

interface CampaignDetailPageProps {
  params: Promise<{ id: string }>;
}

export default async function CampaignDetailPage({
  params,
}: CampaignDetailPageProps) {
  const { id } = await params;
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

  if (!currentStore) {
    return (
      <AdminShell
        tenantName={tenantName}
        userEmail={email}
        role={role}
        memberships={memberships}
        currentTenantId={tenantId}
      >
        <main className="mx-auto w-full max-w-5xl space-y-10">
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl font-medium text-ink-900">
            No store selected
          </h1>
          <p className="text-sm text-ink-500">
            Set up a store before viewing campaigns.
          </p>
        </main>
      </AdminShell>
    );
  }

  const campaign = await getCampaign(currentStore.id, id, {
    userId,
    tenantId,
  });

  if (!campaign) {
    notFound();
  }

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <main className="mx-auto w-full max-w-5xl space-y-10">
        <CampaignAnalytics
          campaign={campaign}
          storeId={currentStore.id}
          session={{ userId, tenantId }}
        />
      </main>
    </AdminShell>
  );
}
