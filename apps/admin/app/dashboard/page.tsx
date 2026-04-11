import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { fetchDashboard } from "@/lib/api/marketplace-api";
import type { DashboardResponse } from "@/lib/api/marketplace-api";

import { StatCard } from "@/components/dashboard/StatCard";
import { RevenueSparkline } from "@/components/dashboard/RevenueSparkline";
import { SetupChecklist } from "@/components/dashboard/SetupChecklist";
import { RecentOrders } from "@/components/dashboard/RecentOrders";
import { TopProducts } from "@/components/dashboard/TopProducts";
import { LowStockAlerts } from "@/components/dashboard/LowStockAlerts";

/**
 * Dashboard — data-driven merchant home with stats, orders, products,
 * setup checklist, and low-stock alerts.
 */
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

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-5xl space-y-10">
        {!currentStore ? (
          <EmptyStoreState />
        ) : !dashboard ? (
          <DashboardError />
        ) : (
          <DashboardContent dashboard={dashboard} email={email} />
        )}
      </div>
    </AdminShell>
  );
}

function EmptyStoreState() {
  return (
    <div className="py-16 text-center">
      <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl font-medium text-foreground">
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

function DashboardError() {
  return (
    <div className="py-16 text-center" role="alert">
      <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl font-medium text-foreground">
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
  email,
}: {
  dashboard: DashboardResponse;
  email: string;
}) {
  const { stats, setup_checklist, recent_orders, top_products, low_stock_items } =
    dashboard;
  const isNewStore =
    !setup_checklist.has_test_order && !setup_checklist.has_product;

  return (
    <>
      <header className="space-y-1">
        <p className="eyebrow">
          Welcome back{email ? `, ${email}` : ""}
        </p>
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl font-medium tracking-tight text-foreground sm:text-5xl">
          Dashboard
        </h1>
      </header>

      <SetupChecklist checklist={setup_checklist} />

      {isNewStore ? (
        <section className="py-10 text-center">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-foreground">
            Complete your store setup to see your dashboard
          </h2>
          <p className="mt-3 max-w-md mx-auto text-sm text-foreground-secondary">
            Add your first product and place a test order to unlock your stats,
            recent orders, and product performance data.
          </p>
        </section>
      ) : (
        <>
          {/* Stat cards — 4 across on desktop, 2x2 tablet, 1 on mobile */}
          <div className="grid grid-cols-1 gap-px border border-border-subtle sm:grid-cols-2 lg:grid-cols-4">
            <StatCard
              label="Revenue today"
              value={formatCurrency(stats.revenue_today)}
              changePercent={stats.revenue_change_pct}
              sparkline={
                <RevenueSparkline data={stats.revenue_sparkline} />
              }
            />
            <StatCard
              label="Orders today"
              value={String(stats.orders_today)}
              subtitle={`${stats.orders_pending} pending / ${stats.orders_fulfilled} fulfilled`}
            />
            <StatCard
              label="Total customers"
              value={String(stats.total_customers)}
              subtitle={
                stats.new_customers_this_week > 0
                  ? `+${stats.new_customers_this_week} this week`
                  : undefined
              }
            />
            <StatCard
              label="Pending reviews"
              value={String(stats.pending_reviews)}
              href="/customers/reviews?status=pending"
            />
          </div>

          {/* Two-column: recent orders + top products */}
          <div className="grid grid-cols-1 gap-10 lg:grid-cols-2">
            <RecentOrders orders={recent_orders} />
            <TopProducts products={top_products} />
          </div>

          {/* Low stock alerts */}
          <LowStockAlerts items={low_stock_items} />
        </>
      )}
    </>
  );
}

function formatCurrency(cents: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(cents / 100);
}
