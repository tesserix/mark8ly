import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import {
  getSupportedProviders,
  listShippingConfigs,
} from "@/lib/api/settings-api";
import { listWarehouses } from "@/lib/api/warehouses-api";
import { ShippingSettingsClient } from "@/components/settings/ShippingSettingsClient";
import { WarehousesSettingsClient } from "@/components/settings/WarehousesSettingsClient";

/**
 * /settings/shipping — the whole "where do we ship from, and with whom"
 * job on one page (#177 PR 5d).
 *
 * Two sections, deliberately in this order: the warehouses a store ships
 * FROM, then the carriers that quote against them. A carrier cannot quote
 * without an origin, so a merchant who arrives with neither is walked
 * through it in the order the dependency actually runs — rather than
 * meeting a carrier form that sends them to another page and back.
 *
 * Same page is not the same object. The address lives on the warehouse and
 * nowhere else; the carrier binds to it by id. That separation is the
 * whole of #177, and it survives the shared page.
 */
export default async function ShippingSettingsPage() {
  const { tenantName, email, role, memberships, tenantId, userId, currentStore } =
    await getServerSessionContext();

  const editable = canEditSettings(role);
  const country = currentStore?.country_code;

  return (
          <AdminPage
        eyebrow="Selling"
        title="Shipping"
        description={
          <>
            Where your store ships from, and which carriers quote for it
            {country ? ` (${country})` : ""}. Carriers quote rates from a
            warehouse address, so add a warehouse first.
          </>
        }
        readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
      >
        {currentStore ? (
          <ShippingSettingsContent
            storeId={currentStore.id}
            userId={userId}
            tenantId={tenantId}
            editable={editable}
            storeCountry={(currentStore.country_code ?? "").toUpperCase()}
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store before configuring shipping.
          </p>
        )}
      </AdminPage>
  );
}

async function ShippingSettingsContent({
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
  const session = { userId, tenantId };
  const [supported, configs, warehouses] = await Promise.all([
    getSupportedProviders(storeId, session),
    listShippingConfigs(storeId, session),
    listWarehouses(storeId, session),
  ]);

  if (!supported) {
    return (
      <p className="border-t border-border-subtle py-10 text-sm text-foreground-tertiary">
        Unable to load supported carriers for this store. Please try again
        later.
      </p>
    );
  }

  return (
    <div className="space-y-12">
      <section id="warehouses" className="scroll-mt-24 space-y-5">
        <div className="space-y-1.5">
          <h2 className="font-serif text-xl text-foreground">Ships from</h2>
          <p className="max-w-2xl text-sm text-foreground-secondary">
            Your pickup locations. Orders are allocated to them in the order
            listed here.
          </p>
        </div>
        <WarehousesSettingsClient
          warehouses={warehouses}
          editable={editable}
          storeCountry={storeCountry}
        />
      </section>

      <section id="carriers" className="scroll-mt-24 space-y-5">
        <div className="space-y-1.5">
          <h2 className="font-serif text-xl text-foreground">Carriers</h2>
          <p className="max-w-2xl text-sm text-foreground-secondary">
            Credentials and fees per carrier. Each one ships from a warehouse
            above.
          </p>
        </div>
        <ShippingSettingsClient
          supported={supported}
          configs={configs}
          warehouses={warehouses}
          editable={editable}
          storeCountry={storeCountry}
        />
      </section>
    </div>
  );
}
