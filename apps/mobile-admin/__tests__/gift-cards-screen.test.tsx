// Gift cards is the screen Task 1 proves the rollout kit on, and it was
// chosen precisely because it has NO backend mutations — no enable, no
// disable, no delete. So it isolates the header work (`CollapsingHeader`'s
// additive `onBack`, `useCollapsingScroll`) from any mutation risk, and the
// negative assertions below (no SwipeRow, no long-press menu) are part of the
// contract rather than an omission.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
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

import { FlatList } from "react-native";
import { fireEvent, render } from "@testing-library/react-native";
import Animated from "react-native-reanimated";
import GiftCardsScreen from "../app/(tabs)/more/marketing/gift-cards/index";
import { swipeRows } from "../test-utils/swipe-convention";
import type { GiftCard } from "@repo/mobile-shared/api/types";

// Same gap as `orders-screen.test.tsx`: jest-expo's reanimated stub exposes
// only `Animated.View`/`Animated.ScrollView`, and this screen now renders an
// `Animated.FlatList` so the CollapsingHeader can be driven from a VIRTUALIZED
// list. Patched onto the existing stub rather than re-mocking the module
// (the real source needs the native Worklets runtime and throws under jest).
(Animated as unknown as Record<string, unknown>).FlatList = FlatList;

function card(over: Partial<GiftCard> = {}): GiftCard {
  return {
    id: "g1",
    code_display: "GIFT-••••-1234",
    status: "active",
    initial_balance: 10000,
    current_balance: 7500,
    currency_code: "AUD",
    ...over,
  } as GiftCard;
}

beforeEach(() => {
  jest.clearAllMocks();
  mockListParams.length = 0;
  mockState.cards = [card(), card({ id: "g2", code_display: "GIFT-••••-9999" })];
  mockState.isLoading = false;
  mockState.isError = false;
});

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

describe("Gift cards — no mutations, so no gestures", () => {
  // The admin API has no enable, disable or delete for a gift card. An armed
  // gesture that can only fail is worse than no gesture at all, so this
  // screen deliberately gets neither a swipe nor a long-press menu. If a
  // mutation endpoint lands later, that is its own task — not a hook left
  // dangling here.
  it("mounts no SwipeRow", () => {
    const { UNSAFE_root } = render(<GiftCardsScreen />);
    expect(swipeRows(UNSAFE_root)).toHaveLength(0);
  });

  it("wires no long-press handler on any row", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    expect(getByTestId("gift-card-row-g1").props.onLongPress).toBeUndefined();
  });

  it("still navigates to the detail screen on press", () => {
    const { getByTestId } = render(<GiftCardsScreen />);
    fireEvent.press(getByTestId("gift-card-row-g1"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/gift-cards/g1");
  });
});

describe("Gift cards — list states", () => {
  it("sends no status for All and the raw key for every other chip", () => {
    const { getByText } = render(<GiftCardsScreen />);
    expect(mockListParams[0]).toBeUndefined();
    fireEvent.press(getByText("Redeemed"));
    expect(mockListParams[mockListParams.length - 1]).toEqual({ status: "redeemed" });
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
