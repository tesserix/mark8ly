// The surface itself: the strip that tells a merchant WHY the row they just
// swiped is unchanged.
//
// It is deliberately not an `Alert.alert`. Alert is modal and interruptive
// and stacks badly during triage — a merchant firing four rows in a row would
// get four dialogs to dismiss before they could touch the list again. This is
// a transient, non-blocking strip that replaces itself.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

import { render, fireEvent } from "@testing-library/react-native";
import { AccessibilityInfo, StyleSheet } from "react-native";
import { useReducedMotion } from "react-native-reanimated";
import { ActionFailureNotice } from "@/components/ui/ActionFailureNotice";
import { DOCK_BOTTOM_GAP, DOCK_HEIGHT } from "@/components/navigation/dock-metrics";
import { theme } from "@/lib/theme";

const failure = {
  key: 1,
  title: "Couldn't delete this segment",
  detail: "segment is still used by 3 campaigns and cannot be deleted.",
};

beforeEach(() => {
  jest.clearAllMocks();
  (useReducedMotion as jest.Mock).mockReturnValue(false);
});

describe("ActionFailureNotice — presence", () => {
  it("renders nothing at all when there is no failure", () => {
    const { queryByTestId } = render(
      <ActionFailureNotice failure={null} onDismiss={jest.fn()} />,
    );
    expect(queryByTestId("action-failure-notice")).toBeNull();
  });

  it("shows both what failed and why", () => {
    const { getByText } = render(
      <ActionFailureNotice failure={failure} onDismiss={jest.fn()} />,
    );
    expect(getByText("Couldn't delete this segment")).toBeTruthy();
    expect(
      getByText("segment is still used by 3 campaigns and cannot be deleted."),
    ).toBeTruthy();
  });
});

describe("ActionFailureNotice — announced, not merely shown", () => {
  // The person a silent failure hurts most is the one who cannot see a
  // transient strip at all. Showing it without announcing it would leave
  // them exactly where they started.
  it("announces the failure to a screen reader", () => {
    const announce = jest.spyOn(AccessibilityInfo, "announceForAccessibility");
    render(<ActionFailureNotice failure={failure} onDismiss={jest.fn()} />);
    expect(announce).toHaveBeenCalledWith(
      "Couldn't delete this segment. segment is still used by 3 campaigns and cannot be deleted.",
    );
  });

  // A merchant who fires the same failing action twice must hear it twice.
  // Keying the announcement on the message text alone would swallow the
  // second one, because the text is identical.
  it("re-announces an IDENTICAL message when it happens again", () => {
    const announce = jest.spyOn(AccessibilityInfo, "announceForAccessibility");
    const { rerender } = render(
      <ActionFailureNotice failure={failure} onDismiss={jest.fn()} />,
    );
    expect(announce).toHaveBeenCalledTimes(1);
    rerender(
      <ActionFailureNotice failure={{ ...failure, key: 2 }} onDismiss={jest.fn()} />,
    );
    expect(announce).toHaveBeenCalledTimes(2);
  });

  it("does not re-announce on an unrelated re-render", () => {
    const announce = jest.spyOn(AccessibilityInfo, "announceForAccessibility");
    const { rerender } = render(
      <ActionFailureNotice failure={failure} onDismiss={jest.fn()} />,
    );
    rerender(<ActionFailureNotice failure={failure} onDismiss={jest.fn()} />);
    expect(announce).toHaveBeenCalledTimes(1);
  });

  it("carries an assertive live region and an alert role", () => {
    const { getByTestId } = render(
      <ActionFailureNotice failure={failure} onDismiss={jest.fn()} />,
    );
    const copy = getByTestId("action-failure-copy");
    expect(copy.props.accessibilityLiveRegion).toBe("assertive");
    expect(copy.props.accessibilityRole).toBe("alert");
  });
});

