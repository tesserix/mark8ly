// Coupons = the discount list, brought onto the increment-3 native-UX kit
// (inc3 Task 6): a collapsing header with a back affordance, enable/disable
// swipes, and a three-item long-press menu.
//
// `PATCH .../coupons/:id {status}` is idempotent and fully reversible between
// `active` and `disabled`, so both swipes fire directly with no confirm. What
// carries the risk here instead:
//
//  1. LEGALITY. `expired` and `scheduled` are system-managed — neither
//     transition is one a merchant can meaningfully make from a row — so
//     those coupons get NO `SwipeRow` at all. The EXPIRED fixture is the one
//     that exposes a missing gate; an active/disabled-only fixture passes
//     against code that arms every row. The backend agrees: `Service.Patch`
//     rejects anything but `active`/`disabled` up front with
//     `status must be 'active' or 'disabled'` (coupon/service.go:159-169),
//     so an ungated row would produce a visible validation error rather than
//     a harmless no-op.
//  2. NO DELETE ITEM, and that is a correctness decision rather than a scope
//     one — see the test that pins it.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

// Local mock (it wins over `__mocks__/react-native-reanimated.js`) — the
// global one carries neither `Animated.FlatList` (which drives the
// CollapsingHeader) nor the `FadeIn` entering animation in full. Same shape
// as products-screen.test.tsx's. A test-environment gap, not a semantics
// change.
jest.mock("react-native-reanimated", () => {
  const { View, FlatList } = require("react-native");
  const { useRef } = require("react");
  class ChainableAnimation {
    duration() {
      return this;
    }
    easing() {
      return this;
    }
  }
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
    default: { View, FlatList },
    FadeIn: new ChainableAnimation(),
    Easing: { bezier: () => (t: number) => t },
    Extrapolation: { CLAMP: "clamp", EXTEND: "extend", IDENTITY: "identity" },
    interpolate,
    useAnimatedStyle: (factory: () => unknown) => factory(),
    useDerivedValue: (factory: () => number) => ({ value: factory() }),
    useAnimatedScrollHandler: (handler: unknown) => handler,
    useSharedValue: (initial: number) => {
      const ref = useRef(undefined) as { current: { value: number } | undefined };
      if (ref.current === undefined) ref.current = { value: initial };
      return ref.current;
    },
    runOnJS: (fn: (...args: unknown[]) => unknown) => fn,
    withSpring: (toValue: number) => toValue,
    withTiming: (toValue: number) => toValue,
    useReducedMotion: jest.fn(() => false),
  };
});

const mockPush = jest.fn();
const mockBack = jest.fn();
jest.mock("expo-router", () => ({
  useRouter: () => ({ push: mockPush, back: mockBack }),
}));

jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: {
    actionSucceeded: jest.fn(() => Promise.resolve()),
    actionFailed: jest.fn(() => Promise.resolve()),
    swipeThreshold: jest.fn(() => Promise.resolve()),
    menuOpen: jest.fn(() => Promise.resolve()),
    selectionChanged: jest.fn(() => Promise.resolve()),
  },
}));

jest.mock("@repo/mobile-shared/stores/tenant-store", () => ({
  useTenantStore: (selector: (s: unknown) => unknown) =>
    selector({ activeStore: { currency_code: "AUD" } }),
}));

// --- controllable coupon list -------------------------------------------
interface ListParams {
  status?: string;
  search?: string;
}
const mockListCalls: (ListParams | undefined)[] = [];
let mockCoupons: unknown[] = [];
let mockIsError = false;
let mockIsLoading = false;
const mockRefetch = jest.fn();

jest.mock("@/lib/hooks/use-coupons", () => ({
  useCoupons: (params?: ListParams) => {
    mockListCalls.push(params);
    const rows = params?.status
      ? mockCoupons.filter((c) => (c as { status: string }).status === params.status)
      : mockCoupons;
    return {
      data: mockIsError ? undefined : { pages: [{ data: rows, total: rows.length, page: 1 }] },
      isLoading: mockIsLoading,
      isRefetching: false,
      isError: mockIsError,
      refetch: mockRefetch,
      fetchNextPage: jest.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
    };
  },
}));

