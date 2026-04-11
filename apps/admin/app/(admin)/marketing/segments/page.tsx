import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listSegments } from "@/lib/api/campaigns-api";
import { SegmentsList } from "@/components/marketing/segments/SegmentsList";
import Link from "next/link";

export default async function SegmentsPage() {
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
              <main className="mx-auto w-full max-w-5xl space-y-10">
          <header className="space-y-3">
            <p className="eyebrow">Marketing</p>
            <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
              Segments
            </h1>
          </header>
          <div className="flex flex-col items-start gap-2 rounded-md border border-ink-200 bg-white px-6 py-12">
            <p className="text-sm text-ink-500">
              Create a store first to start managing segments.
            </p>
          </div>
        </main>
    );
  }

  const result = await listSegments(currentStore.id, { userId, tenantId });
  const segments = result?.data ?? [];

  return (
          <main className="mx-auto w-full max-w-5xl space-y-10">
        <header className="flex items-center justify-between">
          <div className="space-y-3">
            <p className="eyebrow">Marketing</p>
            <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
              Segments
            </h1>
            <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
              Define audience segments to target your campaigns precisely.
            </p>
          </div>
          {canCreate && (
            <Link
              href="/marketing/segments/new"
              className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800"
            >
              Create segment
            </Link>
          )}
        </header>

        <hr className="border-ink-200" />

        <SegmentsList
          segments={segments}
          storeId={currentStore.id}
          session={{ userId, tenantId }}
        />
      </main>
  );
}
