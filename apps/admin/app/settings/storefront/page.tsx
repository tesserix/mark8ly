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
      <div className="mx-auto w-full max-w-6xl space-y-12">
        <header className="space-y-3">
          <p className="eyebrow">Storefront</p>
          <h1 className="font-serif text-5xl font-medium tracking-tight text-foreground">
            Theme &amp; layout
          </h1>
          <p className="max-w-3xl text-base leading-7 text-foreground-secondary">
            Choose a storefront layout, apply a visual preset, then personalize
            the colors, typography, and motion. The current store will pick
            these settings up directly.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view storefront settings but
              cannot publish changes.
            </p>
          )}
        </header>

        {currentStore ? (
          <StorefrontThemeForm store={currentStore} editable={editable} />
        ) : (
          <p className="text-sm text-danger">
            We couldn&apos;t load the current store. Please refresh, or contact
            support if the problem persists.
          </p>
        )}
      </div>
    </AdminShell>
  );
}
