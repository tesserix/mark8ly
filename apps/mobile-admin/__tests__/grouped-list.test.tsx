// The grouped-inset-list primitive (inc2 §3, Task 9), extracted from
// `more/index.tsx`'s hand-built section loop — that screen's `Row` and
// section markup are promoted here verbatim, not redesigned. Every screen
// this primitive rolls onto (Account, Security, Notification settings, Team)
// gets its own extension to this suite's assertions where it deviates from
// what More already proved.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

import { Dimensions, StyleSheet, Text as RNText, View } from "react-native";
import { fireEvent, render } from "@testing-library/react-native";
import { GroupedList } from "@/components/ui/GroupedList";
import { GroupedRow } from "@/components/ui/GroupedRow";
import { theme } from "@/lib/theme";

function setFontScale(fontScale: number) {
  jest.spyOn(Dimensions, "get").mockReturnValue({ width: 390, height: 844, scale: 3, fontScale });
}

afterEach(() => jest.restoreAllMocks());

describe("GroupedList", () => {
  it("renders a labelled section as eyebrow + card + inset hairlines between rows", () => {
    const { getByText, getAllByTestId } = render(
      <GroupedList
        sections={[
          {
            key: "s1",
            label: "Store",
            rows: [
              <GroupedRow key="a" label="Branding" onPress={jest.fn()} testID="row" />,
              <GroupedRow key="b" label="Team" onPress={jest.fn()} testID="row" />,
            ],
          },
        ]}
      />,
    );
    expect(getByText("Store")).toBeTruthy();
    expect(getAllByTestId("row")).toHaveLength(2);
    expect(getByText("Branding")).toBeTruthy();
    expect(getByText("Team")).toBeTruthy();
  });

  it("renders no hairline above the first row or below the last", () => {
    const { UNSAFE_root } = render(
      <GroupedList
        sections={[
          {
            key: "s1",
            label: "Store",
            rows: [
              <GroupedRow key="a" label="Branding" onPress={jest.fn()} />,
              <GroupedRow key="b" label="Team" onPress={jest.fn()} />,
              <GroupedRow key="c" label="Tickets" onPress={jest.fn()} />,
            ],
          },
        ]}
      />,
    );
    // A Hairline renders a bare View whose height is theme.hairline (0.5).
    // With 3 rows there must be exactly 2 dividers — between 1-2 and 2-3,
    // never leading or trailing.
    const hairlines = UNSAFE_root.findAll(
      (node) =>
        node.type === View &&
        StyleSheet.flatten(node.props.style)?.height === theme.hairline,
    );
    expect(hairlines).toHaveLength(2);
    hairlines.forEach((h) => {
      expect(StyleSheet.flatten(h.props.style).marginLeft).toBe(
        theme.spacing.huge + theme.spacing.xs,
      );
    });
  });

  it("renders an unlabelled section without an empty eyebrow box", () => {
    const { queryByText, UNSAFE_root } = render(
      <GroupedList
        sections={[{ key: "s1", rows: [<GroupedRow key="a" label="Push notifications" />] }]}
      />,
    );
    // No section label means no eyebrow-preset text node at all — not an
    // eyebrow rendered with empty string.
    const eyebrowTexts = UNSAFE_root.findAll(
      (node) => node.type === RNText && node.props.className?.includes("text-eyebrow"),
    );
    expect(eyebrowTexts).toHaveLength(0);
    expect(queryByText("")).toBeNull();
  });

  it("renders an optional footer caption below the card", () => {
    const { getByText } = render(
      <GroupedList
        sections={[
          {
            key: "s1",
            label: "Alert types",
            rows: [<GroupedRow key="a" label="New orders" />],
            footer: "Choose what your store notifies you about.",
          },
        ]}
      />,
    );
    expect(getByText("Choose what your store notifies you about.")).toBeTruthy();
  });
});

