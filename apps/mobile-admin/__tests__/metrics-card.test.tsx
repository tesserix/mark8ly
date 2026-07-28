jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

import { StyleSheet } from "react-native";
import { render } from "@testing-library/react-native";

import { MetricsCard } from "@/components/dashboard/MetricsCard";
import { theme } from "@/lib/theme";
import type { DashboardStats } from "@repo/mobile-shared/api/types";

function stats(over: Partial<DashboardStats> = {}): DashboardStats {
  return {
    revenue_today: 312,
    revenue_week: 1104,
    revenue_month: 4280,
    revenue_change_pct: 12.4,
    revenue_trend: [10, 14, 12, 22, 19, 28, 31],
    orders_today: 12,
    orders_pending: 1,
    orders_fulfilled: 38,
    orders_cancelled: 2,
    customers_total: 140,
    customers_new_this_week: 6,
    pending_reviews: 1,
    ...over,
  };
}

/** The trend badge's own label colour — the tone, read off what renders. */
function badgeColor(label: string, tree: ReturnType<typeof render>): string {
  return (StyleSheet.flatten(tree.getByText(label).props.style) as { color: string }).color;
}

describe("MetricsCard — the trend badge", () => {
  it("points up in the moss success tint on a growing month", () => {
    const tree = render(<MetricsCard stats={stats({ revenue_change_pct: 12.4 })} currencyCode="AUD" />);
    expect(tree.getByText("↗ 12.4%")).toBeTruthy();
    expect(badgeColor("↗ 12.4%", tree)).toBe(theme.colors.accent);
  });

  it("points down in the danger tint on a declining month", () => {
    const tree = render(<MetricsCard stats={stats({ revenue_change_pct: -8.2 })} currencyCode="AUD" />);
    expect(tree.getByText("↘ 8.2%")).toBeTruthy();
    expect(badgeColor("↘ 8.2%", tree)).toBe(theme.colors.danger);
  });

  // The defect: `positive = pct >= 0` folded a FLAT month into "growing", so
  // a $0 month with 0.0% change rendered "↗ 0.0%" in the moss success tint —
  // claiming growth where there is none, and spending the screen's one accent
  // on a non-event, on the sign-off screen.
  it("shows no arrow and no accent on a flat month", () => {
    const tree = render(
      <MetricsCard stats={stats({ revenue_change_pct: 0, revenue_month: 0 })} currencyCode="AUD" />,
    );
    expect(tree.queryByText("↗ 0.0%")).toBeNull();
    expect(tree.getByText("0.0%")).toBeTruthy();
    const color = badgeColor("0.0%", tree);
    expect(color).toBe(theme.colors.textTertiary);
    expect(color).not.toBe(theme.colors.accent);
  });

  it("still reads as a decline when a small negative rounds to 0.0%", () => {
    // Only a TRUE zero is flat — the check is on the raw value, so this keeps
    // its downward arrow rather than silently becoming "no change".
    const tree = render(<MetricsCard stats={stats({ revenue_change_pct: -0.04 })} currencyCode="AUD" />);
    expect(tree.getByText("↘ 0.0%")).toBeTruthy();
    expect(badgeColor("↘ 0.0%", tree)).toBe(theme.colors.danger);
  });

  // `NaN === 0` is false and `NaN > 0` is false, so a malformed
  // `revenue_change_pct` fell through to the DECLINE branch and rendered
  // "↘ NaN%" in the danger tint — inventing a bad month out of a parse
  // failure, on the sign-off screen.
  it.each([NaN, Infinity, -Infinity])(
    "says nothing rather than inventing a decline when the change is %p",
    (pct) => {
      const tree = render(<MetricsCard stats={stats({ revenue_change_pct: pct })} currencyCode="AUD" />);
      expect(tree.queryByText(/NaN/)).toBeNull();
      expect(tree.queryByText(/Infinity/)).toBeNull();
      expect(tree.queryByText(/↘/)).toBeNull();
      expect(tree.queryByText(/↗/)).toBeNull();
      expect(tree.getByText("—")).toBeTruthy();
      expect(badgeColor("—", tree)).toBe(theme.colors.textTertiary);
    },
  );

  it("tells a screen reader the change is unavailable rather than a decline", () => {
    const { getByTestId } = render(
      <MetricsCard stats={stats({ revenue_change_pct: NaN })} currencyCode="AUD" />,
    );
    const label = getByTestId("revenue-chart").props.accessibilityLabel as string;
    expect(label).toContain("change unavailable");
    expect(label).not.toContain("down");
  });

  it("tells a screen reader the month is unchanged rather than up", () => {
    const { getByTestId } = render(
      <MetricsCard stats={stats({ revenue_change_pct: 0 })} currencyCode="AUD" />,
    );
    const label = getByTestId("revenue-chart").props.accessibilityLabel as string;
    expect(label).toContain("unchanged");
    expect(label).not.toContain("up ");
  });
});

describe("MetricsCard — money", () => {
  // Whole dollars by TRUNCATION, not rounding — $4,280.75 is $4,280 here, not
  // $4,281. See `formatWholeMoney`: a display column must never read higher
  // than the money actually taken. (This assertion previously expected
  // "$4,281" and so pinned the overstatement.)
  it("renders whole dollars, no cents, in the hero and the subline", () => {
    const { getByText } = render(
      <MetricsCard
        stats={stats({ revenue_month: 4280.75, revenue_today: 312.4, revenue_week: 1104.9 })}
        currencyCode="AUD"
      />,
    );
    expect(getByText("$4,280")).toBeTruthy();
    expect(getByText(/\$312 today/)).toBeTruthy();
    expect(getByText(/\$1,104 this week/)).toBeTruthy();
  });
});
