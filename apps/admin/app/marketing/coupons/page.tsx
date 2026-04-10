import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listCoupons, type ListCouponsQuery } from "@/lib/api/coupons-api";

import { CouponsListHeader } from "@/components/marketing/coupons/CouponsListHeader";
import { CouponsListFilters } from "@/components/marketing/coupons/CouponsListFilters";
import { CouponsList } from "@/components/marketing/coupons/CouponsList";
import { CouponsListEmpty } from "@/components/marketing/coupons/CouponsListEmpty";

interface CouponsPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

function parseSearchParams(
  params: Record<string, string | string[] | undefined>,
): ListCouponsQuery {
  const first = (v: string | string[] | undefined): string | undefined =>
    Array.isArray(v) ? v[0] : v;
  return {
    status: first(params.status),
    search: first(params.search),
    page: first(params.page) ? Number(first(params.page)) : undefined,
    per_page: first(params.per_page) ? Number(first(params.per_page)) : undefined,
  };
}

export default async function CouponsPage({ searchParams }: CouponsPageProps) {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;
  const params = await searchParams;
  const query = parseSearchParams(params);
  const canCreate = role === "owner" || role === "admin";

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="flex flex-col gap-6 px-8 py-6">
          <CouponsListHeader canCreate={false} />
          <CouponsListEmpty variant="no-store" />
        </main>
      </AdminShell>
    );
  }

  const response = await listCoupons(currentStore.id, query, {
    userId,
    tenantId,
  });
  const coupons = response?.data ?? [];

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="flex flex-col gap-6 px-8 py-6">
        <CouponsListHeader canCreate={canCreate} />
        <CouponsListFilters />
        <hr className="border-ink-200" />
        {coupons.length === 0 ? (
          <CouponsListEmpty variant="no-coupons" />
        ) : (
          <CouponsList coupons={coupons} />
        )}
      </main>
    </AdminShell>
  );
}
