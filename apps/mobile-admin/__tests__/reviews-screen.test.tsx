// Reviews = the moderation queue, brought onto the increment-3 native-UX kit
// (inc3 Task 4): a collapsing header with a back affordance, approve/reject
// swipes, and a three-item long-press menu.
//
// The cleanest swipe screen in the increment — `POST /reviews/:id/approve`
// and `/reject` take NO body and are explicitly idempotent server-side, so
// both fire directly with no confirm. What that leaves worth pinning:
//
//  1. WHICH gesture is legal. A non-pending review gets NO `SwipeRow` at all
//     — an armed gesture that can only re-assert a decision already made is
//     worse than no gesture. The APPROVED and REJECTED fixtures are what
//     expose a missing gate; a pending-only fixture passes against code that
//     arms every row.
//  2. Reject fires DIRECTLY despite its danger tone. That is deliberate (it
//     is idempotent and reversible by approving) and it is the row that
//     proves the app-wide invariant isn't merely "danger means confirm".
//  3. The cross-screen invalidation. `review-actions.ts` invalidates
//     ["reviews"], ["review", id] AND ["dashboard"] — the dashboard carries
//     its own `stats.pending_reviews`. That is pinned in
//     mutation-invalidations.test.tsx; this file pins the screen's half, that
//     it goes through those hooks at all rather than hand-rolling a mutation.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

// Local mock (it wins over `__mocks__/react-native-reanimated.js`) because
// this screen needs BOTH surfaces at once and the global mock carries neither
// in full: `Animated.FlatList` (the virtualized list that drives the
// CollapsingHeader) and the `FadeIn` entering animation the list wrapper has
// used since inc2. Same shape as products-screen.test.tsx's. This is a
// test-environment gap, not a semantics change.
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

// --- controllable review list -------------------------------------------
// The params the hook is called with ARE the assertion for the filter chips.
interface ListParams {
  status?: string;
}
const mockListCalls: (ListParams | undefined)[] = [];
let mockReviews: unknown[] = [];
let mockIsError = false;
let mockIsLoading = false;
const mockRefetch = jest.fn();

