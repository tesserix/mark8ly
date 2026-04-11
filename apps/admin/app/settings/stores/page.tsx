import { AdminShell } from "@/components/shell/AdminShell";
import { StoresList } from "@/components/settings/StoresList";
import { GeneralSettingsForm } from "@/components/settings/GeneralSettingsForm";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";

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

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-5xl space-y-12">
        <header className="space-y-3">
          <p className="eyebrow">Store setup</p>
          <h1 className="font-serif text-5xl font-medium tracking-tight text-foreground">
            Stores
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Every storefront under {tenantName}. Switch between stores to edit
            their settings, or add a new one to run a second brand from the
            same account.
          </p>
          {!canManage && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view settings but cannot edit
              them.
            </p>
          )}
        </header>

        <StoresList
          stores={stores}
          currentStoreId={currentStore?.id ?? ""}
          canManage={canManage}
        />

        <section className="space-y-6">
          <div className="space-y-2">
            <p className="eyebrow">Current store</p>
            <h2 className="font-serif text-3xl font-medium tracking-tight text-foreground">
              Store identity
            </h2>
            <p className="max-w-2xl text-sm leading-6 text-foreground-secondary">
              Details for the store you&apos;re currently managing. Some fields
              are locked after onboarding — contact support to change them.
            </p>
          </div>

          {tenant && currentStore ? (
            <GeneralSettingsForm
              tenant={tenant}
              store={currentStore}
              editable={canManage}
            />
          ) : (
            <p className="text-sm text-danger">
              We couldn&apos;t load your store details. Please refresh, or
              contact support if the problem persists.
            </p>
          )}
        </section>
      </div>
    </AdminShell>
  );
}
