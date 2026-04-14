import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import {
  getLoyaltyProgram,
  getLoyaltyMembers,
  getLoyaltyReferrals,
  updateLoyaltyProgram,
} from "@/lib/api/loyalty-api";
import { ProgramConfigForm } from "@/components/marketing/loyalty/ProgramConfigForm";
import { MembersTable } from "@/components/marketing/loyalty/MembersTable";
import { ReferralsTable } from "@/components/marketing/loyalty/ReferralsTable";
import { LoyaltyTabSwitcher } from "./LoyaltyTabSwitcher";

export default async function LoyaltyPage() {
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

  return (
          <div className="space-y-10">
        <header className="space-y-3">
          <p className="eyebrow">Marketing</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            Loyalty program
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Configure your loyalty program, view members, and manage referrals.
          </p>
        </header>

        {currentStore ? (
          <LoyaltyContent
            storeId={currentStore.id}
            storeCurrency={currentStore.currency_code}
            userId={userId}
            tenantId={tenantId}
            editable={editable}
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store before configuring loyalty.
          </p>
        )}
      </div>
  );
}

async function LoyaltyContent({
  storeId,
  storeCurrency,
  userId,
  tenantId,
  editable,
}: {
  storeId: string;
  storeCurrency: string;
  userId: string;
  tenantId: string;
  editable: boolean;
}) {
  const session = { userId, tenantId };

  const [program, membersData, referralsData] = await Promise.all([
    getLoyaltyProgram(storeId, session),
    getLoyaltyMembers(storeId, session),
    getLoyaltyReferrals(storeId, session),
  ]);

  async function handleSaveProgram(data: Record<string, unknown>) {
    "use server";
    await updateLoyaltyProgram(storeId, data, session);
  }

  return (
    <LoyaltyTabSwitcher
      programTab={
        <ProgramConfigForm
          program={program}
          storeId={storeId}
          storeCurrency={storeCurrency}
          editable={editable}
          onSave={handleSaveProgram}
        />
      }
      membersTab={
        <MembersTable
          members={membersData.data}
          total={membersData.total}
        />
      }
      referralsTab={
        <ReferralsTable
          referrals={referralsData.data}
          total={referralsData.total}
        />
      }
    />
  );
}
