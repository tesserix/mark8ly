import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getInStoreCatalog } from "@/lib/api/instore-api";

import { AdminPage } from "@/components/layout";
import { SaleScreen } from "@/components/instore/SaleScreen";

export default async function SellPage() {
  const { currentStore, role, userId, tenantId } = await getServerSessionContext();

  if (!currentStore) {
    return (
      <AdminPage eyebrow="Orders" title="In-store sale" description="Select a store to sell from.">
        <p className="text-sm text-foreground-secondary">No store selected.</p>
      </AdminPage>
    );
  }

  const catalog = await getInStoreCatalog(currentStore.id, { userId, tenantId });

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
        // Viewers cannot reach the sale routes; everyone else here is staff or above.
        canDiscount={role !== "viewer"}
      />
    </AdminPage>
  );
}