jest.mock("@/lib/hooks/use-reviews", () => ({
  useReviews: (params?: ListParams) => {
    mockListCalls.push(params);
    const rows = params?.status
      ? mockReviews.filter((r) => (r as { status: string }).status === params.status)
      : mockReviews;
    return {
      data: mockIsError
        ? undefined
        : { pages: [{ data: rows, meta: { page: 1, page_size: 50, total: rows.length, total_pages: 1 } }] },
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

const mockApprove = jest.fn();
const mockReject = jest.fn();
jest.mock("@/lib/admin-api/review-actions", () => ({
  useApproveReview: () => ({ mutate: mockApprove, isPending: false }),
  useRejectReview: () => ({ mutate: mockReject, isPending: false }),
}));

import { act, fireEvent, render, within } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import ReviewsScreen from "../app/(tabs)/customers/reviews/index";
import { ReviewRow } from "@/components/reviews/ReviewRow";
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
import type { Review } from "@repo/mobile-shared/api/types";

function review(over: Partial<Review> = {}): Review {
  return {
    id: "rv1",
    product_id: "p1",
    customer_name: "Sofia Reyes",
    customer_email: "sofia@example.com",
    rating: 4,
    title: "Lovely linen",
    content: "Wore it all summer.",
    status: "pending",
    verified_purchase: true,
    featured: false,
    helpful_count: 0,
    not_helpful_count: 0,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
    media: [],
    replies: [],
    ...over,
  } as unknown as Review;
}

const PENDING = review();
// The two fixtures that earn their keep: a decided review is the state where
// BOTH swipes are illegal and the whole `SwipeRow` must be absent. A
// pending-only fixture passes against code that gates nothing.
const APPROVED = review({ id: "rv2", status: "approved", title: "Great fit" });
const REJECTED = review({ id: "rv3", status: "rejected", title: "Spam link" });

beforeEach(() => {
  jest.clearAllMocks();
  mockListCalls.length = 0;
  mockReviews = [PENDING, APPROVED, REJECTED];
  mockIsError = false;
  mockIsLoading = false;
});

/** Opens the long-press menu on a given row. */
function longPress(getByTestId: (id: string) => unknown, id: string) {
  fireEvent(getByTestId(`review-row-${id}`) as never, "longPress");
}

/** Every mounted `ReviewRow`, so its props can be read directly. */
function reviewRows(root: Root) {
  return root.findAllByType(ReviewRow);
}

/**
 * One row's mounted `ReviewRow` element.
 *
 * The busy guard is asserted on `onLongPress` THROUGH THIS, not through
 * "firing longPress produced no sheet": `fireEvent(el, "longPress")` against
 * an element with no handler is a silent no-op, so that proxy passes both
 * when the guard holds and when the event never dispatched at all.
 */
function reviewRow(root: Root, id: string) {
  return reviewRows(root).find((n) => (n.props as { review: Review }).review.id === id);
}

describe("Reviews — header", () => {
  it("titles the screen with the editorial collapsing header", () => {
    const { getAllByText } = render(<ReviewsScreen />);
    // CollapsingHeader mounts both the expanded and the collapsed layer.
    expect(getAllByText("Reviews").length).toBeGreaterThan(0);
    expect(getAllByText("CUSTOMERS").length).toBeGreaterThan(0);
  });

  // Reviews is a NESTED route reached from Customers' header link, so unlike
  // the tab roots it MUST keep a back affordance — and the chevron gets its
  // own nav row so the eyebrow, title, chips and rows all share gutter 20.
  it("keeps a back affordance, in its own nav row", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    expect(getByTestId("collapsing-header-nav-row")).toBeTruthy();
    expect(getByTestId("collapsing-header-leading")).toBeTruthy();
  });

  it("goes back when the chevron is tapped", () => {
    const { getByLabelText } = render(<ReviewsScreen />);
    fireEvent.press(getByLabelText("Go back"));
    expect(mockBack).toHaveBeenCalledTimes(1);
  });

  it("drives the header from the list's own scroll offset", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    const list = getByTestId("reviews-list");
    expect(list.props.onScroll).toBeTruthy();
    expect(list.props.scrollEventThrottle).toBe(16);
    // Chips are pinned ABOVE the list, never in `ListHeaderComponent` — that
    // re-introduces the ~110pt dead-paper bug.
    expect(list.props.ListHeaderComponent).toBeFalsy();
  });
});

describe("Reviews — swipe", () => {
  it("offers Approve on the LEADING edge for a pending review, in the accent tone", () => {
    const { UNSAFE_root } = render(<ReviewsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-rv1")?.props as RowActions;
    expect(row.leadingActions).toHaveLength(1);
    expect(row.leadingActions?.[0]).toMatchObject({ key: "approve", tone: CONSTRUCTIVE_TONE });
  });

  it("offers Reject on the TRAILING edge for a pending review, in the danger tone", () => {
    const { UNSAFE_root } = render(<ReviewsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-rv1")?.props as RowActions;
    expect(row.trailingActions).toHaveLength(1);
    expect(row.trailingActions?.[0]).toMatchObject({ key: "reject", tone: DESTRUCTIVE_TONE });
  });

  // The gate the APPROVED/REJECTED fixtures exist for: no `SwipeRow` AT ALL,
  // matching Orders' terminal-status rule. Not "an empty action array" — the
  // whole gesture container is absent, so nothing is armed to be dragged.
  it("mounts NO SwipeRow on an already-approved review", () => {
    const { UNSAFE_root } = render(<ReviewsScreen />);
    expect(swipeRow(UNSAFE_root, "swipe-rv2")).toBeUndefined();
  });

  it("mounts NO SwipeRow on an already-rejected review", () => {
    const { UNSAFE_root } = render(<ReviewsScreen />);
    expect(swipeRow(UNSAFE_root, "swipe-rv3")).toBeUndefined();
  });

  it("holds the app-wide swipe convention on every row", () => {
    assertSwipeConvention(render(<ReviewsScreen />).UNSAFE_root);
  });

  it("never opts any action into full-swipe auto-fire", () => {
    assertNoAutoFire(render(<ReviewsScreen />).UNSAFE_root);
  });

  // Tone → paint, so a tone swap is caught at the pixel and not only at the
  // prop. Moss is this screen's ONE accent and it is spent on Approve.
  it("paints Approve moss and Reject danger", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    const approve = StyleSheet.flatten(getByTestId("swipe-rv1-action-approve").props.style);
    const reject = StyleSheet.flatten(getByTestId("swipe-rv1-action-reject").props.style);
    expect(approve.backgroundColor).toBe(theme.colors.accent);
    expect(reject.backgroundColor).toBe(theme.colors.danger);
  });

  it("Approve moderates the review directly — the POST needs no body", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    fireEvent.press(getByTestId("swipe-rv1-action-approve"));
    expect(mockApprove).toHaveBeenCalledTimes(1);
    expect(mockApprove.mock.calls[0][0]).toBe("rv1");
  });

  /**
   * Reject fires DIRECTLY, with no confirm, despite carrying the danger tone.
   *
   * It is idempotent server-side and reversible by approving, so a confirm
   * would be a tax on the most common triage action in the app. The tone is
   * about which OUTCOME the gesture picks for the customer's review, not
   * about how final it is — the same distinction Products' neutral "Set to
   * draft" makes from the other side.
   */
  it("Reject moderates directly too — the danger tone is not a confirm", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    fireEvent.press(getByTestId("swipe-rv1-action-reject"));
    expect(mockReject).toHaveBeenCalledTimes(1);
    expect(mockReject.mock.calls[0][0]).toBe("rv1");
  });
});

describe("Reviews — long-press menu", () => {
  const KEYS = ["reply", "approve", "reject"];

  // `snapPoints` memoises on `items.length`, so a dropped item resizes the
  // sheet under the merchant's thumb. Illegal actions are DISABLED, never
  // dropped. Report is CUT — no endpoint exists for it.
  it("always renders three items so the sheet never resizes", () => {
    const { getByTestId, getAllByTestId, queryAllByTestId } = render(<ReviewsScreen />);

    longPress(getByTestId, "rv1");
    expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(3);
    for (const key of KEYS) expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();

    // The same three on a state where one of them is illegal.
    fireEvent.press(getByTestId("action-sheet-item-reply"));
    expect(queryAllByTestId(/^action-sheet-item-/)).toHaveLength(0);
    longPress(getByTestId, "rv2");
    expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(3);
    for (const key of KEYS) expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();
  });

  it("offers no Report item — no endpoint exists for it", () => {
    const { getByTestId, queryByText } = render(<ReviewsScreen />);
    longPress(getByTestId, "rv1");
    expect(queryByText(/report/i)).toBeNull();
  });

  it("disables Approve on an already-approved review rather than dropping it", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    longPress(getByTestId, "rv2");
    expect(getByTestId("action-sheet-item-approve").props.accessibilityState.disabled).toBe(true);
    fireEvent.press(getByTestId("action-sheet-item-approve"));
    expect(mockApprove).not.toHaveBeenCalled();
  });

  it("disables Reject on an already-rejected review rather than dropping it", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    longPress(getByTestId, "rv3");
    expect(getByTestId("action-sheet-item-reject").props.accessibilityState.disabled).toBe(true);
    fireEvent.press(getByTestId("action-sheet-item-reject"));
    expect(mockReject).not.toHaveBeenCalled();
  });

  // A rejected review can still be approved, and vice versa — that is the
  // whole reason neither action is behind a confirm.
  it("can still approve a rejected review from the menu", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    longPress(getByTestId, "rv3");
    fireEvent.press(getByTestId("action-sheet-item-approve"));
    expect(mockApprove.mock.calls[0][0]).toBe("rv3");
  });

  it("Reply opens the review detail, where the reply box lives", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    longPress(getByTestId, "rv1");
    fireEvent.press(getByTestId("action-sheet-item-reply"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/customers/reviews/rv1");
  });

  // Tone → paint. Asserting only that the row EXISTS lets `tone: "danger"` be
  // deleted with the test still green.
  //
  // Scoped with `within` rather than a bare `getByText`: "Reject" and
  // "Approve" are ALSO the swipe panel labels on the pending row underneath,
  // so a global text query finds two nodes and throws before it asserts
  // anything. That collision is specific to this screen — Products' menu-only
  // "Archive" has no swipe twin.
  it("paints Reject as the destructive item, not merely labels it", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    longPress(getByTestId, "rv1");
    const rejectItem = within(getByTestId("action-sheet-item-reject")).getByText("Reject");
    expect(StyleSheet.flatten(rejectItem.props.style).color).toBe(theme.colors.danger);
    // Discriminating: a sibling in the same sheet is NOT painted danger.
    const replyItem = within(getByTestId("action-sheet-item-reply")).getByText("Reply");
    expect(StyleSheet.flatten(replyItem.props.style)?.color).not.toBe(theme.colors.danger);
  });
});

