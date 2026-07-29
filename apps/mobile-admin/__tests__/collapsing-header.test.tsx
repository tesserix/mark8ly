// `CollapsingHeader` gained a back chevron (Task 1's `onBack`), so it now
// imports an icon from lucide-react-native's ESM build (`dist/esm/*.mjs`),
// which jest-expo's default `transformIgnorePatterns` does not transform.
// Same one-line Proxy mock ~20 other suites in this directory already use —
// the icon is a decorative glyph inside a labelled button, so rendering
// nothing for it costs no assertion.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

// react-native-reanimated 4.x's real module (and even its shipped mock.js)
// requires the native Worklets module at import time, which throws under
// jest ("Native part of Worklets doesn't seem to be initialized"). Hand-roll
// a minimal virtual mock covering only what CollapsingHeader uses — enough
// to observe the interpolated opacity/height the tests below assert on.
// `interpolate` is a real (simplified, 2-point, clamped) linear
// implementation rather than a stub, because the behaviour under test IS
// the interpolation math (collapsed vs expanded crossover at offset 64).
jest.mock("react-native-reanimated", () => {
  const { View } = require("react-native");

  function interpolate(
    value: number,
    inputRange: [number, number],
    outputRange: [number, number],
  ) {
    const [inMin, inMax] = inputRange;
    const [outMin, outMax] = outputRange;
    const t = Math.max(0, Math.min(1, (value - inMin) / (inMax - inMin)));
    return outMin + t * (outMax - outMin);
  }

  return {
    __esModule: true,
    default: { View },
    Extrapolation: { CLAMP: "clamp" },
    interpolate,
    useAnimatedStyle: (factory: () => unknown) => factory(),
    useDerivedValue: (factory: () => number) => ({ value: factory() }),
    useReducedMotion: jest.fn(() => false),
  };
});

import { Dimensions, StyleSheet, Text as RNText } from "react-native";
import { act, fireEvent, render, within } from "@testing-library/react-native";
import type { ReactTestInstance } from "react-test-renderer";
import type { SharedValue } from "react-native-reanimated";
import {
  COLLAPSED_TITLE_LINES,
  COLLAPSED_TITLE_MIN_SCALE,
  CollapsingHeader,
  EXPANDED_TITLE_LINES,
  EXPANDED_TITLE_MIN_SCALE,
  MAX_FONT_SCALE,
  NAV_ROW_HEIGHT,
  TITLE_MIN_FONT_SIZE,
  headerHeightsFor,
  navRowHeightFor,
} from "@/components/ui/CollapsingHeader";
import { theme } from "@/lib/theme";

function sharedValue(value: number): SharedValue<number> {
  return { value } as unknown as SharedValue<number>;
}

function opacityOf(node: ReactTestInstance): number | undefined {
  return (StyleSheet.flatten(node.props.style) as { opacity?: number }).opacity;
}

function heightOf(node: ReactTestInstance): number | undefined {
  return (StyleSheet.flatten(node.props.style) as { height?: number }).height;
}

type BoxStyle = {
  left?: unknown;
  marginLeft?: unknown;
  marginRight?: unknown;
  paddingLeft?: unknown;
  paddingHorizontal?: unknown;
  flexDirection?: unknown;
  width?: unknown;
  minWidth?: unknown;
};

function boxStyleOf(node: ReactTestInstance): BoxStyle {
  return (StyleSheet.flatten(node.props.style) ?? {}) as BoxStyle;
}

function points(value: unknown): number {
  return typeof value === "number" ? value : 0;
}

function paddingLeftOf(style: BoxStyle): number {
  return style.paddingLeft === undefined
    ? points(style.paddingHorizontal)
    : points(style.paddingLeft);
}

/**
 * Nearest HOST ancestor. The rendered tree alternates composite and host
 * nodes carrying identical props (RN's `View` is a `forwardRef` around a
 * native component), so a naive `.parent` walk visits — and would therefore
 * double-count — every level twice.
 */
function hostParentOf(node: ReactTestInstance): ReactTestInstance | null {
  let parent: ReactTestInstance | null = node.parent;
  while (parent && typeof parent.type !== "string") parent = parent.parent;
  return parent;
}

function contains(root: ReactTestInstance, target: ReactTestInstance): boolean {
  if (root === target) return true;
  for (const child of root.children) {
    if (typeof child === "string") continue;
    if (contains(child, target)) return true;
  }
  return false;
}

/**
 * Resolved left edge of `node` relative to the `collapsing-header` container,
 * in points.
 *
 * Asserting on a single `style.left` would only see ONE of the two ways this
 * block can end up indented, so this walks the ancestor chain and sums every
 * contribution: the node's own absolute `left` and `marginLeft`, each
 * ancestor's horizontal padding, and — for a row — the width of any
 * fixed-width sibling laid out BEFORE it (`width`/`minWidth` plus its
 * margins). That last term is exactly the `44 + 8` the back chevron costs
 * when the title block is nested beside it, which is the misalignment under
 * test; a fix that merely renamed a style constant would not move this
 * number.
 */
