"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import type {
  AnalyticsRange,
  CustomersMetricsResponse,
  MetricsTab,
  OrdersMetricsResponse,
  ReviewsMetricsResponse,
  SalesMetricsResponse,
} from "@/lib/api/marketplace-api";

import { AnalyticsKPI } from "./AnalyticsKPI";
import { AnalyticsRangeSelector } from "./AnalyticsRangeSelector";
import {
  CustomersSegmentChart,
  OrdersLineChart,
  OrdersStatusChart,
  RatingDistributionChart,
  RevenueAreaChart,
  ReviewsLineChart,
} from "./analytics-charts";

// ---------- Props ----------

interface AnalyticsSectionProps {
  storeId: string;
  currencyCode: string;
}

const TABS: ReadonlyArray<{ id: MetricsTab; label: string }> = [
  { id: "sales", label: "Sales" },
  { id: "orders", label: "Orders" },
  { id: "customers", label: "Customers" },
  { id: "reviews", label: "Reviews" },
];

// ---------- Types for per-tab cached payloads ----------

type TabPayload =
  | SalesMetricsResponse
  | OrdersMetricsResponse
  | CustomersMetricsResponse
  | ReviewsMetricsResponse;

type CacheMap = Partial<Record<string, TabPayload>>;

function cacheKey(tab: MetricsTab, range: AnalyticsRange): string {
  return `${tab}:${range}`;
}

// ---------- Section ----------

export function AnalyticsSection({
  storeId,
  currencyCode,
}: AnalyticsSectionProps) {
  const [tab, setTab] = useState<MetricsTab>("sales");
  const [range, setRange] = useState<AnalyticsRange>("30d");
  const [cache, setCache] = useState<CacheMap>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const load = useCallback(
    async (nextTab: MetricsTab, nextRange: AnalyticsRange) => {
      const key = cacheKey(nextTab, nextRange);
      if (cache[key]) {
        setError(null);
        return;
      }
      abortRef.current?.abort();
      const ctrl = new AbortController();
      abortRef.current = ctrl;
      setLoading(true);
      setError(null);
      try {
        const res = await fetch(
          `/api/admin/stores/${storeId}/analytics/${nextTab}?range=${nextRange}`,
          { signal: ctrl.signal, cache: "no-store" },
        );
        if (!res.ok) throw new Error(`Request failed: ${res.status}`);
        const data = (await res.json()) as TabPayload;
        setCache((prev) => ({ ...prev, [key]: data }));
      } catch (err) {
        if ((err as Error).name === "AbortError") return;
        setError("Could not load analytics.");
      } finally {
        if (abortRef.current === ctrl) setLoading(false);
      }
    },
    [cache, storeId],
  );

  useEffect(() => {
    load(tab, range);
  }, [tab, range, load]);

  const key = cacheKey(tab, range);
  const data = cache[key];

  return (
    <section
      aria-labelledby="analytics-heading"
      className="border-t border-border-subtle pt-8"
    >
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2
            id="analytics-heading"
            className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-foreground"
          >
            Analytics
          </h2>
          <p className="mt-1 text-sm text-foreground-secondary">
            Trends across your store over the selected window.
          </p>
        </div>
        <AnalyticsRangeSelector
          value={range}
          onChange={setRange}
          disabled={loading}
        />
      </div>

      {/* Tab nav — editorial underline treatment, matches range selector. */}
      <div
        role="tablist"
        aria-label="Analytics categories"
        className="mt-6 flex gap-6 border-b border-border-subtle"
      >
        {TABS.map((t) => {
          const active = tab === t.id;
          return (
            <button
              key={t.id}
              type="button"
              role="tab"
              aria-selected={active}
              onClick={() => setTab(t.id)}
              className={`
                relative pb-3 text-sm font-medium tracking-[0.02em]
                transition-colors
                focus-visible:outline-2 focus-visible:outline-offset-4
                focus-visible:outline-[color:var(--moss-700)]
                ${
                  active
                    ? "text-[color:var(--ink-900)] after:content-[''] after:absolute after:-bottom-px after:left-0 after:right-0 after:h-0.5 after:bg-[color:var(--ink-900)]"
                    : "text-foreground-tertiary hover:text-[color:var(--ink-900)]/80"
                }
              `}
            >
              {t.label}
            </button>
          );
        })}
      </div>

      {/* Live region for screen-reader announcement on load. */}
      <p className="sr-only" role="status" aria-live="polite">
        {loading
          ? `Loading ${tab} analytics for the last ${range === "7d" ? "7" : range === "30d" ? "30" : "90"} days.`
          : data
            ? `${tab} analytics loaded.`
            : ""}
      </p>

      <div className="mt-6">
        {error ? (
          <ErrorPanel onRetry={() => load(tab, range)} />
        ) : !data ? (
          <SkeletonPanel />
        ) : (
          <TabPanel
            tab={tab}
            data={data}
            currencyCode={currencyCode}
            stale={loading}
          />
        )}
      </div>
    </section>
  );
}

