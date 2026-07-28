import { View, StyleSheet, useWindowDimensions } from "react-native";
import { Hairline, StatusBadge, Text } from "@/components/ui";
import { MAX_FONT_SCALE } from "@/components/ui/Text";
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

/**
 * Four-up at the default text size, TWO-up above it.
 *
 * The strip is four equal columns of the card's inner width, and that width
 * is fixed by the screen gutter — it cannot grow to meet the labels. On a
 * 390pt screen the inner width is 390 − 2×20 (screen gutter) − 2×20 (card
 * padding) = 310, so a quarter column is ~72pt against the widest label,
 * "Cancelled", which needs ~58pt at the `caption` 13pt. That is ~1.2× of
 * headroom and no more: measured on device at the 2× cap, `Pending`,
 * `Fulfilled` and `Cancelled` all broke MID-WORD (`Pendi`/`ng`,
 * `Fulfill`/`ed`, `Cance`/`lled`).
 *
 * So the strip REFLOWS rather than shrinking: at half width (~151pt) the
 * same "Cancelled" needs ~116pt at the capped 26pt and fits on one line.
 * Four columns and two columns are the only options offered — three would
 * leave a 3+1 orphan row, which reads as a layout bug rather than a grid.
 *
 * Reflow, not a tighter local cap, because the labels are the part that has
 * to give and there is nothing wrong with the SIZE they reach — only with
 * the box. Capping them harder than the global 2 would make the strip the
 * one place on the screen that refuses a merchant's chosen text size.
 *
 * Exported so the breakpoint is testable without mocking RN's Dimensions —
 * same pattern as `FilterChips.chipHeightsFor` and
 * `CollapsingHeader.headerHeightsFor`.
 */
export const STRIP_REFLOW_SCALE = 1.2;

export function stripColumnsFor(fontScale: number): 2 | 4 {
  const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
  return scale >= STRIP_REFLOW_SCALE ? 2 : 4;
}

/**
 * `flexBasis` percentages pick the column count purely through flex wrap:
 * 4×22% clears 100% only at four per row, 2×40% only at two. `flexGrow: 1`
 * then spreads the slack so the columns stay equal.
 */
const CELL_BASIS: Record<2 | 4, `${number}%`> = { 4: "22%", 2: "40%" };

function StatCell({
  label,
  value,
  columns,
}: {
  label: string;
  value: number;
  columns: 2 | 4;
}) {
  return (
    <View style={[styles.cell, { flexBasis: CELL_BASIS[columns] }]}>
      {/*
        A COUNT IS ONE TOKEN, exactly as the hero numeral is. Measured at the
        2× cap, `124` wrapped to `12` / `4` — two numbers where the merchant
        has one, on the strip they read their day's orders off. Shrinking to
        fit is the right trade for a figure: it stays a single legible number
        instead of becoming a different, wrong-looking pair.
      */}
      <Text
        preset="h2"
        color="text"
        style={styles.cellValue}
        numberOfLines={1}
        adjustsFontSizeToFit
        minimumFontScale={0.5}
      >
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
  // `useWindowDimensions` (not `PixelRatio.getFontScale()`) so the strip
  // re-flows when the merchant changes their text size with the app
  // foregrounded — a static read would leave it four-up and broken.
  const { fontScale } = useWindowDimensions();
  const columns = stripColumnsFor(fontScale);

  return (
    <View style={styles.card} testID="dashboard-metrics-card">
      <View style={styles.topRow}>
        <Text preset="eyebrow" color="textTertiary">
          This month
        </Text>
        <StatusBadge label={change.label} tone={change.tone} />
      </View>

      {/*
        MONEY READS AS ONE TOKEN. Uncapped this wrapped mid-figure at the 2×
        cap — `$612,4` on one line and `00` on the next, which is not a
        smaller number but a DIFFERENT one at a glance. `numberOfLines={1}`
        forbids the break and `adjustsFontSizeToFit` buys the room back by
        shrinking, which is the correct direction to give for a display
        numeral whose whole job is to be read in one movement.

        `minimumFontScale={0.5}` is the accessible floor, and it is chosen
        rather than guessed: RN floors at `fontSize × minimumFontScale`, and
        at the 2× cap the styled size is 88, so the floor is 44 — EXACTLY
        the size the hero has at the default text size. A merchant who turns
        text up can therefore never end up with a hero numeral smaller than
        the one they started with, no matter how many digits their month has.
      */}
      <Text
        preset="heroNumeral"
        color="text"
        style={styles.hero}
        numberOfLines={1}
        adjustsFontSizeToFit
        minimumFontScale={0.5}
      >
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
        <StatCell label="Today" value={stats.orders_today} columns={columns} />
        <StatCell label="Pending" value={stats.orders_pending} columns={columns} />
        <StatCell label="Fulfilled" value={stats.orders_fulfilled} columns={columns} />
        <StatCell label="Cancelled" value={stats.orders_cancelled} columns={columns} />
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
    // Wrap is what turns `CELL_BASIS` into a column count. `rowGap` is larger
    // than the column gap so the two rows of a reflowed strip read as rows
    // rather than as a block of eight loose values.
    flexWrap: "wrap",
    marginTop: theme.spacing.md,
    columnGap: theme.spacing.sm,
    rowGap: theme.spacing.lg,
  },
  cell: { flexGrow: 1, gap: 1 },
  cellValue: { fontVariant: ["tabular-nums"] },
});
