import Svg, { Circle, Polyline } from "react-native-svg";
import { theme } from "@/lib/theme";

interface SparklineProps {
  /** Ordered series, oldest → newest. Needs ≥2 points to render. */
  data: number[];
  width?: number;
  height?: number;
  color?: string;
}

/**
 * Minimal editorial sparkline — a single hairline stroke with an emphasised
 * end point, no axes or fill. The revenue trend's one accent moment; kept
 * monochrome-moss so it reads as craft, not a dashboard chart.
 */
export function Sparkline({
  data,
  width = 132,
  height = 36,
  color = theme.colors.accent,
}: SparklineProps) {
  if (!data || data.length < 2) return null;

  const min = Math.min(...data);
  const max = Math.max(...data);
  const range = max - min || 1;
  const stepX = width / (data.length - 1);
  // Inset the curve so the stroke and end dot stay inside the viewport.
  const pad = 3;

  const coords = data.map((value, i) => {
    const x = i * stepX;
    const y = pad + (1 - (value - min) / range) * (height - pad * 2);
    return { x, y };
  });

  const points = coords.map((c) => `${c.x.toFixed(2)},${c.y.toFixed(2)}`).join(" ");
  const last = coords[coords.length - 1]!;

  return (
    <Svg width={width} height={height} accessibilityElementsHidden importantForAccessibility="no-hide-descendants">
      <Polyline
        points={points}
        fill="none"
        stroke={color}
        strokeWidth={1.75}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
      <Circle cx={last.x} cy={last.y} r={2.5} fill={color} />
    </Svg>
  );
}
