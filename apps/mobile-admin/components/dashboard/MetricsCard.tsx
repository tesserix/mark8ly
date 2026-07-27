import { View, StyleSheet } from "react-native";
import { Hairline, StatusBadge, Text } from "@/components/ui";
import { RevenueChart } from "@/components/dashboard/RevenueChart";
import { theme } from "@/lib/theme";
import type { DashboardStats } from "@repo/mobile-shared/api/types";

interface MetricsCardProps {
  stats: DashboardStats;
  /** The active store's currency (e.g. "AUD"). Falls back to a plain number. */
  currencyCode?: string;
}

/**
 * Whole-dollar money for the metrics band. The hero numeral and the
 * today/this-week line both read as headline figures, not ledger entries —
 * cents would add four glyphs of noise to a 44pt serif numeral. Distinct
 * from `lib/money.ts#formatMoney`, which keeps 2dp because it formats real
 * per-order amounts.
 */
function wholeMoney(amount: number, currencyCode?: string): string {
  if (!currencyCode) {
    return new Intl.NumberFormat("en-AU", { maximumFractionDigits: 0 }).format(amount);
  }
  return new Intl.NumberFormat("en-AU", {
    style: "currency",
    currency: currencyCode,
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(amount);
}

function StatCell({ label, value }: { label: string; value: number }) {
  return (
    <View style={styles.cell}>
      <Text preset="h2" color="text" style={styles.cellValue}>
        {String(value)}
      </Text>
      <Text preset="caption" color="textTertiary">
        {label}
      </Text>
    </View>
  );
}

/**
 * The Dashboard's ONE elevated card — a deliberate, bounded exception to the
 * design system's "hairline rules between sections, not bordered cards".
 * Everything below it on the screen is hairline-separated rows on the Paper
 * ground. Do not generalise this into a card grid: the elevation is what
 * creates the foreground/background the screen otherwise lacks, and it only
 * works because it is the only one.
 *
 * The moss-tint trend badge is the brief's explicit instruction ("moss-tint
 * ↗ % badge") and is `StatusBadge`'s existing `success` tone unmodified —
 * a TINT (#E8EEE2 field, #2D4A2B text), never a solid moss fill. A negative
 * month takes `danger` rather than a moss tint pointing downwards.
 */
export function MetricsCard({ stats, currencyCode }: MetricsCardProps) {
  const positive = stats.revenue_change_pct >= 0;
  const changeLabel = `${positive ? "↗" : "↘"} ${Math.abs(stats.revenue_change_pct).toFixed(1)}%`;

  return (
    <View style={styles.card} testID="dashboard-metrics-card">
      <View style={styles.topRow}>
        <Text preset="eyebrow" color="textTertiary">
          This month
        </Text>
        <StatusBadge label={changeLabel} tone={positive ? "success" : "danger"} />
      </View>

      <Text preset="heroNumeral" color="text" style={styles.hero}>
        {wholeMoney(stats.revenue_month, currencyCode)}
      </Text>
      <Text preset="caption" color="textTertiary" style={styles.subline}>
        {wholeMoney(stats.revenue_today, currencyCode)} today ·{" "}
        {wholeMoney(stats.revenue_week, currencyCode)} this week
      </Text>

      <View style={styles.chart}>
        <RevenueChart
          data={stats.revenue_trend}
          accessibilityLabel={`Revenue trend this month, ${wholeMoney(
            stats.revenue_month,
            currencyCode,
          )}, ${positive ? "up" : "down"} ${Math.abs(stats.revenue_change_pct).toFixed(
            1,
          )} percent versus last month`}
        />
      </View>

      <Hairline />

      <View style={styles.strip}>
        <StatCell label="Today" value={stats.orders_today} />
        <StatCell label="Pending" value={stats.orders_pending} />
        <StatCell label="Fulfilled" value={stats.orders_fulfilled} />
        <StatCell label="Cancelled" value={stats.orders_cancelled} />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  card: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radii.xl,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xl,
    paddingBottom: theme.spacing.lg,
    // The one place the elevation scale is spent. Kept low and wide so the
    // card lifts off Paper without reading as a drop shadow.
    shadowColor: theme.colors.text,
    shadowOffset: { width: 0, height: 8 },
    shadowOpacity: 0.1,
    shadowRadius: 14,
    elevation: 3,
  },
  topRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  hero: { marginTop: theme.spacing.xs, fontVariant: ["tabular-nums"] },
  subline: { marginTop: 2 },
  chart: { marginTop: theme.spacing.lg, marginBottom: theme.spacing.md },
  strip: {
    flexDirection: "row",
    marginTop: theme.spacing.md,
    gap: theme.spacing.sm,
  },
  cell: { flex: 1, gap: 1 },
  cellValue: { fontVariant: ["tabular-nums"] },
});
