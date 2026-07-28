import { View, StyleSheet } from "react-native";
import { Hairline, StatusBadge, Text } from "@/components/ui";
import { RevenueChart } from "@/components/dashboard/RevenueChart";
import { formatWholeMoney } from "@/lib/money";
import { theme } from "@/lib/theme";
import type { StatusTone } from "@/components/ui";
import type { DashboardStats } from "@repo/mobile-shared/api/types";

interface MetricsCardProps {
  stats: DashboardStats;
  /** The active store's currency (e.g. "AUD"). Falls back to a plain number. */
  currencyCode?: string;
}

/**
 * Month-on-month change → the badge's arrow, tone and screen-reader wording.
 *
 * EXACTLY zero is its own case, not a rounding of "positive". A $0 month with
 * a 0.0% change used to render "↗ 0.0%" in the moss success tint — claiming
 * growth where there is none, and spending the screen's one accent on a
 * non-event, on the sign-off screen. Zero gets the muted tone and NO arrow:
 * there is no direction to point in.
 *
 * Note the ordering: the check is on the raw value, so -0.04 rounding to
 * "0.0%" still reads as a (muted-adjacent) decline rather than silently
 * becoming "no change" — only a true 0 is flat.
 *
 * A non-finite `pct` (a malformed payload, or a server dividing by a zero
 * previous month) is neither growth nor decline and must not be guessed at:
 * `NaN === 0` and `NaN > 0` are both false, so it used to fall through to the
 * DECLINE branch and render "↘ NaN%" in the danger tint — inventing a bad
 * month out of a parse failure. An em dash in the muted tone says "we don't
 * know", which is the truth.
 */
function changeBadge(pct: number): { label: string; tone: StatusTone; spoken: string } {
  if (!Number.isFinite(pct)) {
    return { label: "—", tone: "muted", spoken: "change unavailable" };
  }
  const magnitude = `${Math.abs(pct).toFixed(1)}%`;
  if (pct === 0) return { label: magnitude, tone: "muted", spoken: "unchanged" };
  if (pct > 0) return { label: `↗ ${magnitude}`, tone: "success", spoken: `up ${magnitude}` };
  return { label: `↘ ${magnitude}`, tone: "danger", spoken: `down ${magnitude}` };
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
 * month takes `danger` rather than a moss tint pointing downwards, and a flat
 * month takes `muted` — see `changeBadge`.
 */
export function MetricsCard({ stats, currencyCode }: MetricsCardProps) {
  const change = changeBadge(stats.revenue_change_pct);

  return (
    <View style={styles.card} testID="dashboard-metrics-card">
      <View style={styles.topRow}>
        <Text preset="eyebrow" color="textTertiary">
          This month
        </Text>
        <StatusBadge label={change.label} tone={change.tone} />
      </View>

      <Text preset="heroNumeral" color="text" style={styles.hero}>
        {formatWholeMoney(stats.revenue_month, currencyCode)}
      </Text>
      <Text preset="caption" color="textTertiary" style={styles.subline}>
        {formatWholeMoney(stats.revenue_today, currencyCode)} today ·{" "}
        {formatWholeMoney(stats.revenue_week, currencyCode)} this week
      </Text>

      <View style={styles.chart}>
        <RevenueChart
          data={stats.revenue_trend}
          accessibilityLabel={`Revenue trend this month, ${formatWholeMoney(
            stats.revenue_month,
            currencyCode,
          )}, ${change.spoken} versus last month`}
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
