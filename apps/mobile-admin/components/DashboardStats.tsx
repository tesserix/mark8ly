import { View, StyleSheet } from "react-native";
import { ArrowDownRight, ArrowUpRight } from "lucide-react-native";
import { Card, Text } from "@/components/ui";
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

function StatCard({
  label,
  value,
  trend,
}: {
  label: string;
  value: string;
  trend?: { pct: number };
}) {
  const positive = trend ? trend.pct >= 0 : false;
  return (
    <Card style={styles.card} padding="md">
      <Text preset="eyebrow" color="textTertiary">
        {label}
      </Text>
      <Text preset="h2" color="text" style={styles.cardValue}>
        {value}
      </Text>
      {trend ? (
        <View style={styles.trendRow}>
          {positive ? (
            <ArrowUpRight size={12} color={theme.colors.accent} strokeWidth={2.25} />
          ) : (
            <ArrowDownRight size={12} color={theme.colors.danger} strokeWidth={2.25} />
          )}
          <Text preset="caption" color={positive ? "accent" : "danger"}>
            {Math.abs(trend.pct).toFixed(1)}%
          </Text>
        </View>
      ) : null}
    </Card>
  );
}

function CompactStat({ label, value }: { label: string; value: number }) {
  return (
    <View style={styles.compactCard}>
      <Text preset="h3" color="text">
        {value}
      </Text>
      <Text preset="caption" color="textTertiary" style={styles.compactLabel}>
        {label}
      </Text>
    </View>
  );
}

export function DashboardStats({ stats }: Props) {
  return (
    <View style={styles.container}>
      <View style={styles.row}>
        <StatCard label="Today" value={formatCurrency(stats.revenue_today)} />
        <StatCard label="This week" value={formatCurrency(stats.revenue_week)} />
      </View>
      <View style={styles.row}>
        <StatCard
          label="This month"
          value={formatCurrency(stats.revenue_month)}
          trend={{ pct: stats.revenue_change_pct }}
        />
        <StatCard label="Customers" value={String(stats.customers_total)} />
      </View>

      <View style={[styles.compactStrip, styles.row]}>
        <CompactStat label="Today" value={stats.orders_today} />
        <CompactStat label="Pending" value={stats.orders_pending} />
        <CompactStat label="Fulfilled" value={stats.orders_fulfilled} />
        <CompactStat label="Cancelled" value={stats.orders_cancelled} />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { gap: theme.spacing.sm },
  row: { flexDirection: "row", gap: theme.spacing.sm },
  card: { flex: 1, minHeight: 92 },
  cardValue: { marginTop: theme.spacing.xs },
  trendRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 4,
    marginTop: theme.spacing.xs,
  },
  compactStrip: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    padding: theme.spacing.md,
    marginTop: theme.spacing.xs,
  },
  compactCard: {
    flex: 1,
    alignItems: "center",
    paddingVertical: theme.spacing.xs,
  },
  compactLabel: { marginTop: 2 },
});
