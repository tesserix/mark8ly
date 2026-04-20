import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getCoupon } from "@/lib/api/coupons-api";
import { Breadcrumbs } from "@/components/layout";
import { CouponDetailSummary } from "@/components/marketing/coupons/CouponDetailSummary";
import { CouponUsageTable } from "@/components/marketing/coupons/CouponUsageTable";
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
    <main className="flex flex-col gap-8">
      <Breadcrumbs
        items={[
          { label: "Marketing", href: "/marketing/campaigns" },
          { label: "Coupons", href: "/marketing/coupons" },
          { label: response.data.code },
        ]}
      />

      <header className="flex flex-col gap-2">
        <p className="eyebrow">Coupon</p>
        <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground">
          {response.data.code}
        </h1>
      </header>

      <CouponDetailSummary coupon={response.data} />

      <section className="flex flex-col gap-4 border-t border-border-subtle pt-8">
        <h2 className="font-serif text-2xl font-medium text-foreground">
          Usage history
        </h2>
        <CouponUsageTable
          usages={response.usage ?? []}
          total={response.usage_total ?? 0}
        />
      </section>
    </main>
  );
}
