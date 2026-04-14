import { AdminPage, PageSection, ReadOnlyNotice } from "@/components/layout";
import { StoresList } from "@/components/settings/StoresList";
import { GeneralSettingsForm } from "@/components/settings/GeneralSettingsForm";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import {
  listCountries,
  listCurrencies,
  listTimezones,
} from "@/lib/api/platform-api";

/**
 * /settings/stores — multi-store index + current-store identity editor.
 *
 * Merged from the old /settings/general page so merchants see the full
 * store list and the active store's editable fields (display name, etc.)
 * in one place. Most identity fields are locked after onboarding — slug
 * breaks URLs, currency affects billing, country drives tax — so they
 * route to support.
 */
export default async function StoresIndexPage() {
  const {
    tenantName,
    email,
    tenant,
    role,
    memberships,
    tenantId,
    stores,
    currentStore,
  } = await getServerSessionContext();
  const canManage = canEditSettings(role);

  const [countries, currencies, timezones] = await Promise.all([
    listCountries(),
    listCurrencies(),
    listTimezones(),
  ]);

  return (
          <AdminPage
        eyebrow="Store"
        title="Stores"
        description={
          <>
            Every storefront under {tenantName}. Switch between stores to edit
            their settings, or add a new one to run a second brand from the
            same account.
          </>
        }
        readOnlyNotice={
          !canManage && role ? <ReadOnlyNotice role={role} /> : undefined
        }
      >
        <StoresList
          stores={stores}
          currentStoreId={currentStore?.id ?? ""}
          canManage={canManage}
          countries={countries}
          currencies={currencies}
        />

        <PageSection
          eyebrow="Current store"
          title="Store identity"
          description="Details for the store you're currently managing. Some fields are locked after onboarding — contact support to change them."
        >
          {tenant && currentStore ? (
            <GeneralSettingsForm
              tenant={tenant}
              store={currentStore}
              editable={canManage}
              timezones={timezones}
            />
          ) : (
            <p className="text-sm text-danger">
              We couldn&apos;t load your store details. Please refresh, or
              contact support if the problem persists.
            </p>
          )}
        </PageSection>
      </AdminPage>
  );
}
