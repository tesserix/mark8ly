import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import { InStoreSettingsForm } from "@/components/settings/InStoreSettingsForm";
import { getInStoreSettings } from "@/lib/api/instore-api";
import { canEditSettings, getServerSessionContext } from "@/lib/auth/serverSession";

// Settings → In-store sales: turn selling at a counter or stall on or off for
// the current store, whether cashiers may give discounts, and an AU store's ABN.
export default async function InStoreSettingsPage() {
  const { role, currentStore, userId, tenantId, email } = await getServerSessionContext();
  const editable = canEditSettings(role);
  // Owners and admins only: the API refuses everyone else.
  const settings =
    editable && currentStore ? await getInStoreSettings(currentStore.id, { userId, tenantId, email }) : null;

  return (
    <AdminPage
      eyebrow="Selling"
      title="In-store sales"
      description="Sell at your counter, market stall or pop-up: ring up the sale here, take payment on your own terminal or in cash."
      readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
    >
      {!currentStore ? (
        <p className="text-sm text-foreground-secondary">Select a store first.</p>
      ) : settings ? (
        <InStoreSettingsForm storeName={currentStore.name} countryCode={currentStore.country_code} initial={settings} />
      ) : editable ? (
        <p role="alert" className="text-sm text-danger">
          Couldn&rsquo;t load the in-store settings. Reload the page to try again.
        </p>
      ) : null}
    </AdminPage>
  );
}
