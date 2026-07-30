import { AdminPage } from "@/components/layout";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { fetchDashboard } from "@/lib/api/marketplace-api";
import type { DashboardResponse } from "@/lib/api/marketplace-api";

import Link from "next/link";

import { DashboardHero } from "@/components/dashboard/DashboardHero";
import { OrdersStatStrip } from "@/components/dashboard/OrdersStatStrip";
import { SetupChecklist } from "@/components/dashboard/SetupChecklist";
import { RecentOrders } from "@/components/dashboard/RecentOrders";
import { TopProducts } from "@/components/dashboard/TopProducts";
import { LowStockAlerts } from "@/components/dashboard/LowStockAlerts";
import { AnalyticsSection } from "@/components/dashboard/AnalyticsSection";
import { MobileAppPrompt } from "@/components/dashboard/MobileAppPrompt";

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
            tenantId={tenantId}
          />
        )}
      </AdminPage>
  );
}

function EmptyStoreState() {
  return (
    <div className="flex flex-col items-start gap-3 border-t border-border-subtle py-12">
      <h2 className="font-serif text-3xl font-medium text-foreground">
        Begin with a store
      </h2>
      <p className="max-w-prose text-base text-foreground-secondary">
        Your dashboard lives here — revenue, orders, customers, low stock.
        Spin up your first store and the numbers will follow.
      </p>
      <a
        href="/settings/stores"
        className="mt-3 inline-flex items-center rounded-md bg-[color:var(--ink-900)] px-5 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        Create store
      </a>
    </div>
  );
}

function DashboardLoadError() {
  return (
    <div
      className="flex flex-col items-start gap-3 border-t border-border-subtle py-12"
      role="alert"
    >
      <h2 className="font-serif text-3xl font-medium text-foreground">
        We couldn&rsquo;t reach your store
      </h2>
      <p className="max-w-prose text-base text-foreground-secondary">
        A momentary hiccup on our side. Refresh to try again.
      </p>
    </div>
  );
}

function DashboardContent({
  dashboard,
  currencyCode,
  storeId,
  tenantId,
}: {
  dashboard: DashboardResponse;
  currencyCode: string;
  storeId: string;
  tenantId: string;
}) {
  const { stats, setup_checklist, recent_orders, top_products, low_stock } =
    dashboard;
  const isNewStore = !setup_checklist.has_product;

  return (
    <>
      <SetupChecklist checklist={setup_checklist} storeId={storeId} />

      {isNewStore ? (
        <section className="flex flex-col items-start gap-3 border-t border-border-subtle py-10">
          <h2 className="font-serif text-2xl font-medium text-foreground">
            Your first product unlocks the view
          </h2>
          <p className="max-w-prose text-sm text-foreground-secondary">
            Once you add a product, revenue, recent orders, and performance
            data start flowing into this page.
          </p>
        </section>
      ) : (
        <>
          {/* Editorial masthead — this-month revenue hero + secondary column,
              replacing the old equal-weight 4-card grid. */}
          <DashboardHero stats={stats} currencyCode={currencyCode} />

          {/* Orders as an editorial stat strip. */}
          <OrdersStatStrip stats={stats} />

          {/* Pending reviews — surfaced only when there's something to act on. */}
          {stats.pending_reviews > 0 && (
            <Link
              href="/customers/reviews?status=pending"
              className="flex items-baseline justify-between gap-4 border-t border-border-subtle pt-6 transition-colors hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              <span className="text-sm text-foreground-secondary">
                <span className="font-serif text-2xl font-medium tabular-nums text-foreground">
                  {stats.pending_reviews}
                </span>{" "}
                {stats.pending_reviews === 1 ? "review" : "reviews"} awaiting
                moderation
              </span>
              <span className="text-sm font-medium text-[color:var(--moss-700)]">
                Review →
              </span>
            </Link>
          )}

          {/* Range-scoped analytics — Sales / Orders / Customers / Reviews */}
          <AnalyticsSection storeId={storeId} currencyCode={currencyCode} />

          {/* Two-column: recent orders + top products */}
          <div className="grid grid-cols-1 gap-10 lg:grid-cols-2">
            <RecentOrders orders={recent_orders} currencyCode={currencyCode} />
            <TopProducts products={top_products} currencyCode={currencyCode} />
          </div>

          {/* Low stock alerts */}
          <LowStockAlerts items={low_stock} />

          {/* Mobile app nudge — last, so it never competes with the revenue
              hero, and dismissible so it cannot become a nag. Deliberately
              absent from the new-store branch above: that path is focused on
              adding a first product, and those merchants have just come from
              the onboarding welcome page, which carries the same badges. */}
          <MobileAppPrompt tenantId={tenantId} />
        </>
      )}
    </>
  );
}
