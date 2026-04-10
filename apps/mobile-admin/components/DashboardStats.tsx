import { View, Text, StyleSheet } from "react-native";
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
  subtitle,
}: {
  label: string;
  value: string;
  subtitle?: string;
}) {
  return (
    <View style={styles.card} accessibilityLabel={`${label}: ${value}${subtitle ? `, ${subtitle}` : ""}`}>
      <Text style={styles.cardLabel}>{label}</Text>
      <Text style={styles.cardValue}>{value}</Text>
      {subtitle && <Text style={styles.cardSubtitle}>{subtitle}</Text>}
    </View>
  );
}

function CompactStat({ label, value }: { label: string; value: number }) {
  return (
    <View style={styles.compactCard} accessibilityLabel={`${label}: ${value}`}>
      <Text style={styles.compactValue}>{value}</Text>
      <Text style={styles.compactLabel}>{label}</Text>
    </View>
  );
}

export function DashboardStats({ stats }: Props) {
  const changePct = stats.revenue_change_pct;
  const changeColor = changePct >= 0 ? theme.colors.accent : theme.colors.danger;
  const changeArrow = changePct >= 0 ? "↑" : "↓";

  return (
    <View style={styles.container}>
      <Text style={styles.sectionTitle}>Revenue</Text>
      <View style={styles.row}>
        <StatCard label="Today" value={formatCurrency(stats.revenue_today)} />
        <StatCard
          label="This week"
          value={formatCurrency(stats.revenue_week)}
        />
        <StatCard
          label="This month"
          value={formatCurrency(stats.revenue_month)}
          subtitle={`${changeArrow} ${Math.abs(changePct).toFixed(1)}%`}
        />
      </View>

      <Text style={styles.sectionTitle}>Orders</Text>
      <View style={styles.row}>
        <CompactStat label="Today" value={stats.orders_today} />
        <CompactStat label="Pending" value={stats.orders_pending} />
        <CompactStat label="Fulfilled" value={stats.orders_fulfilled} />
        <CompactStat label="Cancelled" value={stats.orders_cancelled} />
      </View>

      <Text style={styles.sectionTitle}>Customers</Text>
      <View style={styles.row}>
        <StatCard label="Total" value={String(stats.customers_total)} />
        <StatCard
          label="New this week"
          value={String(stats.customers_new_this_week)}
        />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { gap: theme.spacing.lg },
  sectionTitle: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.colors.text,
    opacity: 0.5,
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  row: { flexDirection: "row", gap: theme.spacing.sm },
  card: {
    flex: 1,
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    padding: theme.spacing.md,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  cardLabel: { fontSize: 12, color: theme.colors.text, opacity: 0.5 },
  cardValue: {
    fontSize: 20,
    fontWeight: "700",
    color: theme.colors.text,
    marginTop: theme.spacing.xs,
  },
  cardSubtitle: { fontSize: 12, color: theme.colors.accent, marginTop: 2 },
  compactCard: {
    flex: 1,
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    padding: 10,
    alignItems: "center",
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  compactValue: { fontSize: 18, fontWeight: "700", color: theme.colors.text },
  compactLabel: {
    fontSize: 11,
    color: theme.colors.text,
    opacity: 0.5,
    marginTop: 2,
  },
});
