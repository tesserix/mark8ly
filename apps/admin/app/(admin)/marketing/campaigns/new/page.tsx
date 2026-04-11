import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listSegments } from "@/lib/api/campaigns-api";
import { CampaignWizard } from "@/components/marketing/campaigns/CampaignWizard";

export default async function NewCampaignPage() {
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
              <main className="space-y-10">
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl font-medium text-ink-900">
            No store selected
          </h1>
          <p className="text-sm text-ink-500">
            Set up a store before creating campaigns.
          </p>
        </main>
    );
  }

  const segmentsResult = await listSegments(currentStore.id, {
    userId,
    tenantId,
  });
  const segments = segmentsResult?.data ?? [];

  return (
          <main className="space-y-10">
        <header className="space-y-3">
          <p className="eyebrow">Marketing</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            New campaign
          </h1>
        </header>

        <CampaignWizard
          storeId={currentStore.id}
          segments={segments}
          session={{ userId, tenantId }}
        />
      </main>
  );
}