function leftInsetOf(node: ReactTestInstance): number {
  let inset = 0;
  let current: ReactTestInstance | null = node;
  while (current && current.props?.testID !== "collapsing-header") {
    const own = boxStyleOf(current);
    inset += points(own.left) + points(own.marginLeft);

    const parent = hostParentOf(current);
    if (!parent) break;
    const parentStyle = boxStyleOf(parent);
    inset += paddingLeftOf(parentStyle);

    if (parentStyle.flexDirection === "row") {
      for (const sibling of parent.children) {
        if (typeof sibling === "string") continue;
        if (contains(sibling, current)) break;
        const style = boxStyleOf(sibling);
        inset +=
          points(style.width === undefined ? style.minWidth : style.width) +
          points(style.marginLeft) +
          points(style.marginRight);
      }
    }
    current = parent;
  }
  return inset;
}

describe("CollapsingHeader", () => {
  it("renders fully expanded at scroll offset 0", () => {
    const { getByTestId } = render(
      <CollapsingHeader title="Orders" scrollY={sharedValue(0)} />,
    );
    expect(opacityOf(getByTestId("collapsing-header-expanded"))).toBe(1);
    expect(opacityOf(getByTestId("collapsing-header-collapsed"))).toBe(0);
  });

  it("renders fully collapsed at scroll offset >= 64", () => {
    const { getByTestId } = render(
      <CollapsingHeader title="Orders" scrollY={sharedValue(64)} />,
    );
    expect(opacityOf(getByTestId("collapsing-header-collapsed"))).toBe(1);
    expect(opacityOf(getByTestId("collapsing-header-expanded"))).toBe(0);
  });

  it("renders the title in both the expanded and collapsed layers", () => {
    const { getAllByText } = render(
      <CollapsingHeader title="Orders" scrollY={sharedValue(32)} />,
    );
    // Both layers are always mounted (cross-faded via opacity, not
    // conditionally rendered) so the title text node exists twice.
    expect(getAllByText("Orders")).toHaveLength(2);
  });

  /**
   * Gate-A residual (inc2 Task 8 review): `git grep allowFontScaling|
   * maxFontSizeMultiplier` across the app returned ZERO hits. RN scales both
   * `fontSize` AND `lineHeight` by the Dynamic Type multiplier, and
   * `styles.block` is `position:absolute; top:0; bottom:0` inside a container
   * with `overflow:"hidden"` — so at 1.9× a merchant lost the ascenders of
   * their own shop name. WCAG 2.1 AA is a project baseline and this primitive
   * goes onto Orders next, so the fix lives HERE, not on the Dashboard.
   */
  describe("Dynamic Type", () => {
    // Line boxes RN will scale. The number of boxes per element is KNOWN —
    // enforced by the numberOfLines assertions below.
    const EXPANDED_CONTENT =
      theme.text.caption.lineHeight + // eyebrow (the taller of the two presets)
      4 + // styles.eyebrow marginBottom
      theme.text.h1.lineHeight * EXPANDED_TITLE_LINES;
    const EXPANDED_CONTENT_WITH_SUBTITLE =
      EXPANDED_CONTENT +
      4 + // styles.expandedSubtitle marginTop
      theme.text.body.lineHeight;
    const COLLAPSED_CONTENT =
      theme.text.h3.lineHeight + 2 + theme.text.caption.lineHeight;

    it.each([1, 1.35, 1.6, 1.9, 3.1])(
      "keeps both states taller than their own scaled content at fontScale %s",
      (fontScale) => {
        const { expanded, collapsed } = headerHeightsFor(fontScale);
        const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
        expect(expanded).toBeGreaterThan(EXPANDED_CONTENT * scale);
        expect(collapsed).toBeGreaterThan(COLLAPSED_CONTENT * scale);
      },
    );

    // The subtitle's box is 28pt of extra content (4 margin + 24 body). With
    // a two-line title that is 122 against an EXPANDED_HEIGHT of 96 — it MUST
    // grow the base, not clip against `overflow: "hidden"`.
    it.each([1, 1.35, 1.6, 1.9, 3.1])(
      "grows the expanded base to fit a subtitle at fontScale %s",
      (fontScale) => {
        const { expanded } = headerHeightsFor(fontScale, true);
        const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
        expect(expanded).toBeGreaterThan(EXPANDED_CONTENT_WITH_SUBTITLE * scale);
        expect(expanded).toBeGreaterThan(headerHeightsFor(fontScale).expanded);
      },
    );

    it("sizes the rendered container from the subtitle's presence, not just the font scale", () => {
      const withSubtitle = render(
        <CollapsingHeader
          title="Northside Coffee Roasters"
          eyebrow="Monday, 27 July"
          subtitle="12 orders today"
          scrollY={sharedValue(0)}
        />,
      );
      const withoutSubtitle = render(
        <CollapsingHeader
          title="Northside Coffee Roasters"
          eyebrow="Monday, 27 July"
          scrollY={sharedValue(0)}
        />,
      );
      const { fontScale } = Dimensions.get("window");

      expect(heightOf(withSubtitle.getByTestId("collapsing-header"))).toBe(
        headerHeightsFor(fontScale, true).expanded,
      );
      expect(heightOf(withoutSubtitle.getByTestId("collapsing-header"))).toBe(
        headerHeightsFor(fontScale, false).expanded,
      );
      expect(heightOf(withSubtitle.getByTestId("collapsing-header"))).toBeGreaterThan(
        heightOf(withoutSubtitle.getByTestId("collapsing-header"))!,
      );
    });

    it("caps the multiplier so the accessibility sizes above 2x stop growing the header", () => {
      expect(headerHeightsFor(3.1)).toEqual(headerHeightsFor(MAX_FONT_SCALE));
      // …but honours WCAG 1.4.4's 200% resize rather than clamping lower.
      expect(MAX_FONT_SCALE).toBeGreaterThanOrEqual(2);
    });

    it("never shrinks below the design heights when fontScale is under 1", () => {
      expect(headerHeightsFor(0.85)).toEqual(headerHeightsFor(1));
    });

    it("sizes the rendered container from the device font scale, not a fixed 96", () => {
      // jest-expo reports fontScale 2, so this render also proves the hook is
      // actually read rather than defaulted to 1.
      const { getByTestId } = render(
        <CollapsingHeader title="Bondi Supply" eyebrow="Monday, 27 July" scrollY={sharedValue(0)} />,
      );
      const { fontScale } = Dimensions.get("window");
      expect(fontScale).toBeGreaterThan(1);
      expect(heightOf(getByTestId("collapsing-header"))).toBe(
        headerHeightsFor(fontScale).expanded,
      );
      expect(heightOf(getByTestId("collapsing-header"))).not.toBe(96);
    });

    it("caps every line at MAX_FONT_SCALE", () => {
      const { getByTestId } = render(
        <CollapsingHeader
          title="Northside Coffee Roasters"
          eyebrow="Monday, 27 July"
          subtitle="12 orders today"
          scrollY={sharedValue(0)}
        />,
      );
      for (const layer of ["collapsing-header-expanded", "collapsing-header-collapsed"]) {
        // Host "Text" nodes, matched by tag rather than by the RN component
        // reference — the repo has two @types/react copies and the class
        // identity doesn't typecheck across them.
        const texts = getByTestId(layer).findAll((n) => String(n.type) === "Text");
        expect(texts.length).toBeGreaterThan(0);
        for (const t of texts) {
          expect(t.props.maxFontSizeMultiplier).toBe(MAX_FONT_SCALE);
          // Every line has a KNOWN allowance so the height math holds. The
          // per-element values are asserted below — this only rules out an
          // unbounded line count.
          expect(t.props.numberOfLines).toBeGreaterThanOrEqual(1);
        }
      }
    });

    /**
     * The round-1 clipping fix capped the title at ONE line, which traded the
     * clip for a truncation of the merchant's own shop name: at the DEFAULT
     * text size `Northside Coffee Roasters` rendered `Northside Coffee Ro…`,
     * and at `accessibility-medium` (~1.94×) even `Bondi Beach Co.` became
     * `Bondi Bea…`. Loss of content at 200% resize is exactly what WCAG 2.1
     * SC 1.4.4 forbids. Two lines fit (18 + 4 + 72 = 94 < 96).
     */
    it("lets the EXPANDED title wrap rather than truncating a real shop name", () => {
      const TITLE = "Northside Coffee Roasters";
      const { getByTestId } = render(
        <CollapsingHeader
          title={TITLE}
          eyebrow="Monday, 27 July"
          subtitle="12 orders today"
          scrollY={sharedValue(0)}
        />,
      );

      const expanded = within(getByTestId("collapsing-header-expanded"));
      expect(expanded.getByText(TITLE).props.numberOfLines).toBe(EXPANDED_TITLE_LINES);
      expect(EXPANDED_TITLE_LINES).toBeGreaterThan(1);
      // The supporting lines stay single — they are what the height math
      // budgets exactly one box each for.
      expect(expanded.getByText("Monday, 27 July").props.numberOfLines).toBe(1);
      expect(expanded.getByText("12 orders today").props.numberOfLines).toBe(1);
    });

    /**
     * The COLLAPSED counterpart, and the same ruling: "a `numberOfLines` that
     * ellipsises a merchant's shop name is a different bug, not a fix."
     *
     * Measured on device before this changed (iPhone 17 Pro, cleared bundle,
     * relaunched after each content_size change):
     *   - `The Bondi Store` → `The Bondi Sto…` at `accessibility-large`
     *   - `Northside Coffee Roasters & Bakehouse` → `…Roasters & B…` at the
     *     DEFAULT size, `Northside Co…` at `accessibility-large`
     * The bar's HEIGHT was never the problem (it grew to ~112pt at the capped
     * 2×, ascenders intact) — the title had one line and a scaled serif.
     */
    it("lets the COLLAPSED title wrap rather than truncating a real shop name", () => {
      const TITLE = "Northside Coffee Roasters";
      const { getByTestId } = render(
        <CollapsingHeader
          title={TITLE}
          eyebrow="Monday, 27 July"
          subtitle="12 orders today"
          scrollY={sharedValue(64)}
        />,
      );

      const collapsed = within(getByTestId("collapsing-header-collapsed"));
      expect(collapsed.getByText(TITLE).props.numberOfLines).toBe(COLLAPSED_TITLE_LINES);
      expect(COLLAPSED_TITLE_LINES).toBeGreaterThan(1);
      // The supporting line stays single — it is what the height math budgets
      // exactly one box for.
      expect(collapsed.getByText("12 orders today").props.numberOfLines).toBe(1);
    });

    /**
     * The other half of that one contract. `styles.block` is
     * `position: absolute; top: 0; bottom: 0` inside a container with
     * `overflow: "hidden"`, so a title allowed to draw two lines into a box
     * still sized for one clips SILENTLY — with a fully green suite. This
     * asserts the box grew with what it now holds.
     */
    const COLLAPSED_CONTENT_WRAPPED =
      theme.text.h3.lineHeight * COLLAPSED_TITLE_LINES + 2 + theme.text.caption.lineHeight;

    it.each([1, 1.35, 1.6, 1.9, 3.1])(
      "grows the collapsed box to fit a wrapped title at fontScale %s",
      (fontScale) => {
        const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
        const wrapped = headerHeightsFor(fontScale, false, COLLAPSED_TITLE_LINES).collapsed;
        expect(wrapped).toBeGreaterThan(COLLAPSED_CONTENT_WRAPPED * scale);
        expect(wrapped).toBeGreaterThan(headerHeightsFor(fontScale, false, 1).collapsed);
      },
    );

    it("leaves the one-line collapsed height untouched, so a short title keeps its compact bar", () => {
      // The additive third parameter must DEFAULT to the pre-existing answer:
      // every caller that has not measured two lines pays nothing for this.
      expect(headerHeightsFor(1.6, false).collapsed).toBe(
        headerHeightsFor(1.6, false, 1).collapsed,
      );
      expect(headerHeightsFor(1.6, true).collapsed).toBe(
        headerHeightsFor(1.6, false).collapsed,
      );
    });

    it("clamps a measured line count that the title could never actually draw", () => {
      // Fed from a native layout callback, so it is not trusted.
      expect(headerHeightsFor(1, false, 0).collapsed).toBe(
        headerHeightsFor(1, false, 1).collapsed,
      );
      expect(headerHeightsFor(1, false, 9).collapsed).toBe(
        headerHeightsFor(1, false, COLLAPSED_TITLE_LINES).collapsed,
      );
    });

    /**
     * End to end through the component: the rendered container must resize off
     * the MEASURED line count, not off a constant. Deleting the `onTextLayout`
     * wiring leaves the height frozen and fails this.
     *
     * The seed is the MAXIMUM allowance so a wrapping title never flashes
     * clipped, so the observable transition here is the shrink to one line.
     */
    it("resizes the rendered collapsed bar from the title's measured line count", () => {
      const TITLE = "Northside Coffee Roasters & Bakehouse";
      const { getByTestId } = render(
        <CollapsingHeader title={TITLE} scrollY={sharedValue(64)} />,
      );
      const { fontScale } = Dimensions.get("window");
      const collapsedTitle = within(getByTestId("collapsing-header-collapsed")).getByText(
        TITLE,
      );

      // Seeded at the two-line height before any measurement lands.
      expect(heightOf(getByTestId("collapsing-header"))).toBe(
        headerHeightsFor(fontScale, false, COLLAPSED_TITLE_LINES).collapsed,
      );

      act(() => {
        collapsedTitle.props.onTextLayout({ nativeEvent: { lines: [{}] } });
      });
      expect(heightOf(getByTestId("collapsing-header"))).toBe(
        headerHeightsFor(fontScale, false, 1).collapsed,
      );

      act(() => {
        collapsedTitle.props.onTextLayout({ nativeEvent: { lines: [{}, {}] } });
      });
      expect(heightOf(getByTestId("collapsing-header"))).toBe(
        headerHeightsFor(fontScale, false, COLLAPSED_TITLE_LINES).collapsed,
      );
      expect(heightOf(getByTestId("collapsing-header"))).toBeGreaterThan(
        headerHeightsFor(fontScale, false, 1).collapsed,
      );
    });
  });

  /**
   * Two lines were not enough. Measured on device (iPhone 17 Pro, 402pt wide,
   * cleared bundle, relaunched after every content_size change) against
   * `Northside Coffee Roasters & Bakehouse`, 37 characters:
   *
   *   expanded, DEFAULT size    → `Northside Coffee Roasters & Bakehou…`
   *   expanded, accessibility-L → `Northside Coffee R…`
   *   collapsed, accessibility-L → `Northside Coffee Roaste…`
   *
   * i.e. the truncation is a WIDTH problem present at the default text size
   * as well as the accessibility ones, and the EXPANDED layer — the one the
   * merchant reads at rest — loses more of the name than the collapsed bar
   * does. Line allowance cannot buy the rest (a third collapsed line puts the
   * bar past the expanded one), so the remaining room is horizontal.
   */
  describe("shrink-to-fit floor", () => {
    it("lets BOTH title layers shrink rather than ellipsise, down to a shared floor", () => {
      const TITLE = "Northside Coffee Roasters & Bakehouse";
      const expanded = within(
        render(<CollapsingHeader title={TITLE} scrollY={sharedValue(0)} />).getByTestId(
          "collapsing-header-expanded",
        ),
      ).getByText(TITLE);
      const collapsed = within(
        render(<CollapsingHeader title={TITLE} scrollY={sharedValue(64)} />).getByTestId(
          "collapsing-header-collapsed",
        ),
      ).getByText(TITLE);

      // Both layers, not just the compact bar: fixing only the collapsed one
      // would leave the collapsed state showing MORE of the shop name than
      // the expanded state, which is worse than the bug.
      expect(expanded.props.adjustsFontSizeToFit).toBe(true);
      expect(collapsed.props.adjustsFontSizeToFit).toBe(true);
      expect(expanded.props.minimumFontScale).toBe(EXPANDED_TITLE_MIN_SCALE);
      expect(collapsed.props.minimumFontScale).toBe(COLLAPSED_TITLE_MIN_SCALE);
    });

    /**
     * The floor is a SIZE, not a fraction, and this is the assertion that
     * says so. `minimumFontScale` is a fraction of the already-Dynamic-Type-
     * scaled size, so `TITLE_MIN_FONT_SIZE / fontSize` pins the floor to
     * `TITLE_MIN_FONT_SIZE × fontScale` at every scale.
     *
     * `MetricsCard` can spell its floor as a bare 0.5 because the hero
     * numeral is only ever squeezed at raised text sizes, where half of the
     * 2×-scaled 88pt is exactly its default 44pt. This title is squeezed at
     * the DEFAULT size too, where the same 0.5 would authorise a 10pt
     * collapsed shop name — below iOS's 11pt legibility guidance. Anyone
     * lowering this floor to an inaccessible size fails here.
     */
    it("floors the shrink at a real design-system size, not an arbitrary fraction", () => {
      expect(theme.text.h1.fontSize * EXPANDED_TITLE_MIN_SCALE).toBe(TITLE_MIN_FONT_SIZE);
      expect(theme.text.h3.fontSize * COLLAPSED_TITLE_MIN_SCALE).toBe(TITLE_MIN_FONT_SIZE);
      // `caption` — the smallest type this app ships as running copy, and
      // what this header's own subtitle draws in.
      expect(TITLE_MIN_FONT_SIZE).toBe(theme.text.caption.fontSize);
      expect(TITLE_MIN_FONT_SIZE).toBeGreaterThanOrEqual(11);
    });

    /**
     * Regression: a RED SCREEN on the Dashboard, found on device and invisible
     * to every test in this file, because the tests hand-build the event.
     *
     * With `adjustsFontSizeToFit` on, iOS fires `onTextLayout` for its own
     * font-fitting passes with a payload carrying no `lines` array —
     * `event.nativeEvent.lines.length` threw "Cannot read property 'lines' of
     * null" the instant the header collapsed. The measurement must ignore a
     * payload it cannot read, and must not resize the box off one.
     */
    it("ignores an onTextLayout payload with no line measurement", () => {
      const TITLE = "Northside Coffee Roasters & Bakehouse";
      const { getByTestId } = render(
        <CollapsingHeader title={TITLE} scrollY={sharedValue(64)} />,
      );
      const { fontScale } = Dimensions.get("window");
      const collapsedTitle = within(getByTestId("collapsing-header-collapsed")).getByText(
        TITLE,
      );

      act(() => {
        collapsedTitle.props.onTextLayout({ nativeEvent: { lines: [{}] } });
      });
      const settled = headerHeightsFor(fontScale, false, 1).collapsed;
      expect(heightOf(getByTestId("collapsing-header"))).toBe(settled);

      for (const nativeEvent of [null, undefined, {}, { lines: null }, { lines: [] }]) {
        expect(() => {
          act(() => {
            collapsedTitle.props.onTextLayout({ nativeEvent });
          });
        }).not.toThrow();
        // …and the unreadable pass must not move the box either.
        expect(heightOf(getByTestId("collapsing-header"))).toBe(settled);
      }
    });
  });

  describe("eyebrowPreset", () => {
    // Additive prop, NOT a changed default — the uppercase small-caps eyebrow
    // is this primitive's designed identity, and the Dashboard's sentence-case
    // dateline is a local need. (The "~15 call sites" this comment used to
    // cite belonged to `Eyebrow`, a different primitive; `CollapsingHeader`
    // has one caller today and two after Orders.)
    // Asserted on the resolved utility classes rather than a flattened
    // style: NativeWind compiles classNames natively and does not resolve
    // them to RN style objects under jest, so `textTransform` is undefined in
    // this environment for BOTH presets and would pass vacuously.
    it("defaults to the uppercase small-caps eyebrow", () => {
      const { getByText } = render(
        <CollapsingHeader title="Orders" eyebrow="Today" scrollY={sharedValue(0)} />,
      );
      expect(getByText("Today").props.className).toContain("uppercase");
      expect(getByText("Today").props.className).toContain("text-eyebrow");
    });

    it("renders the eyebrow in sentence case when the caption preset is asked for", () => {
      const { getByText } = render(
        <CollapsingHeader
          title="Bondi Supply"
          eyebrow="Monday, 27 July"
          eyebrowPreset="caption"
          scrollY={sharedValue(0)}
        />,
      );
      const className = getByText("Monday, 27 July").props.className as string;
      expect(className).not.toContain("uppercase");
      expect(className).toContain("text-caption");
    });
  });

  describe("reduced motion", () => {
    const reanimated = jest.requireMock("react-native-reanimated") as {
      useReducedMotion: jest.Mock;
    };

    afterEach(() => {
      reanimated.useReducedMotion.mockReturnValue(false);
    });

    it("snaps straight to the collapsed state at any non-zero offset, with no partial interpolation", () => {
      reanimated.useReducedMotion.mockReturnValue(true);
      // 10px is well short of the 64px collapse distance — under normal
      // interpolation this would be ~16% collapsed, not fully collapsed.
      const { getByTestId } = render(
        <CollapsingHeader title="Orders" scrollY={sharedValue(10)} />,
      );
      expect(opacityOf(getByTestId("collapsing-header-collapsed"))).toBe(1);
      expect(opacityOf(getByTestId("collapsing-header-expanded"))).toBe(0);
    });

    it("stays expanded at offset 0 even with reduced motion on", () => {
      reanimated.useReducedMotion.mockReturnValue(true);
      const { getByTestId } = render(
        <CollapsingHeader title="Orders" scrollY={sharedValue(0)} />,
      );
      expect(opacityOf(getByTestId("collapsing-header-expanded"))).toBe(1);
      expect(opacityOf(getByTestId("collapsing-header-collapsed"))).toBe(0);
    });
  });

  /**
   * `left` is `flex: 1`, i.e. `flexBasis: 0` — it has no basis to shrink FROM.
   * With RN's default `flexShrink: 0` on `right`, a slot that grows past the
   * row (scalable text at an accessibility size) doesn't shrink; the overflow
   * comes straight out of `left`'s resolved width instead, and Orders' "Inbox"
   * title was crushed to nothing while its "3 pending" caption kept every
   * pixel it asked for. The Gate-A Dynamic Type run only ever exercised the
   * Dashboard's rightSlot, which is a FIXED 40pt monogram and cannot show
   * this.
   */
  describe("rightSlot", () => {
    it("shrinks the trailing slot rather than the title, and floors it at the touch target", () => {
      const { getByTestId } = render(
        <CollapsingHeader
          title="Inbox"
          eyebrow="Orders"
          rightSlot={<RNText>3 pending</RNText>}
          scrollY={sharedValue(0)}
        />,
      );
      const right = StyleSheet.flatten(
        getByTestId("collapsing-header-right").props.style,
      ) as { flexShrink?: number; minWidth?: number };
      expect(right.flexShrink).toBe(1);
      expect(right.minWidth).toBe(theme.touchTarget);
    });
  });
});

