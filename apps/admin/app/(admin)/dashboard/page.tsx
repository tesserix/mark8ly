import { AdminPage } from "@/components/layout";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { fetchDashboard } from "@/lib/api/marketplace-api";
import type { DashboardResponse } from "@/lib/api/marketplace-api";

import { StatCard } from "@/components/dashboard/StatCard";
import { RevenueSparkline } from "@/components/dashboard/RevenueSparkline";
import { SetupChecklist } from "@/components/dashboard/SetupChecklist";
import { RecentOrders } from "@/components/dashboard/RecentOrders";
import { TopProducts } from "@/components/dashboard/TopProducts";
import { LowStockAlerts } from "@/components/dashboard/LowStockAlerts";
import { AnalyticsSection } from "@/components/dashboard/AnalyticsSection";

/**
 * Dashboard — data-driven merchant home with stats, orders, products,
 * setup checklist, and low-stock alerts.
 */

// Pull a friendly first name out of an email local part. Best-effort:
// "mahesh.sangawar@gmail.com" → "Mahesh", "alex+work@foo.com" → "Alex".
// Returns null when the email looks weird so the greeting can drop the
// personalization instead of saying "Welcome back, <garbage>".
function firstNameFromEmail(email: string | null | undefined): string | null {
  if (!email) return null;
  const local = email.split("@")[0] ?? "";
  if (!local) return null;
  const cleaned = local.split(/[+.\-_]/)[0] ?? "";
  if (!cleaned || !/^[a-z]+$/i.test(cleaned)) return null;
  return cleaned.charAt(0).toUpperCase() + cleaned.slice(1).toLowerCase();
}
export default async function DashboardPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = await getServerSessionContext();

  const dashboard = currentStore
    ? await fetchDashboard(currentStore.id, { userId, tenantId })
    : null;

  const firstName = firstNameFromEmail(email);

  return (
          <AdminPage
        eyebrow={firstName ? `Welcome back, ${firstName}` : "Welcome back"}
        title="Dashboard"
      >
        {!currentStore ? (
          <EmptyStoreState />
        ) : !dashboard ? (
          <DashboardLoadError />
        ) : (
          <DashboardContent
            dashboard={dashboard}
            currencyCode={currentStore.currency_code ?? "USD"}
            storeId={currentStore.id}
          />
        )}
      </AdminPage>
  );
}

function EmptyStoreState() {
  return (
    <div className="py-16 text-center">
      <h2 className="font-serif text-3xl font-medium text-foreground">
        Create a store to get started
      </h2>
      <p className="mt-3 text-base text-foreground-secondary">
        Set up your first store to access your dashboard.
      </p>
      <a
        href="/settings/stores"
        className="mt-6 inline-flex h-12 items-center justify-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
      >
        Create store
      </a>
    </div>
  );
}

function DashboardLoadError() {
  return (
    <div className="py-16 text-center" role="alert">
      <h2 className="font-serif text-3xl font-medium text-foreground">
        Unable to load dashboard
      </h2>
      <p className="mt-3 text-base text-foreground-secondary">
        We could not reach the server. Please try refreshing the page.
      </p>
    </div>
  );
}

function DashboardContent({
  dashboard,
  currencyCode,
  storeId,
}: {
  dashboard: DashboardResponse;
  currencyCode: string;
  storeId: string;
}) {
  const { stats, setup_checklist, recent_orders, top_products, low_stock } =
    dashboard;
  const isNewStore = !setup_checklist.has_product;

  return (
    <>
      <SetupChecklist checklist={setup_checklist} />

      {isNewStore ? (
        <section className="py-10 text-center">
          <h2 className="font-serif text-2xl font-medium text-foreground">
            Complete your store setup to see your dashboard
          </h2>
          <p className="mt-3 max-w-md mx-auto text-sm text-foreground-secondary">
            Add your first product to unlock your stats, recent orders, and
            product performance data.
          </p>
        </section>
      ) : (
        <>
          {/* Stat cards — 4 across on desktop, 2x2 tablet, 1 on mobile */}
          <div className="grid grid-cols-1 gap-px border border-border-subtle sm:grid-cols-2 lg:grid-cols-4">
            <StatCard
              label="Revenue today"
              value={formatCurrency(stats.revenue_today, currencyCode)}
              changePercent={stats.revenue_change_pct}
              sparkline={<RevenueSparkline data={stats.revenue_trend} />}
            />
            <StatCard
              label="Orders today"
              value={String(stats.orders_today)}
              subtitle={`${stats.orders_pending} pending / ${stats.orders_fulfilled} fulfilled`}
            />
            <StatCard
              label="Total customers"
              value={String(stats.customers_total)}
              subtitle={
                stats.customers_new_this_week > 0
                  ? `+${stats.customers_new_this_week} this week`
                  : undefined
              }
            />
            <StatCard
              label="Pending reviews"
              value={String(stats.pending_reviews)}
              href="/customers/reviews?status=pending"
            />
          </div>

          {/* Range-scoped analytics — Sales / Orders / Customers / Reviews */}
          <AnalyticsSection storeId={storeId} currencyCode={currencyCode} />

          {/* Two-column: recent orders + top products */}
          <div className="grid grid-cols-1 gap-10 lg:grid-cols-2">
            <RecentOrders orders={recent_orders} currencyCode={currencyCode} />
            <TopProducts products={top_products} currencyCode={currencyCode} />
          </div>

          {/* Low stock alerts */}
          <LowStockAlerts items={low_stock} />
        </>
      )}
    </>
  );
}

function formatCurrency(amount: number, currencyCode: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currencyCode,
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(amount);
  } catch {
    return `${currencyCode} ${amount.toFixed(0)}`;
  }
}
