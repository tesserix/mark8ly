import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getTaxConfig } from "@/lib/api/settings-api";
import { TaxJarConfigForm } from "@/components/settings/TaxJarConfigForm";

/**
 * /settings/tax — tax configuration.
 *
 * The tax page is strategy-aware:
 *   - "flat_rate" countries (UK, AU, etc): show read-only VAT/GST rate
 *   - "india_gst": show GST info with note about HSN codes on products
 *   - "taxjar" (US): show TaxJar configuration form
 */
export default async function TaxSettingsPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
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
      <div className="mx-auto w-full max-w-5xl space-y-10">
        <header className="space-y-3">
          <p className="eyebrow">Store setup</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            Tax settings
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Tax calculation for your store
            {currentStore ? ` (${currentStore.country_code})` : ""}.
            The strategy is determined by your store&apos;s country and cannot
            be changed.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view settings but cannot edit
              them.
            </p>
          )}
        </header>

        {currentStore ? (
          <TaxSettingsContent
            storeId={currentStore.id}
            userId={userId}
            tenantId={tenantId}
            editable={editable}
            countryCode={currentStore.country_code}
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store before configuring tax.
          </p>
        )}
      </div>
    </AdminShell>
  );
}

async function TaxSettingsContent({
  storeId,
  userId,
  tenantId,
  editable,
  countryCode,
}: {
  storeId: string;
  userId: string;
  tenantId: string;
  editable: boolean;
  countryCode: string;
}) {
  const taxConfig = await getTaxConfig(storeId, { userId, tenantId });

  if (!taxConfig) {
    return (
      <div className="rounded-[6px] bg-white px-6 py-10 text-center">
        <p className="text-sm text-[color:var(--ink-900)]/50">
          Unable to load tax configuration. Please try again later.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Strategy summary */}
      <div className="rounded-[6px] bg-white px-6 py-5">
        <div className="flex items-start gap-4">
          <div className="space-y-1.5">
            <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg font-medium text-[color:var(--ink-900)]">
              Tax strategy
            </h2>
            <p className="text-sm text-[color:var(--ink-900)]/60">
              {strategyLabel(taxConfig.strategy, countryCode)}
            </p>
          </div>
          <span className="ml-auto inline-block rounded-[4px] bg-[color:var(--moss-700)]/10 px-2.5 py-0.5 text-xs font-semibold uppercase tracking-wider text-[color:var(--moss-700)]">
            {taxConfig.strategy.replace(/_/g, " ")}
          </span>
        </div>
      </div>

      {/* Flat-rate countries */}
      {taxConfig.strategy === "flat_rate" && taxConfig.rate != null && (
        <div className="rounded-[6px] bg-white px-6 py-5">
          <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50 mb-3">
            Rate
          </h3>
          <p className="text-2xl font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-[color:var(--ink-900)]">
            {taxConfig.rate}%
          </p>
          <p className="mt-2 text-sm text-[color:var(--ink-900)]/50">
            This rate is applied automatically to all orders. It is determined
            by your store&apos;s country ({countryCode}) and cannot be changed
            here.
          </p>
        </div>
      )}

      {/* India GST */}
      {taxConfig.strategy === "india_gst" && taxConfig.india_gst && (
        <div className="rounded-[6px] bg-white px-6 py-5 space-y-3">
          <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
            India GST (automatic)
          </h3>
          <p className="text-sm text-[color:var(--ink-900)]/70 leading-6">
            {taxConfig.india_gst.description}
          </p>
          <p className="text-sm text-[color:var(--ink-900)]/50">
            Set the HSN code and GST rate on each product to ensure correct
            tax calculation at checkout. No additional configuration is needed
            here.
          </p>
        </div>
      )}

      {/* US: TaxJar */}
      {taxConfig.strategy === "taxjar" && (
        <div className="rounded-[6px] bg-white px-6 py-5 space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)]/50">
              TaxJar configuration
            </h3>
            {taxConfig.taxjar?.configured && (
              <span
                className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ${
                  taxConfig.taxjar.is_active
                    ? "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]"
                    : "bg-[color:var(--ink-900)]/5 text-[color:var(--ink-900)]/50"
                }`}
              >
                <span
                  className={`h-1.5 w-1.5 rounded-full ${
                    taxConfig.taxjar.is_active
                      ? "bg-[color:var(--moss-700)]"
                      : "bg-[color:var(--ink-900)]/30"
                  }`}
                  aria-hidden="true"
                />
                {taxConfig.taxjar.is_active ? "Active" : "Inactive"}
              </span>
            )}
          </div>

          <p className="text-sm text-[color:var(--ink-900)]/50 leading-6">
            US stores use TaxJar for sales tax calculation. Enter your TaxJar
            API key below to enable automatic tax rates at checkout.
          </p>

          <hr className="border-t border-[color:var(--ink-900)]/6" />

          {editable ? (
            <TaxJarConfigForm
              existingApiKey={taxConfig.taxjar?.api_key}
              existingMode={taxConfig.taxjar?.mode}
              isConfigured={taxConfig.taxjar?.configured ?? false}
            />
          ) : (
            <p className="text-sm text-[color:var(--ink-900)]/50">
              You do not have permission to edit tax configuration.
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function strategyLabel(strategy: string, countryCode: string): string {
  switch (strategy) {
    case "flat_rate":
      return `A flat VAT/GST rate is applied to all orders from ${countryCode}.`;
    case "india_gst":
      return "India GST rates are applied per-product based on HSN codes.";
    case "taxjar":
      return "US sales tax is calculated via TaxJar based on customer location.";
    default:
      return `Tax strategy: ${strategy}`;
  }
}
