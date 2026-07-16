import { View, StyleSheet } from "react-native";
import { ArrowDownRight, ArrowUpRight } from "lucide-react-native";
import { Hairline, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { DashboardStats as DashboardStatsType } from "@repo/mobile-shared/api/types";

interface Props {
  stats: DashboardStatsType;
}

function formatCurrency(amount: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(amount);
}

function StatRow({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.row}>
      <Text preset="body" color="textSecondary">
        {label}
      </Text>
      <Text preset="h3" color="text">
        {value}
      </Text>
    </View>
  );
}

export function DashboardStats({ stats }: Props) {
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
          {formatCurrency(stats.revenue_month)}
        </Text>
        <View style={styles.trendRow}>
          {positive ? (
            <ArrowUpRight size={14} color={theme.colors.text} strokeWidth={2.25} />
          ) : (
            <ArrowDownRight size={14} color={theme.colors.danger} strokeWidth={2.25} />
          )}
          <Text preset="caption" color={positive ? "text" : "danger"}>
            {Math.abs(stats.revenue_change_pct).toFixed(1)}%
          </Text>
        </View>
      </View>

      <Hairline style={styles.heroDivider} />

      {/* Secondary metrics: left-aligned, hairline-separated rows — smaller
          than the hero, not an equal-size card grid. */}
      <View style={styles.list}>
        <StatRow label="Today" value={formatCurrency(stats.revenue_today)} />
        <Hairline />
        <StatRow label="This week" value={formatCurrency(stats.revenue_week)} />
        <Hairline />
        <StatRow label="Customers" value={String(stats.customers_total)} />
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
    gap: 4,
    marginTop: 2,
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
});
