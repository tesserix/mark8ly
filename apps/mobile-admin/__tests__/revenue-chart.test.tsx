import { act, render, type RenderResult } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import { Circle, Line, Path, Polyline } from "react-native-svg";
import type { ReactTestInstance } from "react-test-renderer";
import { RevenueChart, type RevenueChartProps } from "@/components/dashboard/RevenueChart";
import { theme } from "@/lib/theme";

const LABEL = "Revenue trend, last 7 days";

// Mirrors the pattern __tests__/swipe-row.test.tsx uses for components that
// measure their own width via `onLayout`: RNTL/jest never runs a real
// native layout pass, so the test drives `onLayout` directly to populate
// the width the chart needs before it can compute any geometry.
function renderChart(props: Partial<RevenueChartProps> = {}, measuredWidth = 320) {
  const height = props.height ?? 104;
  const utils = render(<RevenueChart data={[]} accessibilityLabel={LABEL} {...props} />);
  const container = utils.getByTestId("revenue-chart");
  act(() => {
    container.props.onLayout({
      nativeEvent: { layout: { width: measuredWidth, height, x: 0, y: 0 } },
    });
  });
  return { ...utils, container };
}

function containerHeight(container: ReactTestInstance): number | undefined {
  return (StyleSheet.flatten(container.props.style) as { height?: number }).height;
}

function yCoordsOf(pointsAttr: string): number[] {
  return pointsAttr.split(" ").map((pair) => Number(pair.split(",")[1]));
}

// react-native-svg's `extractProps` rewrites colour strings into a processed
// `{payload, type}` colour object before they land on the underlying native
// host node, and geometry props (`points`, `d`) don't survive onto that host
// node at all — so querying by testID (which RNTL resolves to the deepest
// host component) returns props that don't match what the component
// actually passed. Querying by component TYPE instead reads `.props` off
// the composite fiber, which still holds the original, unprocessed values.
function typeCount<P>(utils: RenderResult, type: React.ComponentType<P>): number {
  return utils.UNSAFE_queryAllByType(type).length;
}

function lastOfType<P>(utils: RenderResult, type: React.ComponentType<P>): ReactTestInstance {
  const matches = utils.UNSAFE_queryAllByType(type);
  const match = matches[matches.length - 1];
  if (!match) {
    throw new Error(`No instance of ${type.toString()} found`);
  }
  return match;
}

// react-native-svg's `Polyline` renders itself AS a `Path` internally
// (Polyline.js: `React.createElement(Path, { d: ..., ...props })`), so a
// tree that has both the chart's own area `<Path>` and a `<Polyline>` line
// contains TWO `Path` composites — the area, then Polyline's internal one
// (which inherits `fill="none"` from the Polyline props). The area is
// always the first `Path` in render order since it's rendered before the
// line in RevenueChart's JSX.
function areaPathOf(utils: RenderResult): ReactTestInstance {
  const match = utils.UNSAFE_getAllByType(Path)[0];
  if (!match) {
    throw new Error("No area Path found");
  }
  return match;
}

