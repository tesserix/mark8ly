import { StyleSheet } from "react-native";
import { render } from "@testing-library/react-native";

import { StatusBadge, type StatusTone } from "@/components/ui/StatusBadge";
import { theme } from "@/lib/theme";

const path = require("path");
declare const __dirname: string;

/** sRGB relative luminance, WCAG 2.1 §relative-luminance. */
function luminance(hex: string): number {
  const c = hex.replace("#", "");
  const channels = [0, 2, 4]
    .map((i) => parseInt(c.substr(i, 2), 16) / 255)
    .map((v) => (v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)));
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

function contrastRatio(fg: string, bg: string): number {
  const a = luminance(fg);
  const b = luminance(bg);
  const [hi, lo] = a > b ? [a, b] : [b, a];
  return (hi + 0.05) / (lo + 0.05);
}

function badgeStyle(tone: StatusTone) {
  const { getByLabelText } = render(<StatusBadge label="Sample" tone={tone} />);
  return StyleSheet.flatten(getByLabelText("Status: Sample").props.style) as {
    backgroundColor?: string;
    borderWidth?: number;
  };
}

function labelColor(tone: StatusTone): string {
  const { getByText } = render(<StatusBadge label="Sample" tone={tone} />);
  return (StyleSheet.flatten(getByText("Sample").props.style) as { color: string }).color;
}

describe("StatusBadge — the four status tones share one tint construction", () => {
  // The whole point of the 2026-07-28 change: `danger` was a SOLID crimson
  // fill with inverse text — the only non-tint among the four — and `muted`
  // was a transparent chip with a hairline, the only one with no field at
  // all. A solid chip on a Paper screen reads as an alert and steals the
  // view's attention budget from the one intended accent.
  const TINT_TONES: StatusTone[] = ["success", "warning", "danger", "muted"];

  it.each(TINT_TONES)("gives %s a filled tint field and no border", (tone) => {
    const style = badgeStyle(tone);
    expect(style.backgroundColor).toBeTruthy();
    expect(style.backgroundColor).not.toBe("transparent");
    expect(style.borderWidth).toBe(0);
  });

  it("paints danger as oxblood on the blood tint, not white on solid crimson", () => {
    expect(badgeStyle("danger").backgroundColor).toBe(theme.colors.dangerTint);
    expect(labelColor("danger")).toBe(theme.colors.danger);
    expect(labelColor("danger")).not.toBe(theme.colors.inverse);
  });

  it("paints muted as tertiary ink on the sink surface, not a transparent chip", () => {
    expect(badgeStyle("muted").backgroundColor).toBe(theme.colors.sink);
    expect(labelColor("muted")).toBe(theme.colors.textTertiary);
  });
});

describe("StatusBadge — WCAG AA contrast, computed not eyeballed", () => {
  const PAIRS: [StatusTone, string, string][] = [
    ["neutral", theme.colors.inverse, theme.colors.text],
    ["success", theme.colors.accent, theme.colors.accentTint],
    ["warning", theme.colors.warningInk, theme.colors.warningTint],
    ["danger", theme.colors.danger, theme.colors.dangerTint],
    ["muted", theme.colors.textTertiary, theme.colors.sink],
  ];

  it.each(PAIRS)("clears 4.5:1 for %s", (tone, fg, bg) => {
    // Guards against the pair being read off a moved/renamed token and
    // silently resolving to undefined.
    expect(fg).toMatch(/^#[0-9A-Fa-f]{6}$/);
    expect(bg).toMatch(/^#[0-9A-Fa-f]{6}$/);
    expect(labelColor(tone)).toBe(fg);
    expect(badgeStyle(tone).backgroundColor).toBe(bg);
    expect(contrastRatio(fg, bg)).toBeGreaterThanOrEqual(4.5);
  });
});

describe("StatusBadge — token sources agree", () => {
  // eslint-disable-next-line @typescript-eslint/no-var-requires
  const twConfig = require(path.resolve(__dirname, "../tailwind.config.js"));
  const twColors = twConfig.theme.extend.colors;

  it("keeps the danger tint identical in lib/theme.ts and tailwind.config.js", () => {
    expect(theme.colors.dangerTint).toBe("#F6E4E1");
    expect(twColors.danger.tint).toBe("#F6E4E1");
  });

  it("keeps the warning tint identical in lib/theme.ts and tailwind.config.js", () => {
    expect(theme.colors.warningTint).toBe(twColors.warning.tint);
  });

  it("keeps the muted field on the same sink token both sources use", () => {
    expect(theme.colors.sink).toBe(twColors.paper.sink);
  });
});