const mockPatch = jest.fn();
jest.mock("@/lib/admin-api/coupon-actions", () => ({
  usePatchCoupon: () => ({ mutate: mockPatch, isPending: false }),
}));

import { act, fireEvent, render, within } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import CouponsScreen from "../app/(tabs)/more/marketing/coupons/index";
import { CouponRow } from "@/components/marketing/CouponRow";
import { theme } from "@/lib/theme";
import {
  CONSTRUCTIVE_TONE,
  DESTRUCTIVE_TONE,
  assertNoAutoFire,
  assertSwipeConvention,
  swipeRow,
  type Root,
  type RowActions,
} from "../test-utils/swipe-convention";
import type { Coupon } from "@repo/mobile-shared/api/types";

function coupon(over: Partial<Coupon> = {}): Coupon {
  return {
    id: "c-active",
    code: "BONDI10",
    title: "Ten off the summer range",
    description: null,
    type: "percentage",
    value: "10",
    currency_code: "AUD",
    min_purchase: null,
    max_discount: null,
    usage_limit: null,
    per_customer: 1,
    target_type: "all",
    target_ids: [],
    stackable: false,
    starts_at: "2026-06-01T00:00:00Z",
    ends_at: null,
    status: "active",
    usage_count: 4,
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...over,
  } as unknown as Coupon;
}

const ACTIVE = coupon();
const DISABLED = coupon({ id: "c-disabled", code: "COOGEE5", status: "disabled" });
// The fixture that earns its keep: `expired` is system-managed, so it is the
// one state where BOTH toggles are illegal and the whole `SwipeRow` must be
// absent. An active/disabled-only fixture passes against code that gates
// nothing at all.
const EXPIRED = coupon({ id: "c-expired", code: "WINTER20", status: "expired" });

beforeEach(() => {
  jest.clearAllMocks();
  mockListCalls.length = 0;
  mockCoupons = [ACTIVE, DISABLED, EXPIRED];
  mockIsError = false;
  mockIsLoading = false;
});

function longPress(getByTestId: (id: string) => unknown, id: string) {
  fireEvent(getByTestId(`coupon-row-${id}`) as never, "longPress");
}

function couponRows(root: Root) {
  return root.findAllByType(CouponRow);
}

/**
 * One row's mounted `CouponRow` element.
 *
 * The busy guard is asserted on `onLongPress` THROUGH THIS, not through
 * "firing longPress produced no sheet": `fireEvent(el, "longPress")` against
 * an element with no handler is a silent no-op, so that proxy passes both
 * when the guard holds and when the event never dispatched at all.
 */
function couponRow(root: Root, id: string) {
  return couponRows(root).find((n) => (n.props as { coupon: Coupon }).coupon.id === id);
}

