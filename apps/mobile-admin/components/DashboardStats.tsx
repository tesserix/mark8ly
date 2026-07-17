import { View, StyleSheet } from "react-native";
import { ArrowDownRight, ArrowUpRight } from "lucide-react-native";
import { Hairline, Text } from "@/components/ui";
import { Sparkline } from "@/components/dashboard/Sparkline";
import { theme } from "@/lib/theme";
import type { DashboardStats as DashboardStatsType } from "@repo/mobile-shared/api/types";

interface Props {
  stats: DashboardStatsType;
  /** The active store's currency (e.g. "AUD"). Falls back to a plain number. */
  currencyCode?: string;
}

function formatCurrency(amount: number, currencyCode?: string): string {
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

function StatRow({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: string;
}) {
  return (
    <View style={styles.row}>
      <Text preset="body" color="textSecondary">
        {label}
      </Text>
      <View style={styles.rowValue}>
        {hint ? (
          <Text preset="caption" color="accent">
            {hint}
          </Text>
        ) : null}
        <Text preset="h3" color="text">
          {value}
        </Text>
      </View>
    </View>
  );
}

export function DashboardStats({ stats, currencyCode }: Props) {
  const positive = stats.revenue_change_pct >= 0;

  return (
    <View style={styles.container}>
      {/* Hero: this month's revenue is the headline number — large serif
          numeral, left-aligned, with its own trend line. */}
      <View style={styles.hero}>
        <Text preset="eyebrow" color="textTertiary">
          This month
        </Text>
        <Text preset="display" color="text" style={styles.heroValue}>
          {formatCurrency(stats.revenue_month, currencyCode)}
        </Text>
        <View style={styles.trendRow}>
          <View style={styles.trend}>
            {positive ? (
              <ArrowUpRight size={14} color={theme.colors.text} strokeWidth={2.25} />
            ) : (
              <ArrowDownRight size={14} color={theme.colors.danger} strokeWidth={2.25} />
            )}
            <Text preset="caption" color={positive ? "text" : "danger"}>
              {Math.abs(stats.revenue_change_pct).toFixed(1)}%
            </Text>
            <Text preset="caption" color="textTertiary">
              vs last month
            </Text>
          </View>
          <Sparkline data={stats.revenue_trend} />
        </View>
      </View>

      <Hairline style={styles.heroDivider} />

      {/* Secondary metrics: left-aligned, hairline-separated rows — smaller
          than the hero, not an equal-size card grid. */}
      <View style={styles.list}>
        <StatRow label="Today" value={formatCurrency(stats.revenue_today, currencyCode)} />
        <Hairline />
        <StatRow label="This week" value={formatCurrency(stats.revenue_week, currencyCode)} />
        <Hairline />
        <StatRow
          label="Customers"
          value={String(stats.customers_total)}
          hint={
            stats.customers_new_this_week > 0
              ? `+${stats.customers_new_this_week} this week`
              : undefined
          }
        />
      </View>

      <View style={styles.list}>
        <Text preset="eyebrow" color="textTertiary" style={styles.listLabel}>
          Orders
        </Text>
        <StatRow label="Today" value={String(stats.orders_today)} />
        <Hairline />
        <StatRow label="Pending" value={String(stats.orders_pending)} />
        <Hairline />
        <StatRow label="Fulfilled" value={String(stats.orders_fulfilled)} />
        <Hairline />
        <StatRow label="Cancelled" value={String(stats.orders_cancelled)} />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { gap: theme.spacing.lg },
  hero: { gap: theme.spacing.xs },
  heroValue: { marginTop: theme.spacing.xs },
  trendRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    marginTop: theme.spacing.xs,
  },
  trend: {
    flexDirection: "row",
    alignItems: "center",
    gap: 4,
  },
  heroDivider: { marginTop: theme.spacing.xs },
  list: { gap: 0 },
  listLabel: { marginBottom: theme.spacing.xs },
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: theme.spacing.sm,
  },
  rowValue: {
    flexDirection: "row",
    alignItems: "baseline",
    gap: theme.spacing.sm,
  },
});
