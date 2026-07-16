jest.mock("expo-haptics", () => ({
  notificationAsync: jest.fn(),
  NotificationFeedbackType: { Success: "success" },
}));

// Only useReducedMotion is exercised here — no Animated components render in
// this hook, so the fuller virtual mock in variant-row/section-disclosure
// tests isn't needed.
jest.mock("react-native-reanimated", () => ({
  __esModule: true,
  useReducedMotion: jest.fn(() => false),
}));

import { renderHook, act } from "@testing-library/react-native";
import type { ScrollView } from "react-native";
import { useCreatedBanner } from "@/lib/hooks/use-created-banner";

describe("useCreatedBanner — jumpTo honors reduced motion", () => {
  const reanimated = jest.requireMock("react-native-reanimated") as {
    useReducedMotion: jest.Mock;
  };

  afterEach(() => {
    reanimated.useReducedMotion.mockReturnValue(false);
  });

  it("scrolls animated when reduced motion is off", () => {
    const scrollTo = jest.fn();
    const scrollRef = { current: { scrollTo } } as unknown as React.RefObject<ScrollView | null>;
    const { result } = renderHook(() => useCreatedBanner("1", scrollRef));

    act(() => result.current.registerSectionOffset("photos", 100));
    act(() => result.current.jumpTo("photos"));

    expect(scrollTo).toHaveBeenCalledWith({ y: 84, animated: true });
  });

  it("scrolls WITHOUT animation when reduced motion is enabled", () => {
    reanimated.useReducedMotion.mockReturnValue(true);
    const scrollTo = jest.fn();
    const scrollRef = { current: { scrollTo } } as unknown as React.RefObject<ScrollView | null>;
    const { result } = renderHook(() => useCreatedBanner("1", scrollRef));

    act(() => result.current.registerSectionOffset("variants", 200));
    act(() => result.current.jumpTo("variants"));

    expect(scrollTo).toHaveBeenCalledWith({ y: 184, animated: false });
  });
});
