import { AdminShell } from "@/components/shell/AdminShell";
import { StorefrontThemeForm } from "@/components/settings/StorefrontThemeForm";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";

export default async function StorefrontSettingsPage() {
  const {
    tenantName,
    email,
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
      <div className="mx-auto w-full max-w-6xl space-y-6">
        <header className="admin-surface rounded-[2rem] px-6 py-7 sm:px-8">
          <p className="admin-eyebrow">Storefront</p>
          <h1 className="mt-3 font-serif text-4xl font-medium tracking-tight text-foreground">
            Theme &amp; layout
          </h1>
          <p className="mt-3 max-w-3xl text-sm leading-7 text-muted-foreground sm:text-base">
            Choose a storefront layout, apply a visual preset, then
            personalize the colors, typography, and motion. The current
            store will pick these settings up directly.
          </p>
          {!editable && (
            <p className="mt-4 inline-flex rounded-full border border-amber-300 bg-amber-50 px-3 py-1 text-xs text-amber-900">
              Read-only: your role ({role}) can view storefront settings but
              cannot publish changes.
            </p>
          )}
        </header>

        {currentStore ? (
          <StorefrontThemeForm store={currentStore} editable={editable} />
        ) : (
          <div className="rounded-2xl border border-destructive/20 bg-destructive/5 p-6 text-sm text-destructive">
            We couldn&apos;t load the current store. Please refresh, or contact
            support if the problem persists.
          </div>
        )}
      </div>
    </AdminShell>
  );
}
