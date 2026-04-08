import { AdminShell } from "@/components/shell/AdminShell";
import { GeneralSettingsForm } from "@/components/settings/GeneralSettingsForm";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";

/**
 * /settings/general — store identity. Most fields are read-only after
 * onboarding (slug breaks URLs, currency affects billing, country
 * affects tax, owner email is auth-coupled). The merchant can edit
 * the display name; everything else routes to support.
 */
export default async function GeneralSettingsPage() {
  const {
    tenantName,
    email,
    tenant,
    role,
    memberships,
    tenantId,
    currentStore,
  } = await getServerSessionContext();
  const editable = canEditSettings(role);

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
            General settings
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Your store details. Some fields are locked after onboarding —
            contact support to change them.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view settings but cannot edit
              them.
            </p>
          )}
        </header>

        {tenant && currentStore ? (
          <GeneralSettingsForm
            tenant={tenant}
            store={currentStore}
            editable={editable}
          />
        ) : (
          <p className="text-sm text-danger">
            We couldn&apos;t load your store details. Please refresh, or
            contact support if the problem persists.
          </p>
        )}
      </div>
    </AdminShell>
  );
}