/**
 * The back affordance — ADDITIVE, and that is the whole point of the prop.
 *
 * Six of the eight list screens increment 3 rolls this primitive across are
 * NESTED routes that used `BackHeader`, which has a back chevron;
 * `CollapsingHeader` had none, so "every screen gets a collapsing header" was
 * impossible without it. The two tab-root callers that already use this
 * primitive (Dashboard, Orders) pass no `onBack`, and their rendering must
 * stay bit-identical — hence the default case below is asserted first.
 */
describe("CollapsingHeader — back affordance", () => {
  it("renders no back control by default (bit-identical for existing callers)", () => {
    const { queryByLabelText } = render(
      <CollapsingHeader title="Inbox" scrollY={sharedValue(0)} />,
    );
    expect(queryByLabelText("Go back")).toBeNull();
  });

  it("renders a back control in BOTH states when onBack is supplied", () => {
    const onBack = jest.fn();
    const expanded = render(
      <CollapsingHeader title="Coupons" onBack={onBack} scrollY={sharedValue(0)} />,
    );
    expect(expanded.getByLabelText("Go back")).toBeTruthy();
    expanded.unmount();
    const collapsed = render(
      <CollapsingHeader title="Coupons" onBack={onBack} scrollY={sharedValue(200)} />,
    );
    expect(collapsed.getByLabelText("Go back")).toBeTruthy();
  });

  it("fires onBack", () => {
    const onBack = jest.fn();
    const { getByLabelText } = render(
      <CollapsingHeader title="Coupons" onBack={onBack} scrollY={sharedValue(0)} />,
    );
    fireEvent.press(getByLabelText("Go back"));
    expect(onBack).toHaveBeenCalledTimes(1);
  });

  /**
   * The layout invariant this whole increment is built on, and the assertion
   * that would have caught the round-1 defect: eyebrow, title, filter chips
   * and every list row share ONE left edge at `theme.spacing.xl`.
   *
   * Round 1 put the chevron INLINE with the expanded title and indented the
   * title block by `44 + theme.spacing.sm` to clear it, so on device the
   * eyebrow and serif title started at ~72pt while the chips and rows below
   * started at 20pt. The ruling: the chevron gets its own nav row above the
   * editorial block in the expanded state — iOS's own large-title convention.
   */
  it("starts the expanded eyebrow and title at the list gutter, not indented past the back control", () => {
    const { getByTestId } = render(
      <CollapsingHeader
        title="Gift cards"
        eyebrow="MARKETING"
        onBack={jest.fn()}
        rightSlot={<RNText>+</RNText>}
        scrollY={sharedValue(0)}
      />,
    );
    expect(leftInsetOf(getByTestId("collapsing-header-expanded"))).toBe(theme.spacing.xl);
  });

  /**
   * The other half of the ruling: the COLLAPSED bar is CORRECT as it ships
   * and must not move. There the chevron genuinely IS on the title's line, so
   * the inline indent is the standard iOS compact-bar look.
   */
  it("keeps the collapsed title inline behind the back control, at the nav-bar indent", () => {
    const { getByTestId } = render(
      <CollapsingHeader
        title="Gift cards"
        eyebrow="MARKETING"
        onBack={jest.fn()}
        rightSlot={<RNText>+</RNText>}
        scrollY={sharedValue(200)}
      />,
    );
    expect(leftInsetOf(getByTestId("collapsing-header-collapsed"))).toBe(
      theme.spacing.xl + theme.touchTarget + theme.spacing.sm,
    );
  });

  // Both layers of a header with NO leading control sit at the plain gutter,
  // before and after this change — the tab-root callers are untouched.
  it("leaves both layers at the gutter when there is no leading control", () => {
    const { getByTestId } = render(
      <CollapsingHeader title="Inbox" eyebrow="ORDERS" scrollY={sharedValue(0)} />,
    );
    expect(leftInsetOf(getByTestId("collapsing-header-expanded"))).toBe(theme.spacing.xl);
    expect(leftInsetOf(getByTestId("collapsing-header-collapsed"))).toBe(theme.spacing.xl);
  });

  /**
   * The height contract. The nav row is a REAL vertical cost — it holds the
   * leading control and `rightSlot` above the editorial block — so
   * `headerHeightsFor` must know about it or the block clips against the
   * container's `overflow: "hidden"`.
   *
   * It scales with `fontScale` like every other term here. It shipped as a
   * true constant, on the argument that the band holds only fixed glyphs;
   * both slots take arbitrary nodes, and Orders' own shipped pattern puts a
   * caption in `rightSlot`, so at `fontScale: 2` that argument bought a ~72pt
   * two-line caption inside a 44pt band inside `overflow: "hidden"`.
   */
  describe("nav row height", () => {
    it("leaves the arithmetic byte-identical when there is no leading control", () => {
      // Pinned literals, not a re-derivation: Dashboard and Orders pass
      // neither `onBack` nor `leadingSlot` and must keep exactly these.
      expect(headerHeightsFor(1)).toEqual({ expanded: 96, collapsed: 56 });
      expect(headerHeightsFor(1, true)).toEqual({ expanded: 124, collapsed: 56 });
      expect(headerHeightsFor(1.5)).toEqual({ expanded: 144, collapsed: 84 });
      expect(headerHeightsFor(1, false, COLLAPSED_TITLE_LINES)).toEqual({
        expanded: 96,
        collapsed: 82,
      });
      // …and the new parameter defaults to that same answer.
      expect(headerHeightsFor(1.6, false, 1, false)).toEqual(headerHeightsFor(1.6, false, 1));
    });

    it("adds exactly one nav row to the expanded height when a leading control is present", () => {
      expect(NAV_ROW_HEIGHT).toBe(theme.touchTarget);
      for (const fontScale of [1, 1.35, 1.9, 3.1]) {
        const plain = headerHeightsFor(fontScale, false, 1);
        const withNav = headerHeightsFor(fontScale, false, 1, true);
        // Stated as a sum rather than a difference: at 1.9 the subtraction
        // lands on 83.60000000000002 and asserts a float artifact instead of
        // the contract.
        expect(withNav.expanded).toBe(plain.expanded + navRowHeightFor(fontScale));
        expect(withNav.expanded).toBeGreaterThan(plain.expanded);
        // The compact bar already centres the chevron on the title's line, so
        // it pays nothing.
        expect(withNav.collapsed).toBe(plain.collapsed);
      }
    });

    /**
     * The band scales, and this is the assertion that says why.
     *
     * `rightSlot` and `leadingSlot` take arbitrary nodes, and the pattern
     * Orders already ships is a caption ("5 pending"). Two capped caption
     * lines are 36pt at 1× and 72pt at the capped 2× — against a band that
     * used to be a flat 44 at every size, inside `overflow: "hidden"`, with
     * the editorial block pinned directly beneath it. Nothing in
     * `headerHeightsFor` could absorb it, and nothing would have failed: the
     * clip is silent.
     *
     * Scaling the band by the SAME clamped multiplier the text is capped at
     * makes the fit structural rather than a per-caller flag someone can
     * forget to pass — `content = C × s` inside `box = H × s` holds at every
     * scale once it holds at one.
     */
    const NAV_SLOT_ALLOWANCE = theme.text.caption.lineHeight * 2;

    it.each([1, 1.35, 1.6, 1.9, 3.1])(
      "keeps the nav row taller than the text its slots are documented to hold, at fontScale %s",
      (fontScale) => {
        const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
        expect(navRowHeightFor(fontScale)).toBeGreaterThan(NAV_SLOT_ALLOWANCE * scale);
        // …and the band never dips under the touch target it exists to hold.
        expect(navRowHeightFor(fontScale)).toBeGreaterThanOrEqual(theme.touchTarget);
      },
    );

    it("leaves the default-size band exactly as it shipped, and caps its growth with everything else", () => {
      expect(navRowHeightFor(1)).toBe(NAV_ROW_HEIGHT);
      expect(navRowHeightFor(0.85)).toBe(NAV_ROW_HEIGHT);
      expect(navRowHeightFor(3.1)).toBe(navRowHeightFor(MAX_FONT_SCALE));
      expect(navRowHeightFor(1.9)).toBeGreaterThan(navRowHeightFor(1));
    });

    /**
     * End to end through the component, with the exact combination Task 5
     * (Tickets) is specified to ship — `onBack` AND a text-bearing
     * `rightSlot` — at the jest environment's raised font scale (2).
     *
     * The rendered band, the editorial block's `top` and the nav-row term
     * inside the container height are ONE number; if any of the three is left
     * on the unscaled constant the caption clips or the block overlaps the
     * row. Asserting all three against `navRowHeightFor` is what makes that
     * impossible to half-fix.
     */
    it("gives a text-bearing rightSlot a band that fits it, alongside a back control", () => {
      const { getByTestId } = render(
        <CollapsingHeader
          title="Gift cards"
          eyebrow="MARKETING"
          onBack={jest.fn()}
          rightSlot={<RNText maxFontSizeMultiplier={MAX_FONT_SCALE}>5 pending</RNText>}
          scrollY={sharedValue(0)}
        />,
      );
      const { fontScale } = Dimensions.get("window");
      const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
      const band = navRowHeightFor(fontScale);

      expect(scale).toBeGreaterThan(1);
      expect(heightOf(getByTestId("collapsing-header-nav-row"))).toBe(band);
      expect(band).toBeGreaterThan(NAV_SLOT_ALLOWANCE * scale);
      // The block sits below the band it was measured against…
      expect(
        (boxStyleOf(getByTestId("collapsing-header-expanded")) as { top?: unknown }).top,
      ).toBe(band);
      // …and the container is tall enough for both, so neither clips.
      expect(heightOf(getByTestId("collapsing-header"))).toBe(
        headerHeightsFor(fontScale, false, 1, true).expanded,
      );
      expect(heightOf(getByTestId("collapsing-header"))!).toBeGreaterThanOrEqual(
        band + headerHeightsFor(fontScale, false, 1).expanded,
      );
    });

    it("sizes the rendered container off the nav row, in the expanded state only", () => {
      const { fontScale } = Dimensions.get("window");

      const plainExpanded = render(<CollapsingHeader title="Coupons" scrollY={sharedValue(0)} />);
      expect(heightOf(plainExpanded.getByTestId("collapsing-header"))).toBe(
        headerHeightsFor(fontScale, false, 1).expanded,
      );
      plainExpanded.unmount();

      const backExpanded = render(
        <CollapsingHeader title="Coupons" onBack={jest.fn()} scrollY={sharedValue(0)} />,
      );
      expect(heightOf(backExpanded.getByTestId("collapsing-header"))).toBe(
        headerHeightsFor(fontScale, false, 1, true).expanded,
      );
      backExpanded.unmount();

      const plainCollapsed = render(
        <CollapsingHeader title="Coupons" scrollY={sharedValue(200)} />,
      );
      const plainCollapsedHeight = heightOf(plainCollapsed.getByTestId("collapsing-header"));
      plainCollapsed.unmount();

      const backCollapsed = render(
        <CollapsingHeader title="Coupons" onBack={jest.fn()} scrollY={sharedValue(200)} />,
      );
      expect(heightOf(backCollapsed.getByTestId("collapsing-header"))).toBe(plainCollapsedHeight);
      // …and the title still renders at the collapsed state with a back
      // control present, rather than being squeezed out of the row entirely.
      // Both layers are always mounted, so the title node exists twice.
      expect(backCollapsed.getAllByText("Coupons")).toHaveLength(2);
    });

    it("keeps the expanded editorial block clear of the nav row it now sits under", () => {
      const { getByTestId } = render(
        <CollapsingHeader
          title="Gift cards"
          eyebrow="MARKETING"
          onBack={jest.fn()}
          scrollY={sharedValue(0)}
        />,
      );
      const block = boxStyleOf(getByTestId("collapsing-header-expanded")) as { top?: unknown };
      const { fontScale } = Dimensions.get("window");
      // `styles.block` is `top: 0; bottom: 0` — the editorial layer must be
      // pushed BELOW the nav row, and the container grown by the same amount
      // (asserted above), or the two overlap. The offset tracks the SCALED
      // band, not the unscaled constant, or the two disagree at every text
      // size above the default.
      expect(block.top).toBe(navRowHeightFor(fontScale));
      expect(block.top).not.toBe(NAV_ROW_HEIGHT);
    });
  });

  describe("leadingSlot", () => {
    it("renders an arbitrary leading control when no onBack is given", () => {
      const { getByText, queryByLabelText } = render(
        <CollapsingHeader
          title="Coupons"
          leadingSlot={<RNText>Close</RNText>}
          scrollY={sharedValue(0)}
        />,
      );
      expect(getByText("Close")).toBeTruthy();
      expect(queryByLabelText("Go back")).toBeNull();
    });

    /**
     * The horizontal half of the same exposure `styles.right` documents. This
     * slot's comment used to assert it was "a fixed-size glyph button with no
     * text in it" — true of the `onBack` chevron, false of the escape hatch
     * the prop exists to provide, and the suite's own example above renders a
     * `Close` LABEL. `left` is `flex: 1` (`flexBasis: 0`) with no basis to
     * shrink from, so an unshrinkable leading slot that grows at an
     * accessibility size takes the title column's width to zero — exactly
     * what shipped once on Orders' "Inbox".
     */
    it("shrinks the leading slot rather than the title, and floors it at the touch target", () => {
      const { getByTestId } = render(
        <CollapsingHeader
          title="Coupons"
          leadingSlot={<RNText maxFontSizeMultiplier={MAX_FONT_SCALE}>Close</RNText>}
          scrollY={sharedValue(0)}
        />,
      );
      const leading = StyleSheet.flatten(
        getByTestId("collapsing-header-leading").props.style,
      ) as { flexShrink?: number; minWidth?: number };
      expect(leading.flexShrink).toBe(1);
      expect(leading.minWidth).toBe(theme.touchTarget);
    });

    // Documented precedence, asserted: `onBack` is the common case and the
    // one the rollout uses, so a screen that accidentally passes both gets
    // the back chevron rather than two stacked leading controls.
    it("is ignored when onBack is set", () => {
      const { getByLabelText, queryByText } = render(
        <CollapsingHeader
          title="Coupons"
          onBack={jest.fn()}
          leadingSlot={<RNText>Close</RNText>}
          scrollY={sharedValue(0)}
        />,
      );
      expect(getByLabelText("Go back")).toBeTruthy();
      expect(queryByText("Close")).toBeNull();
    });
  });
});
