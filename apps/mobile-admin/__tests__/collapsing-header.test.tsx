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
import { act, render, within } from "@testing-library/react-native";
import type { ReactTestInstance } from "react-test-renderer";
import type { SharedValue } from "react-native-reanimated";
import {
  COLLAPSED_TITLE_LINES,
  COLLAPSED_TITLE_MIN_SCALE,
  CollapsingHeader,
  EXPANDED_TITLE_LINES,
  EXPANDED_TITLE_MIN_SCALE,
  MAX_FONT_SCALE,
  TITLE_MIN_FONT_SIZE,
  headerHeightsFor,
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
