import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getInStoreCatalog, getInStorePermissions, getMySalesToday } from "@/lib/api/instore-api";

import { AdminPage } from "@/components/layout";
import { MySalesToday } from "@/components/instore/MySalesToday";
import { SaleScreen } from "@/components/instore/SaleScreen";

export default async function SellPage() {
  const { currentStore, userId, tenantId } = await getServerSessionContext();

  if (!currentStore) {
    return (
      <AdminPage eyebrow="Orders" title="In-store sale" description="Select a store to sell from.">
        <p className="text-sm text-foreground-secondary">No store selected.</p>
      </AdminPage>
    );
  }

  const session = { userId, tenantId };
  const [catalog, permissions, mySales] = await Promise.all([
    getInStoreCatalog(currentStore.id, session),
    getInStorePermissions(currentStore.id, session),
    getMySalesToday(currentStore.id, session),
  ]);

  return (
    <AdminPage
      eyebrow="Orders"
      title="In-store sale"
      description="Ring up a sale at your counter or stall. Take payment on your own terminal or in cash, then record it here."
    >
      <SaleScreen
        storeId={currentStore.id}
        currencyCode={currentStore.currency_code}
        catalog={catalog}
        // The server decides: staff and above, or a cashier when the store allows it.
        canDiscount={permissions.canDiscount}
      />
      <div className="mt-10 border-t border-border-subtle pt-8 md:max-w-xl">
        <MySalesToday sales={mySales} currencyCode={currentStore.currency_code} />
      </div>
    </AdminPage>
  );
}