describe("Coupons — header", () => {
  it("titles the screen with the editorial collapsing header", () => {
    const { getAllByText } = render(<CouponsScreen />);
    expect(getAllByText("Coupons").length).toBeGreaterThan(0);
    expect(getAllByText("MARKETING").length).toBeGreaterThan(0);
  });

  it("keeps a back affordance, in its own nav row", () => {
    const { getByTestId } = render(<CouponsScreen />);
    expect(getByTestId("collapsing-header-nav-row")).toBeTruthy();
    expect(getByTestId("collapsing-header-leading")).toBeTruthy();
  });

  it("goes back when the chevron is tapped", () => {
    const { getByLabelText } = render(<CouponsScreen />);
    fireEvent.press(getByLabelText("Go back"));
    expect(mockBack).toHaveBeenCalledTimes(1);
  });

  it("keeps the new-coupon button reachable in the right slot", () => {
    const { getByLabelText } = render(<CouponsScreen />);
    fireEvent.press(getByLabelText("New coupon"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/coupons/new");
  });

  it("drives the header from the list's own scroll offset", () => {
    const { getByTestId } = render(<CouponsScreen />);
    const list = getByTestId("coupons-list");
    expect(list.props.onScroll).toBeTruthy();
    expect(list.props.scrollEventThrottle).toBe(16);
    expect(list.props.ListHeaderComponent).toBeFalsy();
  });
});

describe("Coupons — swipe", () => {
  it("offers Enable on the LEADING edge for a disabled coupon, in the accent tone", () => {
    const row = swipeRow(render(<CouponsScreen />).UNSAFE_root, "swipe-c-disabled")
      ?.props as RowActions;
    expect(row.leadingActions).toHaveLength(1);
    expect(row.leadingActions?.[0]).toMatchObject({ key: "enable", tone: CONSTRUCTIVE_TONE });
    expect(row.trailingActions ?? []).toHaveLength(0);
  });

  it("offers Disable on the TRAILING edge for an active coupon, in the NEUTRAL tone", () => {
    const row = swipeRow(render(<CouponsScreen />).UNSAFE_root, "swipe-c-active")
      ?.props as RowActions;
    expect(row.trailingActions).toHaveLength(1);
    expect(row.trailingActions?.[0]).toMatchObject({ key: "disable", tone: "neutral" });
    expect(row.leadingActions ?? []).toHaveLength(0);
  });

  // Dismissive, not destructive: reversible, idempotent, and the trailing
  // edge is a POSITION not a tone. Painting it oxblood would tell a
  // merchant's thumb that switching a code off is as final as cancelling an
  // order.
  it("does not paint Disable as destructive", () => {
    const row = swipeRow(render(<CouponsScreen />).UNSAFE_root, "swipe-c-active")
      ?.props as RowActions;
    expect(row.trailingActions?.[0].tone).not.toBe(DESTRUCTIVE_TONE);
  });

  // The gate the EXPIRED fixture exists for. Neither transition is legal, so
  // the gesture is not merely inert — it is ABSENT.
  it("mounts NO SwipeRow on an expired coupon", () => {
    const { UNSAFE_root } = render(<CouponsScreen />);
    expect(swipeRow(UNSAFE_root, "swipe-c-expired")).toBeUndefined();
  });

  it("mounts NO SwipeRow on a scheduled coupon either", () => {
    mockCoupons = [ACTIVE, coupon({ id: "c-sched", code: "SPRING", status: "scheduled" })];
    const { UNSAFE_root } = render(<CouponsScreen />);
    expect(swipeRow(UNSAFE_root, "swipe-c-sched")).toBeUndefined();
  });

  it("holds the app-wide swipe convention on every row", () => {
    assertSwipeConvention(render(<CouponsScreen />).UNSAFE_root);
  });

  it("never opts any action into full-swipe auto-fire", () => {
    assertNoAutoFire(render(<CouponsScreen />).UNSAFE_root);
  });

  // Tone → paint, so a tone swap is caught at the pixel and not only at the
  // prop. Moss is this screen's ONE accent and it is spent on Enable.
  it("paints Enable moss and Disable in the neutral sink", () => {
    const { getByTestId } = render(<CouponsScreen />);
    const enable = StyleSheet.flatten(
      getByTestId("swipe-c-disabled-action-enable").props.style,
    );
    const disable = StyleSheet.flatten(
      getByTestId("swipe-c-active-action-disable").props.style,
    );
    expect(enable.backgroundColor).toBe(theme.colors.accent);
    expect(disable.backgroundColor).toBe(theme.colors.sink);
    expect(disable.backgroundColor).not.toBe(theme.colors.danger);
  });

  it("Enable switches the coupon on directly — the PATCH needs no extra input", () => {
    const { getByTestId } = render(<CouponsScreen />);
    fireEvent.press(getByTestId("swipe-c-disabled-action-enable"));
    expect(mockPatch).toHaveBeenCalledTimes(1);
    expect(mockPatch.mock.calls[0][0]).toEqual({
      id: "c-disabled",
      body: { status: "active" },
    });
  });

  it("Disable switches the coupon off directly", () => {
    const { getByTestId } = render(<CouponsScreen />);
    fireEvent.press(getByTestId("swipe-c-active-action-disable"));
    expect(mockPatch).toHaveBeenCalledTimes(1);
    expect(mockPatch.mock.calls[0][0]).toEqual({ id: "c-active", body: { status: "disabled" } });
  });
});

describe("Coupons — long-press menu", () => {
  const KEYS = ["edit", "enable", "disable"];

  // `snapPoints` memoises on `items.length`, so a dropped item resizes the
  // sheet under the merchant's thumb. Illegal actions are DISABLED, never
  // dropped.
  it("always renders three items so the sheet never resizes", () => {
    const { getByTestId, getAllByTestId, queryAllByTestId } = render(<CouponsScreen />);

    longPress(getByTestId, "c-active");
    expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(3);
    for (const key of KEYS) expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();

    fireEvent.press(getByTestId("action-sheet-item-edit"));
    expect(queryAllByTestId(/^action-sheet-item-/)).toHaveLength(0);
    // The same three on the EXPIRED coupon, where BOTH toggles are illegal —
    // the sheet keeps its height.
    longPress(getByTestId, "c-expired");
    expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(3);
    for (const key of KEYS) expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();
  });

  /**
   * NO DELETE ITEM — and this is a CORRECTNESS decision, not a scope one.
   *
   * `DELETE /coupons/:id` does not delete. It calls `SoftDisable`, returns
   * `200 {"message":"coupon disabled"}` (handlers/admin/coupons.go:259), logs
   * the audit action as a deactivation, and LEAVES THE ROW IN THE LIST. A
   * menu item labelled "Delete" that leaves the coupon visible is a lie to
   * the merchant — and "Disable", the honest endpoint for exactly that
   * outcome, is already offered one line above it.
   *
   * "Duplicate" is CUT too: no endpoint.
   */
  it("offers no Delete item (the endpoint only disables and the row would stay)", () => {
    const { getByTestId, getAllByTestId, queryByText } = render(<CouponsScreen />);
    longPress(getByTestId, "c-active");
    expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(3);
    expect(queryByText(/delete/i)).toBeNull();
    expect(queryByText(/duplicate/i)).toBeNull();
  });

  it("disables Enable on a coupon that is already active", () => {
    const { getByTestId } = render(<CouponsScreen />);
    longPress(getByTestId, "c-active");
    expect(getByTestId("action-sheet-item-enable").props.accessibilityState.disabled).toBe(true);
    fireEvent.press(getByTestId("action-sheet-item-enable"));
    expect(mockPatch).not.toHaveBeenCalled();
  });

  it("disables Disable on a coupon that is already off", () => {
    const { getByTestId } = render(<CouponsScreen />);
    longPress(getByTestId, "c-disabled");
    expect(getByTestId("action-sheet-item-disable").props.accessibilityState.disabled).toBe(true);
    fireEvent.press(getByTestId("action-sheet-item-disable"));
    expect(mockPatch).not.toHaveBeenCalled();
  });

  // 🔴 The legality gate in the sheet. `expired` is system-managed: neither
  // toggle is a transition a merchant may make, so BOTH are greyed — at
  // constant sheet height.
  it("greys BOTH toggles on an expired coupon rather than dropping them", () => {
    const { getByTestId } = render(<CouponsScreen />);
    longPress(getByTestId, "c-expired");
    expect(getByTestId("action-sheet-item-enable").props.accessibilityState.disabled).toBe(true);
    expect(getByTestId("action-sheet-item-disable").props.accessibilityState.disabled).toBe(true);
    fireEvent.press(getByTestId("action-sheet-item-enable"));
    fireEvent.press(getByTestId("action-sheet-item-disable"));
    expect(mockPatch).not.toHaveBeenCalled();
    // Edit stays live — an expired coupon's dates are exactly what a merchant
    // would go in to change.
    expect(getByTestId("action-sheet-item-edit").props.accessibilityState.disabled).not.toBe(true);
  });

  it("Edit opens the coupon detail screen", () => {
    const { getByTestId } = render(<CouponsScreen />);
    longPress(getByTestId, "c-active");
    fireEvent.press(getByTestId("action-sheet-item-edit"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/coupons/c-active");
  });

  // Neither toggle is destructive: both are reversible, so nothing in this
  // sheet is painted oxblood. Asserted so a later "Delete" cannot slip back
  // in wearing the danger tone.
  it("paints no item in the sheet as destructive", () => {
    const { getByTestId } = render(<CouponsScreen />);
    longPress(getByTestId, "c-active");
    for (const key of KEYS) {
      const item = within(getByTestId(`action-sheet-item-${key}`));
      const label = item.getAllByText(/./)[0];
      expect(StyleSheet.flatten(label.props.style)?.color).not.toBe(theme.colors.danger);
    }
  });

  it("enables an expired-then-edited coupon from the menu once it is disabled", () => {
    const { getByTestId } = render(<CouponsScreen />);
    longPress(getByTestId, "c-disabled");
    fireEvent.press(getByTestId("action-sheet-item-enable"));
    expect(mockPatch.mock.calls[0][0]).toEqual({ id: "c-disabled", body: { status: "active" } });
  });
});

describe("Coupons — no optimistic hide", () => {
  // `usePatchCoupon` invalidates ["coupons"], which prefix-matches this
  // screen's own ["coupons", "list", status, search] key — the list refetches
  // itself and is the authority on its own contents.
  it("leaves the row on screen after Disable", () => {
    const { getByTestId } = render(<CouponsScreen />);
    fireEvent.press(getByTestId("swipe-c-active-action-disable"));
    expect(getByTestId("coupon-row-c-active")).toBeTruthy();
  });

  it("suppresses BOTH the swipe and the long-press while that row's request is open", () => {
    const { getByTestId, queryByTestId, UNSAFE_root } = render(<CouponsScreen />);
    fireEvent.press(getByTestId("swipe-c-active-action-disable"));

    expect(swipeRow(UNSAFE_root, "swipe-c-active")?.props.enabled).toBe(false);
    // Asserted DIRECTLY on the prop — `SwipeRow.enabled` does not reach
    // `onLongPress`, which is the whole reason both are named separately.
    expect(couponRow(UNSAFE_root, "c-active")?.props.onLongPress).toBeUndefined();
    longPress(getByTestId, "c-active");
    expect(queryByTestId("action-sheet-item-edit")).toBeNull();

    // A DIFFERENT row is untouched — the guard is per row.
    expect(swipeRow(UNSAFE_root, "swipe-c-disabled")?.props.enabled).not.toBe(false);
    expect(couponRow(UNSAFE_root, "c-disabled")?.props.onLongPress).toBeDefined();
    longPress(getByTestId, "c-disabled");
    expect(getByTestId("action-sheet-item-edit")).toBeTruthy();
  });

  it("re-enables the row once the mutation settles", () => {
    const { getByTestId, UNSAFE_root } = render(<CouponsScreen />);
    fireEvent.press(getByTestId("swipe-c-active-action-disable"));
    act(() => (mockPatch.mock.calls[0][1].onSuccess as () => void)());

    expect(swipeRow(UNSAFE_root, "swipe-c-active")?.props.enabled).not.toBe(false);
    expect(couponRow(UNSAFE_root, "c-active")?.props.onLongPress).toBeDefined();
  });

  it("releases a row on failure too, so a failed toggle is retryable", () => {
    const { getByTestId, UNSAFE_root } = render(<CouponsScreen />);
    fireEvent.press(getByTestId("swipe-c-active-action-disable"));
    expect(swipeRow(UNSAFE_root, "swipe-c-active")?.props.enabled).toBe(false);

    act(() => (mockPatch.mock.calls[0][1].onError as (e: Error) => void)(new Error("500")));
    expect(swipeRow(UNSAFE_root, "swipe-c-active")?.props.enabled).not.toBe(false);
  });

  it("fires the success and failure haptics exactly once each", () => {
    const { getByTestId } = render(<CouponsScreen />);
    fireEvent.press(getByTestId("swipe-c-active-action-disable"));
    act(() => (mockPatch.mock.calls[0][1].onSuccess as () => void)());
    expect(adminHaptics.actionSucceeded).toHaveBeenCalledTimes(1);

    fireEvent.press(getByTestId("swipe-c-disabled-action-enable"));
    act(() => (mockPatch.mock.calls[1][1].onError as (e: Error) => void)(new Error("x")));
    expect(adminHaptics.actionFailed).toHaveBeenCalledTimes(1);
  });

  it("guards two in-flight rows at once", () => {
    const { getByTestId, UNSAFE_root } = render(<CouponsScreen />);
    fireEvent.press(getByTestId("swipe-c-active-action-disable"));
    fireEvent.press(getByTestId("swipe-c-disabled-action-enable"));

    expect(mockPatch).toHaveBeenCalledTimes(2);
    expect(swipeRow(UNSAFE_root, "swipe-c-active")?.props.enabled).toBe(false);
    expect(swipeRow(UNSAFE_root, "swipe-c-disabled")?.props.enabled).toBe(false);

    act(() => (mockPatch.mock.calls[0][1].onSuccess as () => void)());
    expect(swipeRow(UNSAFE_root, "swipe-c-active")?.props.enabled).not.toBe(false);
    expect(swipeRow(UNSAFE_root, "swipe-c-disabled")?.props.enabled).toBe(false);
  });
});

/**
 * Task 14 retrofit — a failed toggle is EXPLAINED, not only felt.
 *
 * The coupon row carries a status badge, so a failed toggle is at least not
 * invisible — but "the badge didn't move" is not a reason, and a merchant
 * switching a code off before a sale needs to know whether it actually went.
 */
describe("Coupons — surfacing a failed toggle", () => {
  function failDisable(root: ReturnType<typeof render>, error: unknown) {
    fireEvent.press(root.getByTestId("swipe-c-active-action-disable"));
    act(() => (mockPatch.mock.calls.at(-1)?.[1].onError as (e: unknown) => void)(error));
  }

  it("says nothing at all until something fails", () => {
    const root = render(<CouponsScreen />);
    fireEvent.press(root.getByTestId("swipe-c-active-action-disable"));
    expect(root.queryByTestId("action-failure-notice")).toBeNull();
  });

  it("names the action the merchant actually took", () => {
    const root = render(<CouponsScreen />);
    failDisable(root, new TypeError("Network request failed"));
    expect(root.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't switch this coupon off",
    );
  });

  it("names Enable when the enable path is the one that failed", () => {
    const root = render(<CouponsScreen />);
    fireEvent.press(root.getByTestId("swipe-c-disabled-action-enable"));
    act(() => (mockPatch.mock.calls.at(-1)?.[1].onError as (e: unknown) => void)(new Error("x")));
    expect(root.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't switch this coupon on",
    );
  });

  it("clears itself when a later action succeeds", () => {
    const root = render(<CouponsScreen />);
    failDisable(root, new Error("x"));
    fireEvent.press(root.getByTestId("swipe-c-disabled-action-enable"));
    act(() => (mockPatch.mock.calls.at(-1)?.[1].onSuccess as () => void)());
    expect(root.queryByTestId("action-failure-notice")).toBeNull();
  });
});

describe("Coupons — filters and rows", () => {
  it("renders the full filter set as pill chips", () => {
    const { getByTestId } = render(<CouponsScreen />);
    for (const key of ["all", "active", "scheduled", "expired", "disabled"]) {
      expect(getByTestId(`filter-chip-${key}`)).toBeTruthy();
    }
  });

  it("asks the backend for status=expired when that chip is tapped", () => {
    const { getByTestId } = render(<CouponsScreen />);
    mockListCalls.length = 0;
    fireEvent.press(getByTestId("filter-chip-expired-target"));
    expect(mockListCalls).toContainEqual({ status: "expired" });
  });

  it("opens the coupon on tap", () => {
    const { getByTestId } = render(<CouponsScreen />);
    fireEvent.press(getByTestId("coupon-row-c-active"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/coupons/c-active");
  });
});
