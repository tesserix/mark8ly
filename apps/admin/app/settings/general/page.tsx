import { AdminShell } from "@/components/shell/AdminShell";
import { GeneralSettingsForm } from "@/components/settings/GeneralSettingsForm";
import { getServerSessionContext } from "@/lib/auth/serverSession";

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
  const { tenantName, email, tenant } = await getServerSessionContext();

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <div className="mx-auto w-full max-w-3xl px-6 py-10">
        <header className="mb-8">
          <h1 className="text-2xl font-semibold text-foreground">
            General settings
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Your store details. Some fields are locked after onboarding —
            contact support to change them.
          </p>
        </header>

        {tenant ? (
          <GeneralSettingsForm tenant={tenant} />
        ) : (
          <div className="rounded-xl border border-destructive/20 bg-destructive/5 p-6 text-sm text-destructive">
            We couldn&apos;t load your store details. Please refresh, or
            contact support if the problem persists.
          </div>
        )}
      </div>
    </AdminShell>
  );
}