describe("ActionFailureNotice — dismissal", () => {
  it("offers a labelled dismiss control", () => {
    const onDismiss = jest.fn();
    const { getByTestId } = render(
      <ActionFailureNotice failure={failure} onDismiss={onDismiss} />,
    );
    const button = getByTestId("action-failure-dismiss");
    expect(button.props.accessibilityLabel).toBe("Dismiss");
    expect(button.props.accessibilityRole).toBe("button");
    fireEvent.press(button);
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  // 44pt as a REAL box, not a hit-slop promise: the strip floats over a
  // scrolling list and a 20pt glyph is not a target a thumb can find.
  it("gives the dismiss control a real 44pt box", () => {
    const { getByTestId } = render(
      <ActionFailureNotice failure={failure} onDismiss={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByTestId("action-failure-dismiss").props.style);
    expect(style.width).toBeGreaterThanOrEqual(theme.touchTarget);
    expect(style.height).toBeGreaterThanOrEqual(theme.touchTarget);
  });
});

describe("ActionFailureNotice — it must not fight the dock", () => {
  // The floating Ink dock is position:"absolute", 64pt, sitting at
  // insets.bottom + DOCK_BOTTOM_GAP. A bottom-anchored strip that ignores it
  // lands underneath a control the merchant needs to leave the screen.
  it("sits clear of the floating dock", () => {
    const { getByTestId } = render(
      <ActionFailureNotice failure={failure} onDismiss={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByTestId("action-failure-notice").props.style);
    expect(style.position).toBe("absolute");
    // react-native-safe-area-context's jest mock reports a 0 bottom inset, so
    // the floor here is the dock's own height plus its gap. Asserting the
    // ARITHMETIC rather than a literal keeps this true if the dock resizes.
    expect(style.bottom).toBeGreaterThan(DOCK_HEIGHT + DOCK_BOTTOM_GAP);
  });
});

describe("ActionFailureNotice — a message in a fixed box is the bug", () => {
  /**
   * Seven silent-clipping bugs in this app have had the same shape: text
   * that scales, inside a box that does not. The most recent hid a sheet's
   * own buttons at accessibility text sizes.
   *
   * So the guard is structural rather than visual — no descendant of this
   * strip may pin a height, and no text node inside it may cap its own line
   * count. Both are asserted because either one alone silently clips.
   */
  const TALL = {
    key: 1,
    title: "Couldn't set this product to draft",
    detail:
      "segment is still used by 14 campaigns and cannot be deleted, which is a " +
      "deliberately long sentence so that it wraps to several lines at an " +
      "accessibility text size and would clip out of any fixed box.",
  };

  // Every box between the screen edge and the copy. The 44pt dismiss BUTTON
  // is deliberately excluded: it is a control, not a text box, and its fixed
  // square is the accessibility floor rather than a clipping risk.
  const COPY_BOXES = ["action-failure-notice", "action-failure-card", "action-failure-copy"];

  it.each(COPY_BOXES)("pins no height on %s", (testID) => {
    const { getByTestId } = render(<ActionFailureNotice failure={TALL} onDismiss={jest.fn()} />);
    const style = StyleSheet.flatten(getByTestId(testID).props.style);
    expect(style.height).toBeUndefined();
    expect(style.maxHeight).toBeUndefined();
  });

  it.each(["action-failure-title", "action-failure-detail"])(
    "never truncates %s to a line count",
    (testID) => {
      const { getByTestId } = render(
        <ActionFailureNotice failure={TALL} onDismiss={jest.fn()} />,
      );
      expect(getByTestId(testID).props.numberOfLines).toBeUndefined();
    },
  );

  // The copy has to be able to take the whole width the dismiss button
  // doesn't. Without this the strip renders its title on one word per line
  // at an accessibility size.
  it("lets the copy claim the remaining width", () => {
    const { getByTestId } = render(<ActionFailureNotice failure={TALL} onDismiss={jest.fn()} />);
    expect(StyleSheet.flatten(getByTestId("action-failure-copy").props.style).flex).toBe(1);
  });
});

describe("ActionFailureNotice — motion", () => {
  it("does not animate when the merchant has asked for reduced motion", () => {
    (useReducedMotion as jest.Mock).mockReturnValue(true);
    const { getByTestId } = render(
      <ActionFailureNotice failure={failure} onDismiss={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByTestId("action-failure-notice").props.style);
    // Settled state, immediately: full opacity and no offset to travel from.
    expect(style.opacity).toBe(1);
    expect(style.transform).toEqual([{ translateY: 0 }]);
  });
});