// ---------- Tab router ----------

function TabPanel({
  tab,
  data,
  currencyCode,
  stale,
}: {
  tab: MetricsTab;
  data: TabPayload;
  currencyCode: string;
  stale: boolean;
}) {
  const className = `transition-opacity duration-200 ${stale ? "opacity-60" : "opacity-100"}`;
  return (
    <div className={className}>
      {tab === "sales" && (
        <SalesTab data={data as SalesMetricsResponse} currency={currencyCode} />
      )}
      {tab === "orders" && <OrdersTab data={data as OrdersMetricsResponse} />}
      {tab === "customers" && (
        <CustomersTab
          data={data as CustomersMetricsResponse}
          currency={currencyCode}
        />
      )}
      {tab === "reviews" && (
        <ReviewsTab data={data as ReviewsMetricsResponse} />
      )}
    </div>
  );
}

// ---------- Sales ----------

function SalesTab({
  data,
  currency,
}: {
  data: SalesMetricsResponse;
  currency: string;
}) {
  return (
    <div className="space-y-8">
      <div className="grid grid-cols-2 gap-px border border-border-subtle sm:grid-cols-4">
        <AnalyticsKPI
          label="Gross revenue"
          value={formatCurrency(data.gross_revenue, currency)}
        />
        <AnalyticsKPI
          label="Net revenue"
          value={formatCurrency(data.net_revenue, currency)}
          subtitle={
            data.total_refunded > 0
              ? `${formatCurrency(data.total_refunded, currency)} refunded`
              : undefined
          }
        />
        <AnalyticsKPI
          label="Avg order value"
          value={formatCurrency(data.aov, currency)}
          deltaPercent={data.aov_delta_pct}
        />
        <AnalyticsKPI
          label="Coupons redeemed"
          value={String(data.coupon_redemptions)}
          subtitle={
            data.coupon_discount_total > 0
              ? `${formatCurrency(data.coupon_discount_total, currency)} off`
              : undefined
          }
        />
      </div>
      <RevenueAreaChart data={data.revenue_series} currency={currency} />
    </div>
  );
}

// ---------- Orders ----------

function OrdersTab({ data }: { data: OrdersMetricsResponse }) {
  const avgHours = data.avg_hours_to_fulfill;
  return (
    <div className="space-y-8">
      <div className="grid grid-cols-2 gap-px border border-border-subtle sm:grid-cols-4">
        <AnalyticsKPI
          label="Total orders"
          value={String(data.total_orders)}
        />
        <AnalyticsKPI
          label="Fulfillment rate"
          value={`${(data.fulfillment_rate * 100).toFixed(0)}%`}
          subtitle={`${data.status_totals.fulfilled} fulfilled`}
        />
        <AnalyticsKPI
          label="Cancel rate"
          value={`${(data.cancel_rate * 100).toFixed(0)}%`}
          subtitle={`${data.status_totals.cancelled} cancelled`}
        />
        <AnalyticsKPI
          label="Avg time to fulfill"
          value={avgHours !== null ? formatHours(avgHours) : "—"}
          subtitle={avgHours === null ? "Needs fulfilled orders" : undefined}
        />
      </div>
      <div className="grid grid-cols-1 gap-8 lg:grid-cols-2">
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
            Orders by status
          </p>
          <OrdersStatusChart data={data.status_series} />
        </div>
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
            Orders over time
          </p>
          <OrdersLineChart data={data.orders_series} />
        </div>
      </div>
    </div>
  );
}

