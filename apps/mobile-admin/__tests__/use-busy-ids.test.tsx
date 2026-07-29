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

/**
 * The failure SEAM (inc3 Task 14).
 *
 * `settleCallbacks` is the one place every list screen's direct mutations
 * already funnel their outcome through, and it already fires
 * `adminHaptics.actionFailed()` there — so it is where a merchant-readable
 * message belongs too. The extension is strictly additive: the second
 * argument is optional and `onError` gains a parameter react-query already
 * supplies, so every pre-existing call site above still passes unchanged
 * (they are the regression test for that, and they are not edited).
 */
function apiError(status: number, code: string, message: string): Error {
  const err = new Error(message) as Error & { status: number; code: string };
  err.name = "ApiError";
  err.status = status;
  err.code = code;
  return err;
}

describe("useBusyIds — surfacing the failure", () => {
  it("has no notice until something fails", () => {
    const { result } = renderHook(() => useBusyIds());
    expect(result.current.failure).toBeNull();
  });

  it("records the server's own words, and names the action", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() =>
      result.current
        .settleCallbacks("s1", "delete this segment")
        .onError(
          apiError(409, "segment_in_use", "segment is still used by 3 campaigns and cannot be deleted"),
        ),
    );
    expect(result.current.failure?.title).toBe("Couldn't delete this segment");
    expect(result.current.failure?.detail).toContain("3 campaigns");
  });

  it("reads a lost connection differently from a server refusal", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() =>
      result.current.settleCallbacks("a", "approve this order").onError(new TypeError("Network request failed")),
    );
    const offline = result.current.failure?.detail;

    act(() =>
      result.current.settleCallbacks("a", "approve this order").onError(apiError(409, "conflict", "")),
    );
    expect(result.current.failure?.detail).not.toBe(offline);
  });

  // Triage is a queue — a merchant fires the next row long before the last
  // one settles. Two failures must stay ONE readable strip, always the
  // latest, never a growing pile.
  it("replaces the previous notice rather than stacking on it", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.settleCallbacks("a", "approve this order").onError(new Error("x")));
    const first = result.current.failure;
    act(() => result.current.settleCallbacks("b", "fulfil this order").onError(new Error("x")));
    expect(result.current.failure?.title).toBe("Couldn't fulfil this order");
    expect(result.current.failure?.key).not.toBe(first?.key);
  });

  // The same action failing twice produces byte-identical copy. The key must
  // still move, or the surface cannot re-announce it to a screen reader.
  it("advances the key even when the copy is identical", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.settleCallbacks("a", "archive this product").onError(new Error("x")));
    const first = result.current.failure;
    act(() => result.current.settleCallbacks("a", "archive this product").onError(new Error("x")));
    expect(result.current.failure?.detail).toBe(first?.detail);
    expect(result.current.failure?.key).toBeGreaterThan(first?.key ?? 0);
  });

  // A notice about the attempt before a successful retry is simply untrue.
  it("retires the notice when a later action succeeds", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.settleCallbacks("a", "archive this product").onError(new Error("x")));
    expect(result.current.failure).not.toBeNull();
    act(() => result.current.settleCallbacks("a", "archive this product").onSuccess());
    expect(result.current.failure).toBeNull();
  });

  it("retires the notice when the merchant dismisses it", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.settleCallbacks("a", "archive this product").onError(new Error("x")));
    act(() => result.current.dismissFailure());
    expect(result.current.failure).toBeNull();
  });

  // ADDITIVE, and this is the assertion that says so. Every pre-existing
  // call site passes only an id; a changed default would have rippled
  // through every screen in the increment.
  it("stays silent for a call site that passes no action label", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.settleCallbacks("a").onError(new Error("x")));
    expect(result.current.failure).toBeNull();
    expect(adminHaptics.actionFailed).toHaveBeenCalledTimes(1);
  });

  /**
   * The identity contract the four screens depend on.
   *
   * Products/Orders/Customers all memoise their row actions on
   * `busy.settleCallbacks` specifically (not on `busy`) precisely because the
   * object's identity churns on every busy transition. If reporting a failure
   * re-derived `settleCallbacks`, every row's actions — and therefore every
   * memoised `renderItem` — would be rebuilt on each failure.
   */
  it("keeps settleCallbacks stable across a failure", () => {
    const { result } = renderHook(() => useBusyIds());
    const before = result.current.settleCallbacks;
    act(() => result.current.settleCallbacks("a", "archive this product").onError(new Error("x")));
    expect(result.current.settleCallbacks).toBe(before);
  });
});
