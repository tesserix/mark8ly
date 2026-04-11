import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getCoupon } from "@/lib/api/coupons-api";
import { CouponDetailSummary } from "@/components/marketing/coupons/CouponDetailSummary";
import { CouponUsageTable } from "@/components/marketing/coupons/CouponUsageTable";
import Link from "next/link";
import { notFound } from "next/navigation";

interface CouponDetailPageProps {
  params: Promise<{ id: string }>;
}

export default async function CouponDetailPage({
  params,
}: CouponDetailPageProps) {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId } = session;
  const { id } = await params;

  if (!currentStore) {
    notFound();
  }

  const response = await getCoupon(currentStore.id, id, { userId, tenantId });
  if (!response) {
    notFound();
  }

  return (
          <main className="mx-auto w-full max-w-3xl">
        <div className="mb-6 flex items-center gap-3">
          <Link
            href="/marketing/coupons"
            className="text-sm text-ink-500 hover:text-ink-700"
          >
            Coupons
          </Link>
          <span className="text-ink-300">/</span>
          <h1 className="font-serif text-2xl font-semibold text-ink-900">
            {response.data.code}
          </h1>
        </div>

        <CouponDetailSummary coupon={response.data} />

        <hr className="my-8 border-ink-200" />

        <h2 className="mb-4 font-serif text-lg font-semibold text-ink-900">
          Usage history
        </h2>
        <CouponUsageTable
          usages={response.usage ?? []}
          total={response.usage_total ?? 0}
        />
      </main>
  );
}
