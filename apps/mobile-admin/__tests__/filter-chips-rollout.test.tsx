// The pill-chip rollout (inc2 §3). Seven list screens moved off the
// underlined `SegmentedControl` and onto the 40pt `FilterChips` strip Orders
// already uses.
//
// This is a PRESENTATION swap, and the risk it carries is entirely in the
// half that is not presentation: each screen's filter set maps to its own
// backend query, and the sets are not uniform. So every screen below is
// asserted twice — once that it renders a pill per filter (the control
// actually changed), and once that tapping a pill reaches that screen's hook
// with the SAME params the old segmented control produced (the semantics did
// not).
//
// `SegmentedControl` deliberately survives: products/new.tsx uses it as a
// form field for product Status inside a Card, which is not a list filter.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("expo-image", () => {
  const { View } = require("react-native");
  return { Image: View };
});

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

// Local mock (it wins over `__mocks__/react-native-reanimated.js`) because
// these screens need the entrance-animation surface the global mock has no
// reason to carry. Grown in Task 1 for the scroll-driven `CollapsingHeader`:
// Gift cards is the first screen in this file to render one, so the mock now
// also needs `Animated.FlatList` plus the shared-value/scroll-handler/
// interpolation pieces the header reads. The ASSERTIONS below are untouched —
// this is a test-environment gap, not a semantics change.
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
    Extrapolation: { CLAMP: "clamp" },
    interpolate,
    useAnimatedStyle: (factory: () => unknown) => factory(),
    useDerivedValue: (factory: () => number) => ({ value: factory() }),
    useAnimatedScrollHandler: (handler: unknown) => handler,
    // Ref-backed, like the global mock: a real shared value is stable across
    // re-renders, and a fresh literal each render would reset the header.
    useSharedValue: (initial: number) => {
      // No type argument: `useRef` here comes from an untyped `require`.
      const ref = useRef(undefined) as { current: { value: number } | undefined };
      if (ref.current === undefined) ref.current = { value: initial };
      return ref.current;
    },
    useReducedMotion: jest.fn(() => false),
  };
});

jest.mock("expo-router", () => ({ useRouter: () => ({ push: jest.fn() }) }));

jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: {
    selectionChanged: jest.fn(() => Promise.resolve()),
    actionSucceeded: jest.fn(() => Promise.resolve()),
    actionFailed: jest.fn(() => Promise.resolve()),
  },
}));

jest.mock("@repo/mobile-shared/stores/tenant-store", () => ({
  useTenantStore: (selector: (s: unknown) => unknown) =>
    selector({ activeStore: { id: "s1", name: "Bondi Supply", currency_code: "AUD" } }),
}));

// Every list hook below is recorded rather than stubbed blind: the params it
// was called with ARE the assertion.
type Params = Record<string, unknown> | undefined;
const mockCalls: Record<string, Params[]> = {};

function mockRecord(name: string, params: Params) {
  (mockCalls[name] ??= []).push(params);
}

/** Shape every paginated list hook on these screens returns. */
function mockPagedResult() {
  return {
    data: { pages: [{ data: [], meta: { page: 1, total: 0, total_pages: 1 } }] },
    isLoading: false,
    isRefetching: false,
    isError: false,
    refetch: jest.fn(),
    fetchNextPage: jest.fn(),
    hasNextPage: false,
    isFetchingNextPage: false,
  };
}

jest.mock("@/lib/hooks/use-products", () => ({
  useProducts: (params?: Record<string, unknown>) => {
    mockRecord("products", params);
    // Products is the one non-paginated list here.
    return { data: { data: [] }, isLoading: false, isRefetching: false, isError: false, refetch: jest.fn() };
  },
}));
// Products' status swipes/menu (inc3 Task 2) reach a real react-query
// mutation, which pulls in the api client and, through it, the auth
// provider — a module chain this suite has no reason to boot.
jest.mock("@/lib/admin-api/product-status", () => ({
  useSetProductStatus: () => ({ mutate: jest.fn(), isPending: false }),
}));
jest.mock("@/lib/hooks/use-customers", () => ({
  useCustomers: (params?: Record<string, unknown>) => (mockRecord("customers", params), mockPagedResult()),
}));
// Customers' long-press menu (inc3 Task 3) reaches real react-query
// mutations, which pull in the api client and, through it, the auth provider
// — the same module chain Products' status mutation had to be stubbed out of
// two lines above.
jest.mock("@/lib/admin-api/customer-actions", () => ({
  useBlockCustomer: () => ({ mutate: jest.fn(), isPending: false }),
  useUnblockCustomer: () => ({ mutate: jest.fn(), isPending: false }),
}));
jest.mock("@/lib/hooks/use-reviews", () => ({
  useReviews: (params?: Record<string, unknown>) => (mockRecord("reviews", params), mockPagedResult()),
}));
jest.mock("@/lib/hooks/use-tickets", () => ({
  useTickets: (params?: Record<string, unknown>) => (mockRecord("tickets", params), mockPagedResult()),
}));
jest.mock("@/lib/hooks/use-gift-cards", () => ({
  useGiftCards: (params?: Record<string, unknown>) => (mockRecord("giftCards", params), mockPagedResult()),
}));
jest.mock("@/lib/hooks/use-campaigns", () => ({
  useCampaigns: (params?: Record<string, unknown>) => (mockRecord("campaigns", params), mockPagedResult()),
}));
jest.mock("@/lib/hooks/use-coupons", () => ({
  useCoupons: (params?: Record<string, unknown>) => (mockRecord("coupons", params), mockPagedResult()),
}));

