import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getLoyaltyMember, adjustPoints } from "@/lib/api/loyalty-api";
import { Breadcrumbs } from "@/components/layout";
import { MemberDetailPanel } from "@/components/marketing/loyalty/MemberDetailPanel";

interface MemberDetailPageProps {
  params: Promise<{ id: string }>;
}

export default async function MemberDetailPage({
  params,
}: MemberDetailPageProps) {
  const { id } = await params;
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = await getServerSessionContext();

  const editable = canEditSettings(role);

  if (!currentStore) {
    return (
              <p className="text-sm text-danger">
          No store found.
        </p>
    );
  }

  const session = { userId, tenantId };
  const result = await getLoyaltyMember(currentStore.id, id, session);

  if (!result) {
    return (
      <div className="flex flex-col gap-6">
        <Breadcrumbs
          items={[
            { label: "Marketing", href: "/marketing/campaigns" },
            { label: "Loyalty", href: "/marketing/loyalty" },
            { label: "Not found" },
          ]}
        />
        <p className="text-sm text-foreground-tertiary">Member not found.</p>
      </div>
    );
  }

  const storeId = currentStore.id;

  async function handleAdjust(
    points: number,
    description: string,
  ): Promise<boolean> {
    "use server";
    return adjustPoints(storeId, id, { points, description }, session);
  }

  const displayName =
    result.data.customer_name?.trim() ||
    result.data.customer_email ||
    "Member";

  return (
    <div className="flex flex-col gap-8">
      <Breadcrumbs
        items={[
          { label: "Marketing", href: "/marketing/campaigns" },
          { label: "Loyalty", href: "/marketing/loyalty" },
          { label: displayName },
        ]}
      />
      <MemberDetailPanel
        member={result.data}
        transactions={result.transactions}
        editable={editable}
        onAdjust={handleAdjust}
      />
    </div>
  );
}