describe("Reviews — no optimistic hide", () => {
  // `useApproveReview`/`useRejectReview` invalidate ["reviews"], which
  // prefix-matches this screen's own ["reviews", status] key — the list
  // refetches itself and is the authority on its own contents.
  it("leaves the row on screen after Approve", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    fireEvent.press(getByTestId("swipe-rv1-action-approve"));
    expect(getByTestId("review-row-rv1")).toBeTruthy();
  });

  it("suppresses BOTH the swipe and the long-press while that row's request is open", () => {
    const { getByTestId, queryByTestId, UNSAFE_root } = render(<ReviewsScreen />);
    fireEvent.press(getByTestId("swipe-rv1-action-approve"));

    expect(swipeRow(UNSAFE_root, "swipe-rv1")?.props.enabled).toBe(false);
    // The long-press half asserted DIRECTLY on the prop — `SwipeRow.enabled`
    // does not reach `onLongPress`, which is the whole reason both controls
    // are named separately.
    expect(reviewRow(UNSAFE_root, "rv1")?.props.onLongPress).toBeUndefined();
    longPress(getByTestId, "rv1");
    expect(queryByTestId("action-sheet-item-approve")).toBeNull();

    // A DIFFERENT row is untouched — the guard is per row.
    expect(reviewRow(UNSAFE_root, "rv2")?.props.onLongPress).toBeDefined();
    longPress(getByTestId, "rv2");
    expect(getByTestId("action-sheet-item-approve")).toBeTruthy();
  });

  it("re-enables the row once the mutation settles", () => {
    const { getByTestId, UNSAFE_root } = render(<ReviewsScreen />);
    fireEvent.press(getByTestId("swipe-rv1-action-approve"));
    act(() => (mockApprove.mock.calls[0][1].onSuccess as () => void)());

    expect(swipeRow(UNSAFE_root, "swipe-rv1")?.props.enabled).not.toBe(false);
    expect(reviewRow(UNSAFE_root, "rv1")?.props.onLongPress).toBeDefined();
  });

  it("releases a row on failure too, so a failed action is retryable", () => {
    const { getByTestId, UNSAFE_root } = render(<ReviewsScreen />);
    fireEvent.press(getByTestId("swipe-rv1-action-reject"));
    expect(swipeRow(UNSAFE_root, "swipe-rv1")?.props.enabled).toBe(false);

    act(() => (mockReject.mock.calls[0][1].onError as (e: Error) => void)(new Error("500")));
    expect(swipeRow(UNSAFE_root, "swipe-rv1")?.props.enabled).not.toBe(false);
  });

  // `settleCallbacks` owns the haptics — the screen must not fire its own, or
  // every action buzzes twice.
  it("fires the success and failure haptics exactly once each", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    fireEvent.press(getByTestId("swipe-rv1-action-approve"));
    act(() => (mockApprove.mock.calls[0][1].onSuccess as () => void)());
    expect(adminHaptics.actionSucceeded).toHaveBeenCalledTimes(1);

    fireEvent.press(getByTestId("swipe-rv1-action-reject"));
    act(() => (mockReject.mock.calls[0][1].onError as (e: Error) => void)(new Error("x")));
    expect(adminHaptics.actionFailed).toHaveBeenCalledTimes(1);
  });
});

