import { notFound } from "next/navigation";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { SegmentForm } from "@/components/marketing/segments/SegmentForm";
import { getSegment } from "@/lib/api/campaigns-api";
import { getLoyaltyProgram } from "@/lib/api/loyalty-api";

interface EditSegmentPageProps {
  params: Promise<{ id: string }>;
}

export default async function EditSegmentPage({
  params,
}: EditSegmentPageProps) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const { tenantId, userId, currentStore } = session;

  if (!currentStore) {
    return (
      <main className="space-y-10">
        <h1 className="font-serif text-3xl font-medium text-ink-900">
          No store selected
        </h1>
        <p className="text-sm text-ink-500">
          Set up a store before editing segments.
        </p>
      </main>
    );
  }

  const sessionHeaders = { userId, tenantId };
  const [segment, program] = await Promise.all([
    getSegment(currentStore.id, id, sessionHeaders),
    getLoyaltyProgram(currentStore.id, sessionHeaders),
  ]);

  if (!segment) {
    notFound();
  }

  const tiers = program?.tiers ?? [];

  return (
    <main className="space-y-10">
      <header className="space-y-3">
        <p className="eyebrow">Marketing</p>
        <h1 className="font-serif text-5xl font-medium tracking-tight text-foreground">
          Edit segment
        </h1>
        <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
          Update the name, description, or rules for this audience segment.
        </p>
      </header>

      <SegmentForm tiers={tiers} initialSegment={segment} />
    </main>
  );
}