import type { ComponentType } from "react";
import { render, fireEvent } from "@testing-library/react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import ProductsScreen from "../app/(tabs)/products/index";
import CustomersScreen from "../app/(tabs)/customers/index";
import ReviewsScreen from "../app/(tabs)/customers/reviews/index";
import TicketsScreen from "../app/(tabs)/more/settings/tickets/index";
import GiftCardsScreen from "../app/(tabs)/more/marketing/gift-cards/index";
import CampaignsScreen from "../app/(tabs)/more/marketing/campaigns/index";
import CouponsScreen from "../app/(tabs)/more/marketing/coupons/index";

beforeEach(() => {
  jest.clearAllMocks();
  for (const key of Object.keys(mockCalls)) delete mockCalls[key];
});

interface Case {
  name: string;
  Screen: ComponentType;
  hook: string;
  /** Every chip key the screen must render, in order. */
  keys: string[];
  /** The key tapped, and the params its hook must then receive. */
  tap: string;
  expected: Params;
  /** The params the hook receives at rest, on the default "all" filter. */
  atRest: Params;
}

// One row per converted screen. `expected`/`atRest` are lifted from what the
// screen's own code passes today — they are the "semantics preserved" proof,
// not a redesign of any screen's query.
const CASES: Case[] = [
  {
    name: "Products",
    Screen: ProductsScreen,
    hook: "products",
    // "Inactive" is deliberately absent: the backend enum is
    // draft|active|archived and status=inactive was a hard 400. "Archived"
    // joined in inc3 Task 2 — it is the only route back to a product the
    // long-press menu archived.
    keys: ["all", "active", "draft", "archived"],
    tap: "draft",
    expected: { status: "draft" },
    // Products collapses an empty param bag to `undefined` rather than `{}`.
    atRest: undefined,
  },
  {
    name: "Customers",
    Screen: CustomersScreen,
    hook: "customers",
    keys: ["all", "active", "blocked"],
    tap: "blocked",
    expected: { status: "blocked" },
    // Customers always spreads into an object, empty on "all".
    atRest: {},
  },
  {
    name: "Reviews",
    Screen: ReviewsScreen,
    hook: "reviews",
    keys: ["all", "pending", "approved", "rejected"],
    tap: "approved",
    expected: { status: "approved" },
    atRest: undefined,
  },
  {
    name: "Tickets",
    Screen: TicketsScreen,
    hook: "tickets",
    keys: ["all", "open", "pending", "resolved", "closed"],
    tap: "resolved",
    expected: { status: "resolved" },
    atRest: undefined,
  },
  {
    name: "Gift cards",
    Screen: GiftCardsScreen,
    hook: "giftCards",
    // Label is "Off", key is `disabled` — the key is what reaches the query.
    keys: ["all", "active", "redeemed", "expired", "disabled"],
    tap: "disabled",
    expected: { status: "disabled" },
    atRest: undefined,
  },
  {
    name: "Campaigns",
    Screen: CampaignsScreen,
    hook: "campaigns",
    keys: ["all", "draft", "scheduled", "sent", "paused"],
    tap: "scheduled",
    expected: { status: "scheduled" },
    atRest: undefined,
  },
  {
    name: "Coupons",
    Screen: CouponsScreen,
    hook: "coupons",
    keys: ["all", "active", "scheduled", "expired", "disabled"],
    tap: "expired",
    expected: { status: "expired" },
    atRest: undefined,
  },
];

describe.each(CASES)("$name — pill chips", ({ Screen, hook, keys, tap, expected, atRest }) => {
  it("renders one pill per filter, and no underlined segmented tab", () => {
    const { getByTestId } = render(<Screen />);
    for (const key of keys) expect(getByTestId(`filter-chip-${key}`)).toBeTruthy();
    // The pill lives inside a real ≥44pt press box, not a hitSlop region.
    expect(getByTestId(`filter-chip-${tap}-target`)).toBeTruthy();
  });

  it("asks its hook for nothing extra while the default filter is active", () => {
    render(<Screen />);
    expect(mockCalls[hook]?.[0]).toEqual(atRest);
  });

  it("carries the tapped filter through to its own query unchanged", () => {
    const { getByTestId } = render(<Screen />);
    fireEvent.press(getByTestId(`filter-chip-${tap}-target`));
    expect(mockCalls[hook]?.at(-1)).toEqual(expected);
  });

  it("fires the selection haptic on a filter change", () => {
    const { getByTestId } = render(<Screen />);
    fireEvent.press(getByTestId(`filter-chip-${tap}-target`));
    expect(adminHaptics.selectionChanged).toHaveBeenCalled();
  });
});
