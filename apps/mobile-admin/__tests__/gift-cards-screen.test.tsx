// Gift cards — the collapsing header (inc3 Task 1) plus Enable/Disable
// (inc3 Task 13).
//
// Task 1 deliberately shipped this screen with NO swipe and NO long-press
// menu: it was the one list screen with zero backend mutations, which
// isolated the header work from mutation risk. `POST
// /gift-cards/:id/{enable,disable}` shipped afterwards, so the gestures land
// here now. The header assertions below are unchanged and must stay that way.
//
// Two things carry the real risk and are pinned hardest:
//
//  1. WHICH cards may be swiped. 🔴 There is no `expired` STATUS in the
//     backend enum (giftcard/models.go: pending|active|disabled|depleted|
//     refunded) — expiry is a TIMESTAMP, so an expired card still reads
//     `status: "active"`. The PENDING and EXPIRED fixtures are what expose a
//     missing gate; an active/disabled-only fixture passes against
//     completely broken code.
//  2. THE PRODUCT PROMISE. Disabling freezes a balance a customer has
//     already paid for; re-enabling restores it in full. No refund is
//     issued and no balance is destroyed. The copy assertions below are that
//     promise, written down.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

// Local mock (it wins over `__mocks__/react-native-reanimated.js`) because
// this screen now needs BOTH the virtualized `Animated.FlatList` that drives
// the CollapsingHeader AND the shared-value/gesture surface `SwipeRow` uses.
// Same shape as products-screen.test.tsx's. A test-environment gap, not a
// semantics change.
jest.mock("react-native-reanimated", () => {
  const { View, FlatList } = require("react-native");
  const { useRef } = require("react");
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
    Easing: { bezier: () => (t: number) => t },
    Extrapolation: { CLAMP: "clamp", EXTEND: "extend", IDENTITY: "identity" },
    interpolate,
    useAnimatedStyle: (factory: () => unknown) => factory(),
    useDerivedValue: (factory: () => number) => ({ value: factory() }),
    useAnimatedScrollHandler: (handler: unknown) => handler,
    // Ref-backed: a real shared value is stable across re-renders, and a
    // fresh literal each render would reset both the header and any
    // in-progress SwipeRow drag.
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

interface GiftCardsQueryState {
  cards: GiftCard[];
  isLoading: boolean;
  isError: boolean;
}
const mockState: GiftCardsQueryState = { cards: [], isLoading: false, isError: false };
const mockRefetch = jest.fn();
const mockFetchNextPage = jest.fn();
/** Every `params` the screen has asked `useGiftCards` for, in order. */
const mockListParams: (unknown | undefined)[] = [];

jest.mock("@/lib/hooks/use-gift-cards", () => ({
  useGiftCards: (params?: { status?: string }) => {
    mockListParams.push(params);
    return {
      data: { pages: [{ data: mockState.cards }] },
      isLoading: mockState.isLoading,
      isRefetching: false,
      isError: mockState.isError,
      refetch: mockRefetch,
      fetchNextPage: mockFetchNextPage,
      hasNextPage: false,
      isFetchingNextPage: false,
    };
  },
}));

// Only the MUTATION is faked. `canSetGiftCardStatus` is NOT — it lives in
// the dependency-free `lib/gift-card-status.ts` and it IS the gate under
// test, so stubbing it would make every "no SwipeRow" assertion below a test
// of the stub.
const mockSetStatus = jest.fn();
jest.mock("@/lib/admin-api/gift-card-actions", () => ({
  useSetGiftCardStatus: () => ({ mutate: mockSetStatus, isPending: false }),
}));

import { act, fireEvent, render } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import GiftCardsScreen from "../app/(tabs)/more/marketing/gift-cards/index";
import { GiftCardRow } from "@/components/marketing/GiftCardRow";
import { theme } from "@/lib/theme";
import {
  CONSTRUCTIVE_TONE,
  DESTRUCTIVE_TONE,
  assertNoAutoFire,
  assertSwipeConvention,
  swipeRow,
  swipeRows,
  type Root,
  type RowActions,
} from "../test-utils/swipe-convention";
import type { GiftCard } from "@repo/mobile-shared/api/types";

function card(over: Partial<GiftCard> = {}): GiftCard {
  return {
    id: "g-active",
    code: "GIFTAAAA1234",
    code_display: "GIFT-••••-1234",
    status: "active",
    initial_balance: 10000,
    current_balance: 7500,
    currency_code: "AUD",
    created_at: "2026-07-01T00:00:00Z",
    purchased_via_storefront: false,
    ...over,
  } as GiftCard;
}

const ACTIVE = card();
const DISABLED = card({
  id: "g-disabled",
  code_display: "GIFT-••••-9999",
  status: "disabled",
});
// Paid for but not captured — 409 invalid_transition in BOTH directions.
const PENDING = card({ id: "g-pending", code_display: "GIFT-••••-0001", status: "pending" });
// Spent to zero — 409 invalid_transition in BOTH directions.
const DEPLETED = card({
  id: "g-depleted",
  code_display: "GIFT-••••-0002",
  status: "depleted",
  current_balance: 0,
});
// 🔴 The fixture that earns its keep. Still `status: "active"` — the backend
// has no `expired` status — but past `expires_at`, so the server answers 410
// `gift_card_expired` either way. A gate switching on status alone arms both
// gestures on this row.
const EXPIRED = card({
  id: "g-expired",
  code_display: "GIFT-••••-0003",
  status: "active",
  expires_at: "2020-01-01T00:00:00Z",
});

/** Every card whose two toggles are BOTH illegal. */
const UNTOGGLEABLE = [PENDING, DEPLETED, EXPIRED];

beforeEach(() => {
  jest.clearAllMocks();
  mockListParams.length = 0;
  mockState.cards = [ACTIVE, DISABLED, PENDING, DEPLETED, EXPIRED];
  mockState.isLoading = false;
  mockState.isError = false;
});

/** Opens the long-press menu on a given row. */
function longPress(getByTestId: (id: string) => unknown, id: string) {
  fireEvent(getByTestId(`gift-card-row-${id}`) as never, "longPress");
}

/**
 * One row's mounted `GiftCardRow` element.
 *
 * The busy guard is asserted on `onLongPress` THROUGH THIS, not through
 * "firing longPress produced no sheet": `fireEvent(el, "longPress")` against
 * an element with no handler is a silent no-op, so that proxy passes both
 * when the guard holds and when the event never dispatched at all.
 */
function giftCardRow(root: Root, id: string) {
  return root
    .findAllByType(GiftCardRow)
    .find((n) => (n.props as { card: GiftCard }).card.id === id);
}

/** The rendered testIDs of the open sheet's items, in order. */
function menuKeys(getAllByTestId: (m: RegExp) => { props: { testID: string } }[]) {
  return getAllByTestId(/^action-sheet-item-/).map((n) =>
    n.props.testID.replace("action-sheet-item-", ""),
  );
}

describe("Gift cards — the collapsing header", () => {
  it("replaces BackHeader with a CollapsingHeader carrying the eyebrow and title", () => {
    const { getByTestId, getAllByText } = render(<GiftCardsScreen />);
    expect(getByTestId("collapsing-header")).toBeTruthy();
    // Both layers are always mounted and cross-faded, so the title appears
    // twice — the proof it is THIS primitive and not the old flat header.
    expect(getAllByText("Gift cards")).toHaveLength(2);
    expect(getAllByText("MARKETING")).toHaveLength(1);
  });

  it("keeps a back affordance, and it navigates back", () => {
    const { getByLabelText } = render(<GiftCardsScreen />);
    fireEvent.press(getByLabelText("Go back"));
    expect(mockBack).toHaveBeenCalledTimes(1);
  });

  it("keeps the Issue gift card action in the trailing slot", () => {
    const { getByLabelText } = render(<GiftCardsScreen />);
    fireEvent.press(getByLabelText("Issue gift card"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/gift-cards/new");
  });

  // The header only collapses if something feeds its `scrollY`, and only a
  // reanimated scroll view can. A plain `FlatList` would render every row
  // identically and leave the header frozen expanded forever — a bug no row
  // assertion can see.
  it("drives the header from an animated list, throttled at 16ms", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    const list = getByTestId("gift-cards-list");
    expect(list.props.onScroll).toEqual(expect.any(Function));
    expect(list.props.scrollEventThrottle).toBe(16);
  });

  // The chips block owns its own vertical rhythm and a hugging wrapper;
  // moving it into `ListHeaderComponent` re-introduces the ~110pt of dead
  // paper the Orders revert already paid for once.
  it("keeps the filter chips OUTSIDE the list, not in its header", () => {
    const { getByTestId, getByText } = render(<GiftCardsScreen />);
    expect(getByTestId("gift-cards-list").props.ListHeaderComponent).toBeUndefined();
    expect(getByText("Redeemed")).toBeTruthy();
  });
});

describe("Gift cards — which rows may be swiped", () => {
  it("offers Enable on the LEADING edge of a disabled card, in the accent tone", () => {
    const { UNSAFE_root } = render(<GiftCardsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-g-disabled")?.props as RowActions;
    expect(row.leadingActions).toHaveLength(1);
    expect(row.leadingActions?.[0]).toMatchObject({ key: "enable", tone: CONSTRUCTIVE_TONE });
    expect(row.trailingActions ?? []).toHaveLength(0);
  });

  it("offers Disable on the TRAILING edge of an active card, in the NEUTRAL tone", () => {
    const { UNSAFE_root } = render(<GiftCardsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-g-active")?.props as RowActions;
    expect(row.trailingActions).toHaveLength(1);
    expect(row.trailingActions?.[0]).toMatchObject({ key: "disable", tone: "neutral" });
    expect(row.leadingActions ?? []).toHaveLength(0);
  });

  // Disable is reversible, idempotent and destroys nothing — dismissive, not
  // destructive. The trailing edge is a POSITION, not a tone. Painting it
  // oxblood would tell a merchant the balance is being taken.
  it("never paints Disable as destructive", () => {
    const { UNSAFE_root } = render(<GiftCardsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-g-active")?.props as RowActions;
    expect(row.trailingActions?.[0].tone).not.toBe(DESTRUCTIVE_TONE);
  });

  it("paints Enable moss and Disable in the neutral sink", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    const enable = StyleSheet.flatten(
      getByTestId("swipe-g-disabled-action-enable").props.style,
    );
    const disable = StyleSheet.flatten(getByTestId("swipe-g-active-action-disable").props.style);
    expect(enable.backgroundColor).toBe(theme.colors.accent);
    expect(disable.backgroundColor).toBe(theme.colors.sink);
    expect(disable.backgroundColor).not.toBe(theme.colors.danger);
  });

  it.each(UNTOGGLEABLE.map((c) => [c.status === "active" ? "expired" : c.status, c.id]))(
    "mounts NO SwipeRow at all on a %s card — an armed gesture that can only 4xx is worse than none",
    (_label, id) => {
      const { UNSAFE_root, getByTestId } = render(<GiftCardsScreen />);
      expect(swipeRow(UNSAFE_root, `swipe-${id}`)).toBeUndefined();
      // …and the row itself is still there, still tappable. "No gesture" must
      // not silently become "no row".
      expect(getByTestId(`gift-card-row-${id}`)).toBeTruthy();
    },
  );

  it("mounts a SwipeRow ONLY for the two toggleable cards", () => {
    const { UNSAFE_root } = render(<GiftCardsScreen />);
    expect(swipeRows(UNSAFE_root)).toHaveLength(2);
  });

  it("obeys the app-wide side/tone convention", () => {
    const { UNSAFE_root } = render(<GiftCardsScreen />);
    assertSwipeConvention(UNSAFE_root);
  });

  it("lets nothing fire from the drag itself — this app has no undo", () => {
    const { UNSAFE_root } = render(<GiftCardsScreen />);
    assertNoAutoFire(UNSAFE_root);
  });

  it("Disable freezes the card directly — the POST needs no extra input", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    fireEvent.press(getByTestId("swipe-g-active-action-disable"));
    expect(mockSetStatus).toHaveBeenCalledTimes(1);
    expect(mockSetStatus.mock.calls[0][0]).toEqual({ id: "g-active", status: "disabled" });
  });

  it("Enable restores the card directly", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    fireEvent.press(getByTestId("swipe-g-disabled-action-enable"));
    expect(mockSetStatus).toHaveBeenCalledTimes(1);
    expect(mockSetStatus.mock.calls[0][0]).toEqual({ id: "g-disabled", status: "active" });
  });
});

describe("Gift cards — the long-press menu", () => {
  const KEYS = ["view", "enable", "disable"];

  it.each([ACTIVE.id, DISABLED.id, PENDING.id, DEPLETED.id, EXPIRED.id])(
    "shows the same three items in the same order on %s, so the sheet never resizes",
    (id) => {
      const { getByTestId, getAllByTestId } = render(<GiftCardsScreen />);
      longPress(getByTestId, id);
      expect(menuKeys(getAllByTestId as never)).toEqual(KEYS);
    },
  );

  it("greys Enable on an already-active card rather than dropping it", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    longPress(getByTestId, "g-active");
    expect(getByTestId("action-sheet-item-enable").props.accessibilityState.disabled).toBe(true);
    expect(getByTestId("action-sheet-item-disable").props.accessibilityState.disabled).toBe(
      false,
    );
    fireEvent.press(getByTestId("action-sheet-item-enable"));
    expect(mockSetStatus).not.toHaveBeenCalled();
  });

  it("greys Disable on an already-disabled card rather than dropping it", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    longPress(getByTestId, "g-disabled");
    expect(getByTestId("action-sheet-item-disable").props.accessibilityState.disabled).toBe(true);
    expect(getByTestId("action-sheet-item-enable").props.accessibilityState.disabled).toBe(false);
    fireEvent.press(getByTestId("action-sheet-item-disable"));
    expect(mockSetStatus).not.toHaveBeenCalled();
  });

  it.each(UNTOGGLEABLE.map((c) => [c.status === "active" ? "expired" : c.status, c.id]))(
    "greys BOTH toggles on a %s card, and neither fires",
    (_label, id) => {
      const { getByTestId } = render(<GiftCardsScreen />);
      longPress(getByTestId, id);
      expect(getByTestId("action-sheet-item-enable").props.accessibilityState.disabled).toBe(
        true,
      );
      expect(getByTestId("action-sheet-item-disable").props.accessibilityState.disabled).toBe(
        true,
      );
      fireEvent.press(getByTestId("action-sheet-item-enable"));
      fireEvent.press(getByTestId("action-sheet-item-disable"));
      expect(mockSetStatus).not.toHaveBeenCalled();
    },
  );

  // Delete stays CUT. The backend exposes none, and one would CASCADE the
  // transaction ledger — including rows referencing real orders. A merchant
  // reaching for it on a phone would be destroying an audit trail.
  it("offers no Delete anywhere in the menu, on any card", () => {
    const { getByTestId, getAllByTestId, queryAllByText } = render(<GiftCardsScreen />);
    for (const id of [ACTIVE.id, PENDING.id, EXPIRED.id]) {
      longPress(getByTestId, id);
      for (const key of menuKeys(getAllByTestId as never)) {
        expect(key).not.toMatch(/delete|remove|void/i);
      }
      fireEvent.press(getByTestId("action-sheet-item-view"));
    }
    expect(queryAllByText(/delete/i)).toHaveLength(0);
  });

  it("routes View to the card's detail screen", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    longPress(getByTestId, "g-active");
    fireEvent.press(getByTestId("action-sheet-item-view"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/gift-cards/g-active");
  });

  it("identifies the target card by its code in the sheet title", () => {
    const { getByTestId, getAllByText, queryAllByText } = render(<GiftCardsScreen />);
    // Counted rather than merely found: the code is already on screen in the
    // ROW, so `getAllByText(code)` passes with no sheet title at all. The
    // delta is the assertion.
    const before = queryAllByText("GIFT-••••-9999").length;
    longPress(getByTestId, "g-disabled");
    expect(getAllByText("GIFT-••••-9999")).toHaveLength(before + 1);
  });

  // ── The product promise, written down ──────────────────────────────────
  // A merchant MAY disable a card a customer has paid for and holds a
  // balance on. The safeguard is REVERSIBILITY, not restriction: disabling
  // freezes the balance and re-enabling restores it in full. No refund is
  // issued and no balance is destroyed.
  it("tells the merchant the balance is FROZEN, not taken", () => {
    const { getByTestId, getByText } = render(<GiftCardsScreen />);
    longPress(getByTestId, "g-active");
    expect(getByText(/freezes the balance/i)).toBeTruthy();
  });

  it("tells the merchant Enable gives the balance back in full", () => {
    const { getByTestId, getByText } = render(<GiftCardsScreen />);
    longPress(getByTestId, "g-disabled");
    expect(getByText(/restores the balance/i)).toBeTruthy();
  });

  it("never implies the money is gone or that the customer was refunded", () => {
    const { getByTestId, getAllByTestId } = render(<GiftCardsScreen />);
    longPress(getByTestId, "g-active");
    // Scoped to the MENU LABELS, not the whole tree: the screen legitimately
    // renders the words "Expired" (a filter chip) and "Redeemed" (another)
    // elsewhere, and a tree-wide scan would fail on those while proving
    // nothing about the copy under the merchant's thumb. `PressableRow`
    // mirrors each item's label into its accessibilityLabel, so this is the
    // string a screen reader speaks too.
    const labels = (getAllByTestId(/^action-sheet-item-/) as { props: Record<string, string> }[])
      .map((n) => n.props.accessibilityLabel)
      .join(" | ");
    for (const wrong of [/refund/i, /forfeit/i, /destroy/i, /delete/i, /remove/i, /void/i]) {
      expect(labels).not.toMatch(wrong);
    }
    // …and the two words that must be there.
    expect(labels).toMatch(/freezes the balance/i);
    expect(labels).toMatch(/restores the balance/i);
  });
});

describe("Gift cards — the busy guard and the failure surface", () => {
  it("suppresses BOTH that row's swipe and its long-press while its request is open", () => {
    const { getByTestId, UNSAFE_root } = render(<GiftCardsScreen />);
    fireEvent.press(getByTestId("swipe-g-active-action-disable"));

    expect(swipeRow(UNSAFE_root, "swipe-g-active")?.props.enabled).toBe(false);
    // `SwipeRow.enabled` does NOT reach the child row's `onLongPress` — the
    // menu is a second, independent route onto the same row.
    expect(giftCardRow(UNSAFE_root, "g-active")?.props.onLongPress).toBeUndefined();
  });

  it("leaves a NEIGHBOURING row fully live — the guard is per-id, not per-screen", () => {
    const { getByTestId, UNSAFE_root } = render(<GiftCardsScreen />);
    fireEvent.press(getByTestId("swipe-g-active-action-disable"));

    expect(swipeRow(UNSAFE_root, "swipe-g-disabled")?.props.enabled).not.toBe(false);
    expect(giftCardRow(UNSAFE_root, "g-disabled")?.props.onLongPress).toEqual(
      expect.any(Function),
    );
  });

  it("re-arms the row once the request settles", () => {
    const { getByTestId, UNSAFE_root } = render(<GiftCardsScreen />);
    fireEvent.press(getByTestId("swipe-g-active-action-disable"));
    expect(swipeRow(UNSAFE_root, "swipe-g-active")?.props.enabled).toBe(false);

    act(() => {
      mockSetStatus.mock.calls[0][1].onSuccess();
    });
    expect(swipeRow(UNSAFE_root, "swipe-g-active")?.props.enabled).not.toBe(false);
  });

  // No optimistic hide and no optimistic update: the mutation invalidates a
  // strict prefix of this list's own key, so the refetch is authoritative.
  it("does not hide or re-badge the row optimistically", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    fireEvent.press(getByTestId("swipe-g-active-action-disable"));
    expect(getByTestId("gift-card-row-g-active")).toBeTruthy();
    expect(getByTestId("gift-card-row-g-active").props.accessibilityLabel).toMatch(/active/i);
  });

  it("names the action when the server refuses, instead of a haptic alone", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    fireEvent.press(getByTestId("swipe-g-active-action-disable"));
    act(() => {
      // `name: "ApiError"` is load-bearing: `asApiError` matches the error
      // STRUCTURALLY (never `instanceof`, see action-failure-message.ts) and
      // an object without it is treated as "the device never reached the
      // server" — the merchant would be told to check their connection after
      // a perfectly successful round trip that returned 410.
      mockSetStatus.mock.calls[0][1].onError({
        name: "ApiError",
        status: 410,
        code: "gift_card_expired",
        message: "gift card has expired",
      });
    });
    expect(getByTestId("action-failure-title")).toHaveTextContent(
      /couldn't disable this gift card/i,
    );
    expect(getByTestId("action-failure-detail")).toHaveTextContent(/expired/i);
  });
});

describe("Gift cards — list states", () => {
  it("sends no status for All and the raw key for every other chip", () => {
    const { getByText } = render(<GiftCardsScreen />);
    expect(mockListParams[0]).toBeUndefined();
    fireEvent.press(getByText("Redeemed"));
    expect(mockListParams[mockListParams.length - 1]).toEqual({ status: "redeemed" });
  });

  it("still navigates to the detail screen on press", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    fireEvent.press(getByTestId("gift-card-row-g-active"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/gift-cards/g-active");
  });

  it("shows a left-aligned empty state when there are no cards", () => {
    mockState.cards = [];
    const { getByText } = render(<GiftCardsScreen />);
    expect(getByText("No gift cards yet")).toBeTruthy();
  });

  it("offers a retry when the list fails to load", () => {
    mockState.cards = [];
    mockState.isError = true;
    const { getByText } = render(<GiftCardsScreen />);
    fireEvent.press(getByText("Try again"));
    expect(mockRefetch).toHaveBeenCalledTimes(1);
  });
});
