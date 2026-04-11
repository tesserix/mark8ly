import Link from "next/link";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getLoyaltyMember, adjustPoints } from "@/lib/api/loyalty-api";
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
              <div className="mx-auto w-full max-w-5xl space-y-6">
          <Link
            href="/marketing/loyalty"
            className="text-sm text-[color:var(--moss-700)] hover:underline"
          >
            ← Back to loyalty
          </Link>
          <p className="text-sm text-[color:var(--ink-900)]/50">
            Member not found.
          </p>
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

  return (
          <div className="mx-auto w-full max-w-5xl space-y-6">
        <Link
          href="/marketing/loyalty"
          className="text-sm text-[color:var(--moss-700)] hover:underline"
        >
          ← Back to loyalty
        </Link>
        <MemberDetailPanel
          member={result.data}
          transactions={result.transactions}
          editable={editable}
          onAdjust={handleAdjust}
        />
      </div>
  );
}
