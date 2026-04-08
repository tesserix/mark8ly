import { AdminShell } from "@/components/shell/AdminShell";
import { GeneralSettingsForm } from "@/components/settings/GeneralSettingsForm";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";

/**
 * Admin /settings/general — Phase N.
 *
 * First real admin feature page on top of the chrome. Reads the
 * tenant row populated during onboarding (business name, slug,
 * country, currency, timezone, owner email) and lets the merchant
 * edit the store name. Everything else renders read-only with a
 * "contact support to change" hint — slug breaks URLs, currency
 * affects billing, country affects tax, owner email is auth-coupled.
 *
 * Timezone editing is a sensible follow-up but needs a searchable
 * picker to be usable with ~400 entries; left out of v1.
 */
export default async function GeneralSettingsPage() {
  const { tenantName, email, tenant, role } = await getServerSessionContext();
  const editable = canEditSettings(role);

  return (
    <AdminShell tenantName={tenantName} userEmail={email} role={role}>
      <div className="mx-auto w-full max-w-5xl space-y-6">
        <header className="admin-surface rounded-[2rem] px-6 py-7 sm:px-8">
          <p className="admin-eyebrow">Store setup</p>
          <h1 className="mt-3 font-serif text-4xl font-medium tracking-tight text-foreground">
            General settings
          </h1>
          <p className="mt-3 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
            Your store details. Some fields are locked after onboarding —
            contact support to change them.
          </p>
          {!editable && (
            <p className="mt-4 inline-flex rounded-full border border-amber-300 bg-amber-50 px-3 py-1 text-xs text-amber-900">
              Read-only: your role ({role}) can view settings but cannot
              edit them.
            </p>
          )}
        </header>

        {tenant ? (
          <GeneralSettingsForm tenant={tenant} editable={editable} />
        ) : (
          <div className="rounded-2xl border border-destructive/20 bg-destructive/5 p-6 text-sm text-destructive">
            We couldn&apos;t load your store details. Please refresh, or
            contact support if the problem persists.
          </div>
        )}
      </div>
    </AdminShell>
  );
}
