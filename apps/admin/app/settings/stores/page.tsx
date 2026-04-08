import { AdminShell } from "@/components/shell/AdminShell";
import { StoresList } from "@/components/settings/StoresList";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";

/**
 * /settings/stores — multi-store index. Lists every store under the
 * current tenant; owners and admins can add a new one.
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
