// `FilterChips` — the 40pt pill row that replaces the underlined
// `SegmentedControl` on Orders (inc2 Task 9). `SegmentedControl` is NOT
// changed: it still has eight other call sites, and re-skinning a shared
// primitive to suit one screen is the ripple this increment already paid for
// once. This is an additive sibling.
jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: {
    selectionChanged: jest.fn(() => Promise.resolve()),
  },
}));

import { ScrollView, StyleSheet } from "react-native";
import { render, fireEvent } from "@testing-library/react-native";
import { FilterChips, chipHeightsFor } from "@/components/ui/FilterChips";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { theme } from "@/lib/theme";

type Key = "all" | "pending" | "confirmed";

const CHIPS: { key: Key; label: string }[] = [
  { key: "all", label: "All" },
  { key: "pending", label: "Pending" },
  { key: "confirmed", label: "Confirmed" },
];

function renderChips(value: Key = "all", onChange = jest.fn()) {
  return {
    onChange,
    ...render(<FilterChips chips={CHIPS} value={value} onChange={onChange} />),
  };
}

beforeEach(() => jest.clearAllMocks());

describe("FilterChips — selection", () => {
  it("renders one chip per entry", () => {
    const { getByTestId } = renderChips();
    for (const chip of CHIPS) expect(getByTestId(`filter-chip-${chip.key}`)).toBeTruthy();
  });

  it("reports the tapped chip's key", () => {
    const { getByTestId, onChange } = renderChips();
    fireEvent.press(getByTestId("filter-chip-pending"));
    expect(onChange).toHaveBeenCalledWith("pending");
  });

  // Asserted on the `-target` node: the a11y state belongs on the Pressable
  // (the control), not on the inner pill View (the paint).
  it("marks the active chip selected for assistive tech", () => {
    const { getByTestId } = renderChips("pending");
    expect(getByTestId("filter-chip-pending-target").props.accessibilityState.selected).toBe(true);
    expect(getByTestId("filter-chip-all-target").props.accessibilityState.selected).toBe(false);
  });

  it("fires the selection haptic on a real change", () => {
    const { getByTestId } = renderChips("all");
    fireEvent.press(getByTestId("filter-chip-pending"));
    expect(adminHaptics.selectionChanged).toHaveBeenCalledTimes(1);
  });

  // Re-tapping the chip that is already active is not a change: it must not
  // buzz, and it must not push a redundant state update that re-renders the
  // list for nothing.
  it("stays silent when the already-active chip is tapped", () => {
    const { getByTestId, onChange } = renderChips("pending");
    fireEvent.press(getByTestId("filter-chip-pending"));
    expect(adminHaptics.selectionChanged).not.toHaveBeenCalled();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("scrolls horizontally so a long filter set stays reachable", () => {
    const { UNSAFE_getByType } = renderChips();
    const scroll = UNSAFE_getByType(ScrollView);
    expect(scroll.props.horizontal).toBe(true);
    expect(scroll.props.showsHorizontalScrollIndicator).toBe(false);
  });
});

// The defect: Orders passed `contentContainerStyle={{paddingVertical: 8}}`
// to give the pinned search field and the pill row some air. That padding
// lands on the ScrollView's INNER row, and the inner row sits in a scroll box
// pinned to exactly `heights.target` — so there was nowhere for it to go and
// the two came out flush. The prop is gone (a caller knob that silently
// no-ops is worse than no knob) and the rhythm is a hugging wrapper the
// fixed-height box sits inside.
describe("FilterChips — vertical rhythm", () => {
  it("puts real padding above and below the strip", () => {
    const { getByTestId } = renderChips();
    const block = StyleSheet.flatten(getByTestId("filter-chips-block").props.style);
    expect(block.paddingTop).toBe(theme.spacing.md);
    expect(block.paddingBottom).toBe(theme.spacing.sm);
  });

  // The regression guard proper: putting the spacing back on the content
  // container would look right in a diff and do nothing on device.
  it("keeps the spacing out of the fixed-height scroll box, where it is inert", () => {
    const { UNSAFE_getByType } = renderChips();
    const scroll = UNSAFE_getByType(ScrollView);
    const box = StyleSheet.flatten(scroll.props.style);
    const content = StyleSheet.flatten(scroll.props.contentContainerStyle);

    // jest-expo's Dimensions mock reports fontScale 2, hence the 2x target.
    expect(box.height).toBe(chipHeightsFor(2).target);
    expect(content.paddingVertical ?? 0).toBe(0);
    expect(content.paddingTop ?? 0).toBe(0);
    expect(content.paddingBottom ?? 0).toBe(0);
  });

  // The ~110pt dead-paper bug: a ScrollView in a flex column stretches, and
  // `styles.row`'s alignItems:center then centres the pills in the tall box.
  // The wrapper is now the strip's outermost node, so it has to refuse to
  // stretch too — otherwise the fix to the padding reopens the older bug.
  it("refuses to stretch in a flex column, wrapper and box alike", () => {
    const { getByTestId, UNSAFE_getByType } = renderChips();
    const block = StyleSheet.flatten(getByTestId("filter-chips-block").props.style);
    const box = StyleSheet.flatten(UNSAFE_getByType(ScrollView).props.style);
    expect(block.flexGrow).toBe(0);
    expect(block.flexShrink).toBe(0);
    expect(box.flexGrow).toBe(0);
    expect(box.flexShrink).toBe(0);
  });

  // The wrapper adds fixed padding, so the only thing that must track the
  // font scale is the box — and it still does.
  it("still sizes its box from the live font scale", () => {
    const { UNSAFE_getByType } = renderChips();
    const box = StyleSheet.flatten(UNSAFE_getByType(ScrollView).props.style);
    expect(box.height).toBeGreaterThan(chipHeightsFor(1).target);
  });
});

describe("FilterChips — the one-accent rule", () => {
  // Orders spends its single moss accent on the Approve swipe action and
  // nothing else. An active chip in moss would be a second — the brief calls
  // this out explicitly. Active is the ink fill the Ink dock already
  // established as this app's "selected" language.
  it("paints the active chip ink, never moss", () => {
    const { getByTestId } = renderChips("pending");
    const style = StyleSheet.flatten(getByTestId("filter-chip-pending").props.style);
    expect(style.backgroundColor).toBe(theme.colors.text);
    expect(style.backgroundColor).not.toBe(theme.colors.accent);
  });

  it("leaves an inactive chip on the paper ground with a hairline", () => {
    const { getByTestId } = renderChips("pending");
    const style = StyleSheet.flatten(getByTestId("filter-chip-all").props.style);
    expect(style.backgroundColor).not.toBe(theme.colors.text);
    expect(style.backgroundColor).not.toBe(theme.colors.accent);
    expect(style.borderColor).toBe(theme.colors.border);
  });

  it("uses no moss anywhere in the row", () => {
    const { UNSAFE_root } = renderChips("pending");
    const flat = JSON.stringify(
      UNSAFE_root.findAll(() => true).map((n) => n.props?.style ?? null),
    );
    expect(flat).not.toContain(theme.colors.accent);
  });
});

describe("FilterChips — touch target and Dynamic Type", () => {
  // A real minHeight box, never hitSlop: hitSlop extends the responder region
  // but not the rendered control, so it satisfies nothing a user can see and
  // nothing an accessibility audit measures.
  // NB: jest-expo's Dimensions mock reports `fontScale: 2`, so the rendered
  // pill here is the 2x box (80), not the 40 baseline — which is precisely
  // the case worth asserting on. The baseline is pinned by `chipHeightsFor`
  // below.
  it("presents a 44pt touch target around the pill at the rendered text size", () => {
    const { getByTestId } = renderChips();
    const style = StyleSheet.flatten(getByTestId("filter-chip-all").props.style);
    const target = StyleSheet.flatten(getByTestId("filter-chip-all-target").props.style);

    expect(style.height).toBe(chipHeightsFor(2).pill);
    expect(style.height).toBeGreaterThanOrEqual(theme.text.label.lineHeight * 2);
    expect(target.minHeight).toBeGreaterThanOrEqual(theme.touchTarget);
    expect(target.minHeight).toBeGreaterThanOrEqual(style.height);
  });

  it("puts the pill at 40pt at the default text size", () => {
    expect(chipHeightsFor(1).pill).toBe(40);
  });

  it("caps the chip label's font multiplier so it cannot outgrow the pill", () => {
    const { getByText } = render(<FilterChips chips={CHIPS} value="all" onChange={jest.fn()} />);
    expect(getByText("All").props.maxFontSizeMultiplier).toBe(2);
  });

  // Same structural guarantee CollapsingHeader's `headerHeightsFor` makes:
  // the BOX scales by the same clamped multiplier the TEXT is capped at, so
  // "content fits" is arithmetic rather than something re-checked by eye at
  // every text size.
  describe("chipHeightsFor", () => {
    it("keeps the label's line box inside the pill at every scale", () => {
      for (const scale of [1, 1.35, 1.9, 2, 3.1]) {
        const { pill } = chipHeightsFor(scale);
        const clamped = Math.min(Math.max(scale, 1), 2);
        expect(pill).toBeGreaterThanOrEqual(theme.text.label.lineHeight * clamped);
      }
    });

    it("clamps at 2x so an accessibility text size cannot eat the screen", () => {
      expect(chipHeightsFor(3.1)).toEqual(chipHeightsFor(2));
    });

    it("never drops the touch target below 44pt", () => {
      for (const scale of [1, 1.5, 2, 3.1]) {
        expect(chipHeightsFor(scale).target).toBeGreaterThanOrEqual(theme.touchTarget);
      }
    });
  });
});
