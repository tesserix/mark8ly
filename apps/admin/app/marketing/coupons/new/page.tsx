import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { CouponForm } from "@/components/marketing/coupons/CouponForm";
import { createCoupon, type CreateCouponBody } from "@/lib/api/coupons-api";
import { redirect } from "next/navigation";

export default async function NewCouponPage() {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;

  if (!currentStore) {
    redirect("/marketing/coupons");
  }

  if (role !== "owner" && role !== "admin") {
    redirect("/marketing/coupons");
  }

  async function handleSubmit(body: CreateCouponBody): Promise<boolean> {
    "use server";
    const result = await createCoupon(currentStore!.id, body, {
      userId,
      tenantId,
    });
    return result !== null;
  }

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-2xl px-8 py-6">
        <h1 className="mb-6 font-serif text-2xl font-semibold text-ink-900">
          Create coupon
        </h1>
        <CouponForm
          storeId={currentStore.id}
          storeCurrency={currentStore.currency_code ?? "USD"}
          onSubmit={handleSubmit}
        />
      </main>
    </AdminShell>
  );
}