describe("RevenueChart", () => {
  describe("accessibility", () => {
    it("announces the chart as a single labeled image instead of hiding it from screen readers", () => {
      const { container } = renderChart({ data: [10, 20, 15, 30] });
      expect(container.props.accessible).toBe(true);
      expect(container.props.accessibilityRole).toBe("image");
      expect(container.props.accessibilityLabel).toBe(LABEL);
      // The interface REQUIRES accessibilityLabel, meaning this chart is
      // meant to be announced — unlike Sparkline, which hides itself
      // entirely via these two props.
      expect(container.props.accessibilityElementsHidden).not.toBe(true);
      expect(container.props.importantForAccessibility).not.toBe("no-hide-descendants");
    });
  });

  describe("edge case: empty array", () => {
    it("renders no area, line, halo, or dot", () => {
      const utils = renderChart({ data: [] });
      expect(typeCount(utils, Path)).toBe(0);
      expect(typeCount(utils, Polyline)).toBe(0);
      expect(typeCount(utils, Circle)).toBe(0);
    });

    it("still reserves the full 104pt height and renders three gridlines", () => {
      const utils = renderChart({ data: [] });
      expect(typeCount(utils, Line)).toBe(3);
      expect(containerHeight(utils.container)).toBe(104);
    });
  });

  describe("edge case: single point", () => {
    it("renders only a centered dot and halo — no area or line to draw a trend from", () => {
      const utils = renderChart({ data: [42] }, 320);
      expect(typeCount(utils, Path)).toBe(0);
      expect(typeCount(utils, Polyline)).toBe(0);

      const circles = utils.UNSAFE_queryAllByType(Circle);
      expect(circles).toHaveLength(2); // halo + dot
      for (const circle of circles) {
        expect(circle.props.cx).toBe(160); // width / 2
      }
    });
  });

  describe("edge case: all-zero series", () => {
    it("draws a flat line through the vertical middle instead of a divide-by-zero NaN", () => {
      const utils = renderChart({ data: [0, 0, 0, 0] }, 300);
      const line = lastOfType(utils, Polyline);
      const area = areaPathOf(utils);

      expect(line.props.points as string).not.toMatch(/NaN/);
      expect(area.props.d as string).not.toMatch(/NaN/);

      const yValues = yCoordsOf(line.props.points as string);
      expect(new Set(yValues.map((y) => y.toFixed(2))).size).toBe(1);
      expect(yValues[0]).toBeCloseTo(52); // height (104) / 2
    });
  });

  describe("normal series", () => {
    const DATA = [10, 40, 25, 60];

    it("draws a moss area + stroke line with one point per data value", () => {
      const utils = renderChart({ data: DATA }, 300);
      const line = lastOfType(utils, Polyline);
      const area = areaPathOf(utils);

      expect(line.props.stroke).toBe(theme.colors.accent);
      expect(line.props.strokeWidth).toBe(2.25);
      expect(area.props.fill).toBe(theme.colors.accentTint);
      expect((line.props.points as string).split(" ")).toHaveLength(DATA.length);
    });

    it("places the endpoint dot and its halo at the last data point, dot smaller than halo", () => {
      const utils = renderChart({ data: DATA }, 300);
      const circles = utils.UNSAFE_queryAllByType(Circle);
      expect(circles).toHaveLength(2);
      const [halo, dot] = circles as [ReactTestInstance, ReactTestInstance];

      // Last point sits inset from the measured width by PAD_X (11 = halo
      // radius 9 + 2), not flush against the edge — otherwise the halo
      // clips into a quarter-circle against the card boundary (caught on a
      // real device screenshot, not by any prop assertion).
      expect(dot.props.cx).toBe(289);
      expect(halo.props.cx).toBe(289);
      expect(dot.props.fill).toBe(theme.colors.accent);
      expect(halo.props.fill).toBe(theme.colors.accentTint);
      expect(dot.props.r as number).toBeLessThan(halo.props.r as number);
    });

    it("draws higher values nearer the top of the chart (smaller y)", () => {
      const utils = renderChart({ data: [10, 60] }, 200);
      const line = lastOfType(utils, Polyline);
      const [firstY, secondY] = yCoordsOf(line.props.points as string);
      expect(secondY).toBeLessThan(firstY as number);
    });

    it("renders exactly three gridlines using the shared hairline token, not a new colour", () => {
      const utils = renderChart({ data: DATA }, 300);
      const gridlines = utils.UNSAFE_queryAllByType(Line);
      expect(gridlines).toHaveLength(3);
      for (const gridline of gridlines) {
        expect(gridline.props.stroke).toBe(theme.colors.hairline);
      }
    });

    it("stretches the SVG to the measured container width, since the interface has no width prop", () => {
      const { getByTestId } = renderChart({ data: DATA }, 300);
      expect(getByTestId("revenue-chart-svg").props.width).toBe(300);
    });
  });

  describe("height prop", () => {
    it("defaults to 104pt", () => {
      const { container } = renderChart({ data: [1, 2] });
      expect(containerHeight(container)).toBe(104);
    });

    it("honours a custom height", () => {
      const { container } = renderChart({ data: [1, 2], height: 60 }, 200);
      expect(containerHeight(container)).toBe(60);
    });
  });
});
