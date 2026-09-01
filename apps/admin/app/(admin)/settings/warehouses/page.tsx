import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { listWarehouses } from "@/lib/api/warehouses-api";
import { WarehousesSettingsClient } from "@/components/settings/WarehousesSettingsClient";

/**
 * /settings/warehouses — the store's pickup locations (#177 PR 5c).
 *
 * Deliberately its own page rather than a section of /settings/shipping: a
 * warehouse belongs to the store, and every carrier ships from the same
 * ones. Editing it inside a carrier card is what let a mistyped name
 * create a second, stockless warehouse that quietly took allocations.
 */
export default async function WarehousesSettingsPage() {
  const { role, tenantId, userId, currentStore } =
    await getServerSessionContext();

  const editable = canEditSettings(role);

  return (
    <AdminPage
      eyebrow="Selling"
      title="Warehouses"
      description="The locations you ship from. Carriers quote rates from a warehouse address, and orders are allocated to them in the order listed here."
      readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
    >
      {currentStore ? (
        <WarehousesContent
          storeId={currentStore.id}
          userId={userId}
          tenantId={tenantId}
          editable={editable}
          storeCountry={(currentStore.country_code ?? "").toUpperCase()}
        />
      ) : (
        <p className="text-sm text-danger">
          No store found. Please create a store before adding warehouses.
        </p>
      )}
    </AdminPage>
  );
}

async function WarehousesContent({
  storeId,
  userId,
  tenantId,
  editable,
  storeCountry,
}: {
  storeId: string;
  userId: string;
  tenantId: string;
  editable: boolean;
  storeCountry: string;
}) {
  const warehouses = await listWarehouses(storeId, { userId, tenantId });

  return (
    <WarehousesSettingsClient
      warehouses={warehouses}
      editable={editable}
      storeCountry={storeCountry}
    />
  );
}
