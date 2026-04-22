import { getServerSessionContext } from "@/lib/auth/serverSession";
import { SegmentForm } from "@/components/marketing/segments/SegmentForm";
import { getLoyaltyProgram } from "@/lib/api/loyalty-api";

export default async function NewSegmentPage() {
  const session = await getServerSessionContext();
  const { tenantId, userId, currentStore } = session;

  if (!currentStore) {
    return (
      <main className="space-y-10">
        <h1 className="font-serif text-3xl font-medium text-ink-900">
          No store selected
        </h1>
        <p className="text-sm text-ink-500">
          Set up a store before creating segments.
        </p>
      </main>
    );
  }

  const program = await getLoyaltyProgram(currentStore.id, {
    userId,
    tenantId,
  });
  const tiers = program?.tiers ?? [];

  return (
    <main className="space-y-10">
      <header className="space-y-3">
        <p className="eyebrow">Marketing</p>
        <h1 className="font-serif text-5xl font-medium tracking-tight text-foreground">
          New segment
        </h1>
        <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
          Define rules to create a reusable audience segment for your
          campaigns.
        </p>
      </header>

      <SegmentForm tiers={tiers} />
    </main>
  );
}
