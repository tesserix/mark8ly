jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: {
    actionSucceeded: jest.fn(() => Promise.resolve()),
    actionFailed: jest.fn(() => Promise.resolve()),
    swipeThreshold: jest.fn(() => Promise.resolve()),
    menuOpen: jest.fn(() => Promise.resolve()),
    selectionChanged: jest.fn(() => Promise.resolve()),
  },
}));

import { renderHook, act } from "@testing-library/react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { useBusyIds } from "@/lib/use-busy-ids";

beforeEach(() => {
  jest.clearAllMocks();
});

describe("useBusyIds", () => {
  it("guards only the row that was marked", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.markBusy("a"));
    expect(result.current.isBusy("a")).toBe(true);
    expect(result.current.isBusy("b")).toBe(false);
  });

  // The bug this replaces: Orders' first version used ONE slot, so marking B
  // overwrote A's guard and A's onSuccess then cleared it outright, re-arming
  // B while B's own request was still open.
  it("keeps A guarded while B is settling", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => {
      result.current.markBusy("a");
      result.current.markBusy("b");
    });
    act(() => result.current.settleCallbacks("b").onSuccess());
    expect(result.current.isBusy("a")).toBe(true);
    expect(result.current.isBusy("b")).toBe(false);
  });

  it("clears the guard on error as well as success", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.markBusy("a"));
    act(() => result.current.settleCallbacks("a").onError());
    expect(result.current.isBusy("a")).toBe(false);
  });

  it("clears the guard directly via clearBusy", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.markBusy("a"));
    act(() => result.current.clearBusy("a"));
    expect(result.current.isBusy("a")).toBe(false);
  });

  // Without a new identity, a FlatList renderItem memoised on `isBusy` never
  // re-runs and a guarded row keeps rendering as swipeable.
  it("returns a NEW isBusy identity whenever the set changes", () => {
    const { result } = renderHook(() => useBusyIds());
    const before = result.current.isBusy;
    act(() => result.current.markBusy("a"));
    expect(result.current.isBusy).not.toBe(before);
  });

  // …and the converse: a no-op mark must NOT churn identities, or every
  // memoised renderItem re-runs for nothing on each repeated call.
  it("keeps the SAME isBusy identity when the set does not change", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.markBusy("a"));
    const before = result.current.isBusy;
    act(() => result.current.markBusy("a"));
    expect(result.current.isBusy).toBe(before);
  });

  // The outcome is reported in the hand, not only on screen — this app has no
  // undo and the merchant's thumb is already moving to the next row.
  it("reports the outcome haptically", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.settleCallbacks("a").onSuccess());
    expect(adminHaptics.actionSucceeded).toHaveBeenCalledTimes(1);
    expect(adminHaptics.actionFailed).not.toHaveBeenCalled();

    act(() => result.current.settleCallbacks("a").onError());
    expect(adminHaptics.actionFailed).toHaveBeenCalledTimes(1);
    expect(adminHaptics.actionSucceeded).toHaveBeenCalledTimes(1);
  });
});
