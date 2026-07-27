// `react-native-gesture-handler` is globally mocked (see
// `__mocks__/react-native-gesture-handler.js`) with a `GestureDetector` that
// passes through its `gesture` prop via `__getLastGesture()`. That lets these
// tests invoke the same `onUpdate`/`onEnd` worklet bodies SwipeRow wires up
// to `Gesture.Pan()`, without driving a real drag (which RNTL/jest cannot
// do — there is no native Gesture Handler runtime under test).
import { act, render } from "@testing-library/react-native";
import { Text as RNText } from "react-native";
import * as GestureHandler from "react-native-gesture-handler";
import { SwipeRow, type SwipeAction } from "@/components/ui/SwipeRow";

// `__getLastGesture` is a test-only escape hatch the global mock
// (`__mocks__/react-native-gesture-handler.js`) adds — it isn't part of the
// real module's type surface, so it's read off the namespace import via a
// loose cast rather than a named import TypeScript would reject.
const __getLastGesture = (
  GestureHandler as unknown as {
    __getLastGesture: () => {
      handlers: {
        enabled?: boolean;
        onUpdate: (event: { translationX: number }) => void;
        onEnd: (event: { translationX: number }) => void;
      };
    } | null;
  }
).__getLastGesture;

jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: {
    swipeThreshold: jest.fn(() => Promise.resolve()),
  },
}));

import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";

const ROW_WIDTH = 400;
// Threshold is 40% of row width per the brief.
const PAST_THRESHOLD = ROW_WIDTH * 0.4 + 1;
const BELOW_THRESHOLD = ROW_WIDTH * 0.4 - 1;

function action(overrides: Partial<SwipeAction> = {}): SwipeAction {
  return {
    key: "approve",
    label: "Approve",
    icon: <RNText>icon</RNText>,
    tone: "accent",
    onPress: jest.fn(),
    ...overrides,
  };
}

function renderRow(props: Partial<React.ComponentProps<typeof SwipeRow>> = {}) {
  const utils = render(
    <SwipeRow {...props}>
      <RNText>Row content</RNText>
    </SwipeRow>,
  );
  const rowNode = utils.getByTestId("swipe-row");
  // Establish the row's measured width before any gesture is driven — the
  // component reads it from a shared value written in `onLayout`, and the
  // 40% threshold is meaningless (always 0) until this fires.
  act(() => {
    rowNode.props.onLayout({
      nativeEvent: { layout: { width: ROW_WIDTH, height: 64, x: 0, y: 0 } },
    });
  });
  const gesture = __getLastGesture();
  if (!gesture) {
    throw new Error("SwipeRow did not register a pan gesture");
  }
  return { ...utils, gesture };
}

beforeEach(() => {
  jest.clearAllMocks();
});

describe("SwipeRow", () => {
  it("fires the primary trailing action on release past the threshold", () => {
    const trailing = action({ key: "cancel", label: "Cancel", tone: "danger" });
    const { gesture } = renderRow({ trailingActions: [trailing] });

    act(() => {
      gesture.handlers.onUpdate({ translationX: -PAST_THRESHOLD });
      gesture.handlers.onEnd({ translationX: -PAST_THRESHOLD });
    });

    expect(trailing.onPress).toHaveBeenCalledTimes(1);
  });

  it("fires the primary leading action on release past the threshold", () => {
    const leading = action({ key: "approve", tone: "accent" });
    const { gesture } = renderRow({ leadingActions: [leading] });

    act(() => {
      gesture.handlers.onUpdate({ translationX: PAST_THRESHOLD });
      gesture.handlers.onEnd({ translationX: PAST_THRESHOLD });
    });

    expect(leading.onPress).toHaveBeenCalledTimes(1);
  });

  it("does not fire the action on release below the threshold", () => {
    const leading = action();
    const { gesture } = renderRow({ leadingActions: [leading] });

    act(() => {
      gesture.handlers.onUpdate({ translationX: BELOW_THRESHOLD });
      gesture.handlers.onEnd({ translationX: BELOW_THRESHOLD });
    });

    expect(leading.onPress).not.toHaveBeenCalled();
  });

  it("springs back to rest on release below the threshold", () => {
    const leading = action();
    const { gesture, getByTestId, rerender } = renderRow({ leadingActions: [leading] });

    act(() => {
      gesture.handlers.onUpdate({ translationX: BELOW_THRESHOLD });
      gesture.handlers.onEnd({ translationX: BELOW_THRESHOLD });
    });

    rerender(
      <SwipeRow leadingActions={[leading]}>
        <RNText>Row content</RNText>
      </SwipeRow>,
    );
    const content = getByTestId("swipe-row-content");
    const transform = content.props.style.find(
      (s: Record<string, unknown> | null | undefined) => s && "transform" in s,
    );
    expect(transform.transform[0].translateX).toBe(0);
  });

  it("fires the threshold haptic exactly once per crossing, not per frame", () => {
    const leading = action();
    const { gesture } = renderRow({ leadingActions: [leading] });

    act(() => {
      // Below threshold — no crossing yet.
      gesture.handlers.onUpdate({ translationX: BELOW_THRESHOLD });
      // Crosses the threshold — fires once.
      gesture.handlers.onUpdate({ translationX: PAST_THRESHOLD });
      // Still past threshold on subsequent frames — must NOT re-fire.
      gesture.handlers.onUpdate({ translationX: PAST_THRESHOLD + 10 });
      gesture.handlers.onUpdate({ translationX: PAST_THRESHOLD + 20 });
    });
    expect(adminHaptics.swipeThreshold).toHaveBeenCalledTimes(1);

    act(() => {
      // Drops back below threshold, then crosses again — a SECOND crossing.
      gesture.handlers.onUpdate({ translationX: BELOW_THRESHOLD });
      gesture.handlers.onUpdate({ translationX: PAST_THRESHOLD });
    });
    expect(adminHaptics.swipeThreshold).toHaveBeenCalledTimes(2);
  });

  it("disables the gesture when enabled is false", () => {
    const leading = action();
    const { gesture } = renderRow({ leadingActions: [leading], enabled: false });

    act(() => {
      gesture.handlers.onUpdate({ translationX: PAST_THRESHOLD });
      gesture.handlers.onEnd({ translationX: PAST_THRESHOLD });
    });

    expect(leading.onPress).not.toHaveBeenCalled();
    expect(adminHaptics.swipeThreshold).not.toHaveBeenCalled();
    expect(gesture.handlers.enabled).toBe(false);
  });
});