/**
 * Task 14 retrofit — a failed action is EXPLAINED, not only felt.
 *
 * Every fixture here drives the mutation's `onError`. A test whose mutation
 * succeeds proves nothing about this surface: before the retrofit, a failed
 * approve produced one haptic and a row whose badge still read Pending, and
 * every other assertion in this file passed anyway.
 */
describe("Reviews — surfacing a failed moderation", () => {
  function failApprove(root: ReturnType<typeof render>, error: unknown) {
    fireEvent.press(root.getByTestId("swipe-rv1-action-approve"));
    act(() => (mockApprove.mock.calls.at(-1)?.[1].onError as (e: unknown) => void)(error));
  }

  it("says nothing at all until something fails", () => {
    const root = render(<ReviewsScreen />);
    fireEvent.press(root.getByTestId("swipe-rv1-action-approve"));
    expect(root.queryByTestId("action-failure-notice")).toBeNull();
  });

  it("names the action the merchant actually took", () => {
    const root = render(<ReviewsScreen />);
    failApprove(root, new TypeError("Network request failed"));
    expect(root.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't approve this review",
    );
  });

  it("names Reject when the reject path is the one that failed", () => {
    const root = render(<ReviewsScreen />);
    fireEvent.press(root.getByTestId("swipe-rv1-action-reject"));
    act(() => (mockReject.mock.calls.at(-1)?.[1].onError as (e: unknown) => void)(new Error("x")));
    expect(root.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't reject this review",
    );
  });

  it("clears itself when a later action succeeds", () => {
    const root = render(<ReviewsScreen />);
    failApprove(root, new Error("x"));
    fireEvent.press(root.getByTestId("swipe-rv1-action-reject"));
    act(() => (mockReject.mock.calls.at(-1)?.[1].onSuccess as () => void)());
    expect(root.queryByTestId("action-failure-notice")).toBeNull();
  });
});

describe("Reviews — filters and rows", () => {
  it("renders the full filter set as pill chips", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    for (const key of ["all", "pending", "approved", "rejected"]) {
      expect(getByTestId(`filter-chip-${key}`)).toBeTruthy();
    }
  });

  it("asks the backend for status=pending when that chip is tapped", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    mockListCalls.length = 0;
    fireEvent.press(getByTestId("filter-chip-pending-target"));
    expect(mockListCalls).toContainEqual({ status: "pending" });
  });

  it("opens the review on tap", () => {
    const { getByTestId } = render(<ReviewsScreen />);
    fireEvent.press(getByTestId("review-row-rv1"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/customers/reviews/rv1");
  });
});
