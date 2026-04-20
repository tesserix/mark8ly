import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getCampaign } from "@/lib/api/campaigns-api";
import { Breadcrumbs } from "@/components/layout";
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
      <main className="flex flex-col gap-6">
        <h1 className="font-serif text-3xl font-medium tracking-tight text-foreground">
          No store selected
        </h1>
        <p className="text-sm text-foreground-secondary">
          Set up a store before viewing campaigns.
        </p>
      </main>
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
    <main className="flex flex-col gap-10">
      <Breadcrumbs
        items={[
          { label: "Marketing", href: "/marketing/campaigns" },
          { label: "Campaigns", href: "/marketing/campaigns" },
          { label: campaign.name || "Campaign" },
        ]}
      />
      <CampaignAnalytics campaign={campaign} />
    </main>
  );
}