// ---------- Customers ----------

function CustomersTab({
  data,
  currency,
}: {
  data: CustomersMetricsResponse;
  currency: string;
}) {
  return (
    <div className="space-y-8">
      <div className="grid grid-cols-2 gap-px border border-border-subtle sm:grid-cols-3">
        <AnalyticsKPI label="New customers" value={String(data.new_total)} />
        <AnalyticsKPI
          label="Returning"
          value={String(data.returning_total)}
        />
        <AnalyticsKPI
          label="Unique buyers"
          value={String(data.unique_buyers)}
        />
      </div>
      <div>
        <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
          New vs returning
        </p>
        <CustomersSegmentChart data={data.series} />
      </div>
      {data.top_customers.length > 0 && (
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
            Top customers
          </p>
          <ul className="mt-3 divide-y divide-border-subtle border border-border-subtle">
            {data.top_customers.map((c) => (
              <li
                key={c.email}
                className="flex items-center justify-between px-5 py-3 text-sm"
              >
                <span className="text-foreground">{c.email}</span>
                <div className="flex items-baseline gap-4 text-foreground-secondary">
                  <span>{c.order_count} orders</span>
                  <span
                    className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-base text-foreground"
                    style={{ fontFeatureSettings: '"tnum" 1' }}
                  >
                    {formatCurrency(c.spend, currency)}
                  </span>
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

// ---------- Reviews ----------

function ReviewsTab({ data }: { data: ReviewsMetricsResponse }) {
  return (
    <div className="space-y-8">
      <div className="grid grid-cols-2 gap-px border border-border-subtle sm:grid-cols-2">
        <AnalyticsKPI
          label="Average rating"
          value={data.avg_rating > 0 ? data.avg_rating.toFixed(2) : "—"}
          subtitle={data.total_reviews > 0 ? `${data.total_reviews} reviews` : undefined}
        />
        <AnalyticsKPI
          label="Reviews in period"
          value={String(data.total_reviews)}
        />
      </div>
      <div className="grid grid-cols-1 gap-8 lg:grid-cols-2">
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
            Rating distribution
          </p>
          <RatingDistributionChart distribution={data.distribution} />
        </div>
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
            Reviews over time
          </p>
          <ReviewsLineChart data={data.series} />
        </div>
      </div>
    </div>
  );
}

// ---------- UI helpers ----------

function SkeletonPanel() {
  return (
    <div
      role="status"
      aria-label="Loading analytics"
      className="animate-pulse space-y-6"
    >
      <div className="grid grid-cols-2 gap-px border border-border-subtle sm:grid-cols-4">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="px-5 py-4">
            <div className="h-2 w-20 rounded bg-[color:var(--ink-900)]/10" />
            <div className="mt-3 h-6 w-24 rounded bg-[color:var(--ink-900)]/10" />
          </div>
        ))}
      </div>
      <div className="h-[260px] rounded bg-[color:var(--ink-900)]/5" />
    </div>
  );
}

function ErrorPanel({ onRetry }: { onRetry: () => void }) {
  return (
    <div
      role="alert"
      className="flex h-[260px] flex-col items-center justify-center gap-3 border border-dashed border-border-subtle"
    >
      <p className="text-sm text-foreground-secondary">
        Analytics unavailable.
      </p>
      <button
        type="button"
        onClick={onRetry}
        className="text-sm font-medium text-[color:var(--moss-700)] hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        Retry
      </button>
    </div>
  );
}

// ---------- Formatters ----------

function formatCurrency(amount: number, currency: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(amount);
  } catch {
    return `${currency} ${amount.toFixed(0)}`;
  }
}

function formatHours(hours: number): string {
  if (hours < 1) return `${Math.round(hours * 60)}m`;
  if (hours < 48) return `${hours.toFixed(1)}h`;
  return `${Math.round(hours / 24)}d`;
}
