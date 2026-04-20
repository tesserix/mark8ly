import Link from "next/link";

import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listSegments } from "@/lib/api/campaigns-api";
import { AdminPage } from "@/components/layout";
import { SegmentsList } from "@/components/marketing/segments/SegmentsList";

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
      <AdminPage eyebrow="Marketing" title="Segments">
        <div className="flex flex-col items-start gap-2 border border-border-subtle bg-[color:var(--background-elevated)] px-6 py-12">
          <p className="text-sm text-foreground-secondary">
            Create a store first to start managing segments.
          </p>
        </div>
      </AdminPage>
    );
  }

  const result = await listSegments(currentStore.id, { userId, tenantId });
  const segments = result?.data ?? [];

  return (
    <AdminPage
      eyebrow="Marketing"
      title="Segments"
      description="Define audience segments to target your campaigns precisely."
      actions={
        canCreate ? (
          <Link
            href="/marketing/segments/new"
            className="inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Create segment
          </Link>
        ) : undefined
      }
    >
      <SegmentsList
        segments={segments}
        storeId={currentStore.id}
        session={{ userId, tenantId }}
      />
    </AdminPage>
  );
}
