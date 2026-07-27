import { useCallback, useMemo, useState } from "react";
import { StyleSheet, View, type LayoutChangeEvent } from "react-native";
import Svg, { Circle, Line, Path, Polyline } from "react-native-svg";
import { theme } from "@/lib/theme";

export interface RevenueChartProps {
  /** revenue_trend from the dashboard payload — oldest → newest, any length. */
  data: number[];
  height?: number;
  accessibilityLabel: string;
}

const DEFAULT_HEIGHT = 104;
const STROKE_WIDTH = 2.25;
const DOT_RADIUS = 3.5;
const HALO_RADIUS = 9;
// Keeps the curve, stroke, and endpoint halo inside the viewport — must
// clear HALO_RADIUS at the top/bottom edge or the halo clips at the
// extremes of the range.
const PAD_Y = 12;
// Same reasoning horizontally: the last data point (where the endpoint dot
// and halo always sit) lands at the right edge of the series. Without this,
// SVG's default overflow:hidden clips the halo into a quarter-circle at the
// card's edge — caught on-device with real varying data, not by any unit
// test (device screenshot revealed a clipped dot that a11y/colour
// assertions can't catch).
const PAD_X = HALO_RADIUS + 2;
const GRIDLINE_FRACTIONS = [0.25, 0.5, 0.75] as const;

interface Point {
  x: number;
  y: number;
}

interface ChartGeometry {
  endpoint: Point;
  /** Null when there are fewer than 2 points — nothing to draw a trend from. */
  areaPath: string | null;
  linePoints: string | null;
}

/**
 * Maps a data series onto SVG coordinates for a `width`×`height` viewport.
 *
 * Handles the three edge cases real stores hit:
 * - Empty series → `null` (nothing to draw).
 * - Single point → no line to compute, so it's centered horizontally as a
 *   standalone reading rather than pinned to x=0.
 * - Flat series (including all-zero) would divide by zero computing
 *   `(value - min) / range`. Rather than defaulting `range` to 1 (which
 *   biases the flat line toward whichever edge the shared value happens to
 *   land on), every point normalizes to 0.5 so a flat series always draws a
 *   flat line straight through the vertical middle.
 */
function buildGeometry(data: number[], width: number, height: number): ChartGeometry | null {
  if (data.length === 0 || width <= 0) return null;

  const min = Math.min(...data);
  const max = Math.max(...data);
  const range = max - min;
  const bottom = height - PAD_Y;
  const usable = Math.max(bottom - PAD_Y, 0);
  const normalize = (value: number) => (range === 0 ? 0.5 : (value - min) / range);
  const yFor = (value: number) => bottom - normalize(value) * usable;

  if (data.length === 1) {
    const point = { x: width / 2, y: yFor(data[0] as number) };
    return { endpoint: point, areaPath: null, linePoints: null };
  }

  const usableWidth = Math.max(width - 2 * PAD_X, 0);
  const stepX = usableWidth / (data.length - 1);
  const points: Point[] = data.map((value, i) => ({ x: PAD_X + i * stepX, y: yFor(value) }));
  const first = points[0] as Point;
  const last = points[points.length - 1] as Point;

  const linePoints = points.map((p) => `${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(" ");
  const areaPath = [
    `M ${first.x.toFixed(2)},${bottom.toFixed(2)}`,
    ...points.map((p) => `L ${p.x.toFixed(2)},${p.y.toFixed(2)}`),
    `L ${last.x.toFixed(2)},${bottom.toFixed(2)}`,
    "Z",
  ].join(" ");

  return { endpoint: last, areaPath, linePoints };
}

/**
 * The Dashboard's one visual centrepiece — a 104pt moss area chart replacing
 * the old `Sparkline`. Moss-tint fill, moss stroke, three faint gridlines,
 * and an emphasised endpoint dot with a soft halo (built from the existing
 * accent/accentTint tokens rather than a new translucent colour — no fourth
 * colour introduced).
 *
 * No `width` prop exists on the interface, so the chart measures its own
 * container via `onLayout` and draws in real pixel coordinates (matching
 * `components/ui/SwipeRow.tsx`'s pattern) rather than stretching a fixed
 * viewBox with `preserveAspectRatio="none"` — that would scale x and y
 * non-uniformly and render the endpoint dot/halo as ellipses instead of
 * circles.
 *
 * Unlike `Sparkline` (which hides itself from screen readers entirely via
 * `accessibilityElementsHidden`), this component's REQUIRED
 * `accessibilityLabel` means it is meant to be announced: `accessible` +
 * `accessibilityRole="image"` collapse the SVG internals into one announced
 * element carrying that label, instead of removing the subtree from the
 * accessibility tree.
 */
export function RevenueChart({ data, height = DEFAULT_HEIGHT, accessibilityLabel }: RevenueChartProps) {
  const [width, setWidth] = useState(0);

  const handleLayout = useCallback((event: LayoutChangeEvent) => {
    const measured = event.nativeEvent.layout.width;
    setWidth((prev) => (prev === measured ? prev : measured));
  }, []);

  const geometry = useMemo(() => buildGeometry(data, width, height), [data, width, height]);

  return (
    <View
      testID="revenue-chart"
      onLayout={handleLayout}
      style={[styles.container, { height }]}
      accessible
      accessibilityRole="image"
      accessibilityLabel={accessibilityLabel}
    >
      {width > 0 ? (
        <Svg width={width} height={height} testID="revenue-chart-svg">
          {GRIDLINE_FRACTIONS.map((fraction, index) => (
            <Line
              key={fraction}
              testID={`revenue-chart-gridline-${index}`}
              x1={0}
              x2={width}
              y1={height * fraction}
              y2={height * fraction}
              stroke={theme.colors.hairline}
              strokeWidth={1}
            />
          ))}
          {geometry?.areaPath ? (
            <Path testID="revenue-chart-area" d={geometry.areaPath} fill={theme.colors.accentTint} />
          ) : null}
          {geometry?.linePoints ? (
            <Polyline
              testID="revenue-chart-line"
              points={geometry.linePoints}
              fill="none"
              stroke={theme.colors.accent}
              strokeWidth={STROKE_WIDTH}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          ) : null}
          {geometry ? (
            <>
              <Circle
                testID="revenue-chart-halo"
                cx={geometry.endpoint.x}
                cy={geometry.endpoint.y}
                r={HALO_RADIUS}
                fill={theme.colors.accentTint}
              />
              <Circle
                testID="revenue-chart-dot"
                cx={geometry.endpoint.x}
                cy={geometry.endpoint.y}
                r={DOT_RADIUS}
                fill={theme.colors.accent}
              />
            </>
          ) : null}
        </Svg>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { width: "100%" },
});