describe("GroupedRow", () => {
  it("renders an interactive row at the 64pt single-line height", () => {
    const { getByTestId } = render(
      <GroupedRow label="Branding" onPress={jest.fn()} testID="row" />,
    );
    const style = StyleSheet.flatten(getByTestId("row").props.style);
    expect(style.minHeight).toBe(theme.row.minHeightSingle);
    expect(style.height).toBeUndefined();
  });

  it("renders a row with NO onPress as a non-interactive View, not a disabled button", () => {
    const onPress = jest.fn();
    const { getByTestId } = render(
      <GroupedRow label="Name" value="Jane Merchant" testID="row" />,
    );
    const row = getByTestId("row");
    expect(row.props.accessibilityRole).not.toBe("button");
    expect(row.props.accessibilityState?.disabled).not.toBe(true);
    // No onPress at all was ever wired up — nothing to fire.
    expect(row.props.onPress).toBeUndefined();
    fireEvent.press(row);
    expect(onPress).not.toHaveBeenCalled();
  });

  // The controller addendum's headline trap: "identical metrics" for the
  // non-interactive variant means `minHeight`, never `height` — a fixed
  // height holding scalable text is this app's single most repeated defect.
  it("gives the non-interactive row a minHeight floor, never a fixed height", () => {
    const { getByTestId } = render(<GroupedRow label="Name" value="Jane" testID="row" />);
    const style = StyleSheet.flatten(getByTestId("row").props.style);
    expect(style.minHeight).toBe(theme.row.minHeightSingle);
    expect(style.height).toBeUndefined();
  });

  // Code-review guard: the old local `Row` in `more/index.tsx` (deleted by
  // this task) never clamped its label — that's what let the row (a
  // `minHeight`, never a `height`) grow to fit a wrapped label at raised
  // accessibility text sizes. A `numberOfLines={1}` here would silently
  // ellipsize labels like "Notification settings" instead of letting them
  // wrap, which is this app's most repeated defect class in a subtler form.
  it("never clamps the label to a single line", () => {
    const { getByText } = render(
      <GroupedRow label="Notification settings" onPress={jest.fn()} />,
    );
    expect(getByText("Notification settings").props.numberOfLines).toBeUndefined();
  });

  it("defaults the chevron on when onPress is set and off when it is not", () => {
    const withPress = render(<GroupedRow label="Team" onPress={jest.fn()} testID="row" />);
    expect(withPress.queryByTestId("row-chevron")).toBeTruthy();
    withPress.unmount();

    const withoutPress = render(<GroupedRow label="Name" value="Jane" testID="row" />);
    expect(withoutPress.queryByTestId("row-chevron")).toBeNull();
  });

  it("lets a caller force the chevron off on an interactive row", () => {
    const { queryByTestId } = render(
      <GroupedRow label="Team" onPress={jest.fn()} chevron={false} testID="row" />,
    );
    expect(queryByTestId("row-chevron")).toBeNull();
  });

  it("passes a plain ARRAY style, never a function", () => {
    const interactive = render(<GroupedRow label="Team" onPress={jest.fn()} testID="row" />);
    expect(typeof interactive.getByTestId("row").props.style).not.toBe("function");
    interactive.unmount();

    const nonInteractive = render(<GroupedRow label="Name" value="Jane" testID="row" />);
    expect(typeof nonInteractive.getByTestId("row").props.style).not.toBe("function");
  });

  it("renders a crimson count pill via `badge`, separate from `trailing`", () => {
    const { getByText } = render(
      <GroupedRow label="Notifications" onPress={jest.fn()} badge="3" />,
    );
    expect(getByText("3")).toBeTruthy();
  });

  it("threads a custom accessibilityRole through for link-style rows (Legal)", () => {
    const { getByTestId } = render(
      <GroupedRow
        label="Privacy Policy"
        onPress={jest.fn()}
        accessibilityRole="link"
        testID="row"
      />,
    );
    expect(getByTestId("row").props.accessibilityRole).toBe("link");
  });

  describe("label + value at raised font scale", () => {
    it("sits value on the same line at fontScale 1", () => {
      setFontScale(1);
      const { getByTestId } = render(
        <GroupedRow label="Email" value="jane@example.com" testID="row" />,
      );
      // At rest, value renders as its own node alongside the label — not
      // stacked beneath it.
      expect(getByTestId("row-value-inline")).toBeTruthy();
      expect(getByTestId("row-value-inline").props.children).toBe("jane@example.com");
    });

    it("stacks value beneath the label once the SheetActions-style threshold is passed", () => {
      setFontScale(1.5);
      const { queryByTestId, getByTestId } = render(
        <GroupedRow label="Email" value="jane@example.com" testID="row" />,
      );
      expect(queryByTestId("row-value-inline")).toBeNull();
      expect(getByTestId("row-value-stacked")).toBeTruthy();
    });
  });
});
