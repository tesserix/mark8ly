import { AdminShell } from "@/components/shell/AdminShell";
import { StoresList } from "@/components/settings/StoresList";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";

/**
 * Admin /settings/stores — Phase Q.2.
 *
 * Lists every store under the current tenant with a "switch to
 * store" button for each, and an "Add store" CTA for owner/admin
 * roles. The current store is badged. Store editing (name,
 * currency, logo) continues to live on /settings/general — this
 * page is purely the multi-store index.
 */
export default async function StoresIndexPage() {
  const {
    tenantName,
    email,
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
      <div className="mx-auto w-full max-w-5xl space-y-6">
        <header className="admin-surface rounded-[2rem] px-6 py-7 sm:px-8">
          <p className="admin-eyebrow">Store setup</p>
          <h1 className="mt-3 font-serif text-4xl font-medium tracking-tight text-foreground">
            Stores
          </h1>
          <p className="mt-3 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
            Every storefront under {tenantName}. Switch between
            stores to edit their settings, or add a new one to run a
            second brand from the same account.
          </p>
        </header>

        <StoresList
          stores={stores}
          currentStoreId={currentStore?.id ?? ""}
          canManage={canManage}
        />
      </div>
    </AdminShell>
  );
}
