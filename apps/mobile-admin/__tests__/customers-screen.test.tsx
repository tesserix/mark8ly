// Customers, brought onto the increment-3 native-UX kit (inc3 Task 3):
// collapsing header, and a four-item long-press menu.
//
// NO SWIPE, deliberately. The only state-changing action on this screen is
// Block, and `BlockCustomerRequest.Reason` is `binding:"required"` — so it can
// never be a fire-on-tap gesture. Unblock is direct but has no constructive
// pair worth arming a gesture for. That absence is asserted, not assumed.
//
// The FIXTURES are the part of this file that carries the real risk. The demo
// store's only customer has no name AND no phone — the shape that has already
// caused three shipped defects and that passes against almost any bug — so a
// fixture set built from "a customer" alone proves nothing. All four shapes
// are here: named+phoned, no-name/no-phone, blocked, and named-without-phone.
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
jest.mock("expo-router", () => ({ useRouter: () => ({ push: mockPush }) }));

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
    selector({ activeStore: { id: "s1", name: "Bondi Supply", currency_code: "AUD" } }),
}));

// --- controllable customer list ----------------------------------------
interface ListParams {
  status?: string;
  search?: string;
}
const mockListCalls: (ListParams | undefined)[] = [];
let mockCustomers: unknown[] = [];
let mockIsError = false;
let mockIsLoading = false;
const mockRefetch = jest.fn();

jest.mock("@/lib/hooks/use-customers", () => ({
  useCustomers: (params?: ListParams) => {
    mockListCalls.push(params);
    const rows = params?.status
      ? mockCustomers.filter((c) => (c as { status: string }).status === params.status)
      : mockCustomers;
    return {
      data: mockIsError
        ? undefined
        : {
            pages: [
              {
                data: rows,
                meta: { page: 1, page_size: 50, total: rows.length, total_pages: 1 },
              },
            ],
          },
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

const mockBlock = jest.fn();
const mockUnblock = jest.fn();
jest.mock("@/lib/admin-api/customer-actions", () => ({
  useBlockCustomer: () => ({ mutate: mockBlock, isPending: false }),
  useUnblockCustomer: () => ({ mutate: mockUnblock, isPending: false }),
}));

import { act, fireEvent, render } from "@testing-library/react-native";
import { Linking, StyleSheet } from "react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import CustomersScreen from "../app/(tabs)/customers/index";
import { CustomerRow } from "@/components/CustomerRow";
import { theme } from "@/lib/theme";
import { swipeRows, type Root } from "../test-utils/swipe-convention";
import type { Customer } from "@repo/mobile-shared/api/types";

function customer(over: Partial<Customer> = {}): Customer {
  return {
    id: "c-named",
    email: "ada@example.com",
    tags: [],
    status: "active",
    marketing_opt_in: false,
    order_count: 2,
    total_spent: 120,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  } as unknown as Customer;
}

/** A name AND a phone — the only shape on which every menu item is legal. */
const NAMED = customer({
  first_name: "Ada",
  last_name: "Lovelace",
  phone: "(02) 9999 8888",
});
/**
 * The demo store's REAL customer: email only, no name, no phone. The shape
 * `lib/customer-identity.ts` exists for, and the one that passes against
 * almost any bug — so it is the fixture the identity assertions run on.
 */
const NONAME = customer({ id: "c-noname", email: "mahesh.sangawar@gmail.com" });
/** The only shape on which Block is illegal and Unblock is legal. */
const BLOCKED = customer({
  id: "c-blocked",
  email: "grace@example.com",
  first_name: "Grace",
  last_name: "Hopper",
  phone: "+61400123456",
  status: "blocked",
});
/** Named, but no phone — proves the Call gate keys on the PHONE, not the name. */
const NOPHONE = customer({
  id: "c-nophone",
  email: "alan@example.com",
  first_name: "Alan",
  last_name: "Turing",
});

let openURL: jest.SpyInstance;

beforeEach(() => {
  jest.clearAllMocks();
  mockListCalls.length = 0;
  mockCustomers = [NAMED, NONAME, BLOCKED, NOPHONE];
  mockIsError = false;
  mockIsLoading = false;
  openURL = jest.spyOn(Linking, "openURL").mockResolvedValue(true);
});

afterEach(() => {
  openURL.mockRestore();
});

/** Opens the long-press menu on a given row. */
function longPress(getByTestId: (id: string) => unknown, id: string) {
  fireEvent(getByTestId(`customer-row-${id}`) as never, "longPress");
}

/** Every mounted `CustomerRow`, so its props can be read directly. */
function customerRows(root: Root) {
  return root.findAllByType(CustomerRow);
}

/**
 * One row's mounted `CustomerRow` element.
 *
 * The busy guard is asserted on `onLongPress` THROUGH THIS, not through
 * "firing longPress produced no sheet": `fireEvent(el, "longPress")` against
 * an element with no handler is a silent no-op, so that proxy passes both
 * when the guard holds and when the event never dispatched at all.
 */
function customerRow(root: Root, id: string) {
  return customerRows(root).find((n) => (n.props as { customer: Customer }).customer.id === id);
}

describe("Customers — header", () => {
  it("titles the screen with the editorial collapsing header", () => {
    const { getAllByText } = render(<CustomersScreen />);
    // CollapsingHeader mounts both the expanded and the collapsed layer.
    expect(getAllByText("Customers").length).toBeGreaterThan(0);
    expect(getAllByText("PEOPLE").length).toBeGreaterThan(0);
  });

  // Customers is a TAB ROOT: no `onBack`, no `leadingSlot`, therefore no nav
  // row and geometry identical to the other tab-root callers.
  it("renders no back affordance and therefore no nav row", () => {
    const { queryByTestId } = render(<CustomersScreen />);
    expect(queryByTestId("collapsing-header-nav-row")).toBeNull();
    expect(queryByTestId("collapsing-header-leading")).toBeNull();
  });

  it("drives the header from the list's own scroll offset", () => {
    const { getByTestId } = render(<CustomersScreen />);
    const list = getByTestId("customers-list");
    expect(list.props.onScroll).toBeTruthy();
    expect(list.props.scrollEventThrottle).toBe(16);
    // The search field is PINNED above the list, so the list starts at the
    // top — never parked past a hidden field (proven undiscoverable on device
    // and reverted on Orders).
    expect(list.props.contentOffset?.y ?? 0).toBe(0);
    expect(list.props.ListHeaderComponent).toBeFalsy();
  });

  it("keeps the Reviews link reachable from the header", () => {
    const { getByLabelText } = render(<CustomersScreen />);
    fireEvent.press(getByLabelText("View customer reviews"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/customers/reviews");
  });
});

describe("Customers — long-press menu", () => {
  const KEYS = ["email", "call", "block", "unblock"];

  // `snapPoints` memoises on `items.length`, so a dropped item resizes the
  // sheet under the merchant's thumb. Illegal actions are DISABLED, never
  // dropped.
  it("renders exactly four items on every customer shape", () => {
    const { getByTestId, getAllByTestId, queryAllByTestId } = render(<CustomersScreen />);

    for (const id of ["c-named", "c-noname", "c-blocked"]) {
      longPress(getByTestId, id);
      expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(4);
      for (const key of KEYS) expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();
      // Close it before the next shape — `ActionSheet` only re-presents on a
      // false→true edge.
      fireEvent.press(getByTestId("action-sheet-item-email"));
      expect(queryAllByTestId(/^action-sheet-item-/)).toHaveLength(0);
    }
  });

  it("disables Call when the customer has no phone, rather than dropping it", () => {
    const { getByTestId } = render(<CustomersScreen />);
    longPress(getByTestId, "c-noname");
    expect(getByTestId("action-sheet-item-call").props.accessibilityState.disabled).toBe(true);
    fireEvent.press(getByTestId("action-sheet-item-call"));
    expect(openURL).not.toHaveBeenCalled();
  });

  // The gate keys on the PHONE, not on "has no name" — a fixture set without
  // a named-but-phoneless customer cannot tell the two apart.
  it("disables Call for a NAMED customer who simply has no phone", () => {
    const { getByTestId } = render(<CustomersScreen />);
    longPress(getByTestId, "c-nophone");
    expect(getByTestId("action-sheet-item-call").props.accessibilityState.disabled).toBe(true);
  });

  it("opens tel: with the phone number when Call is enabled", () => {
    const { getByTestId } = render(<CustomersScreen />);
    longPress(getByTestId, "c-named");
    expect(getByTestId("action-sheet-item-call").props.accessibilityState.disabled).toBe(false);
    fireEvent.press(getByTestId("action-sheet-item-call"));
    // Spaces and punctuation stripped: `tel:(02) 9999 8888` is not a URL any
    // dialler accepts, and the merchant's own address book format is not the
    // app's to assume.
    expect(openURL).toHaveBeenCalledWith("tel:0299998888");
  });

  it("opens mailto: for every customer — email is the one field they all have", () => {
    const { getByTestId } = render(<CustomersScreen />);
    longPress(getByTestId, "c-noname");
    expect(getByTestId("action-sheet-item-email").props.accessibilityState.disabled).toBe(false);
    fireEvent.press(getByTestId("action-sheet-item-email"));
    expect(openURL).toHaveBeenCalledWith("mailto:mahesh.sangawar@gmail.com");
  });

  it("disables Block on an already-blocked customer and enables Unblock", () => {
    const { getByTestId } = render(<CustomersScreen />);
    longPress(getByTestId, "c-blocked");
    expect(getByTestId("action-sheet-item-block").props.accessibilityState.disabled).toBe(true);
    expect(getByTestId("action-sheet-item-unblock").props.accessibilityState.disabled).toBe(
      false,
    );
  });

  it("disables Unblock on a customer who is not blocked", () => {
    const { getByTestId } = render(<CustomersScreen />);
    longPress(getByTestId, "c-named");
    expect(getByTestId("action-sheet-item-unblock").props.accessibilityState.disabled).toBe(true);
    fireEvent.press(getByTestId("action-sheet-item-unblock"));
    expect(mockUnblock).not.toHaveBeenCalled();
  });

  // The action with a REQUIRED reason must never reach the API without one.
  it("opens the reason sheet for Block and does NOT call the mutation on tap", () => {
    const { getByTestId, getByLabelText } = render(<CustomersScreen />);
    longPress(getByTestId, "c-named");
    fireEvent.press(getByTestId("action-sheet-item-block"));
    expect(mockBlock).not.toHaveBeenCalled();
    // The sheet is armed and waiting for the reason.
    expect(getByTestId("block-sheet-customer")).toHaveTextContent("Ada Lovelace");
    expect(getByLabelText("Block customer").props.accessibilityState.disabled).toBe(true);
  });

  it("sends the typed reason with the block once it is submitted", () => {
    const { getByTestId, getByLabelText } = render(<CustomersScreen />);
    longPress(getByTestId, "c-named");
    fireEvent.press(getByTestId("action-sheet-item-block"));
    fireEvent.changeText(getByLabelText("Block reason"), "Repeated chargebacks");
    fireEvent.press(getByLabelText("Block customer"));
    expect(mockBlock).toHaveBeenCalledTimes(1);
    expect(mockBlock.mock.calls[0][0]).toEqual({
      id: "c-named",
      reason: "Repeated chargebacks",
    });
  });

  it("fires Unblock directly (no body, idempotent)", () => {
    const { getByTestId } = render(<CustomersScreen />);
    longPress(getByTestId, "c-blocked");
    fireEvent.press(getByTestId("action-sheet-item-unblock"));
    expect(mockUnblock).toHaveBeenCalledTimes(1);
    expect(mockUnblock.mock.calls[0][0]).toBe("c-blocked");
  });

  // Tone → paint, so a tone swap is caught at the pixel and not only at the
  // prop. Block is the one destructive item here.
  it("paints Block as the destructive item, not merely labels it", () => {
    const { getByText, getByTestId } = render(<CustomersScreen />);
    longPress(getByTestId, "c-named");
    expect(StyleSheet.flatten(getByText("Block").props.style).color).toBe(theme.colors.danger);
    // Discriminating: a sibling in the same sheet is NOT painted danger.
    expect(StyleSheet.flatten(getByText("Email").props.style)?.color).not.toBe(
      theme.colors.danger,
    );
  });

  // The one place the two identity cases are actually distinguishable: for a
  // no-name customer the title IS the email and there is no subtitle, so
  // asserting against a NAMED customer alone would pass against the exact
  // duplicate-email bug `customer-identity` was written to fix.
  it("titles the sheet with the customer identity, not a duplicated email", () => {
    const { getByTestId, getAllByText } = render(<CustomersScreen />);

    // The row alone renders the email once (no subtitle for a no-name
    // customer). Opening the menu must add exactly one more — the title.
    const rowOnly = getAllByText("mahesh.sangawar@gmail.com").length;
    longPress(getByTestId, "c-noname");
    expect(getAllByText("mahesh.sangawar@gmail.com")).toHaveLength(rowOnly + 1);

    fireEvent.press(getByTestId("action-sheet-item-email"));
    // A named customer is titled by their NAME; their email stays off the
    // sheet entirely.
    const beforeNamed = getAllByText("Ada Lovelace").length;
    longPress(getByTestId, "c-named");
    expect(getAllByText("Ada Lovelace")).toHaveLength(beforeNamed + 1);
  });
});

describe("Customers — no swipe", () => {
  /**
   * The only state-changing action here needs a REQUIRED reason, so it can
   * never fire from a gesture; the one direct action (Unblock) has no
   * constructive pair worth arming a row for. Asserted rather than assumed:
   * a later task adding a `SwipeRow` here has to justify it against this.
   */
  it("mounts no SwipeRow on any row", () => {
    const { UNSAFE_root } = render(<CustomersScreen />);
    expect(swipeRows(UNSAFE_root)).toHaveLength(0);
  });
});

describe("Customers — in-flight rows", () => {
  it("suppresses long-press while that customer's own request is open", () => {
    const { getByTestId, queryByTestId, UNSAFE_root } = render(<CustomersScreen />);
    longPress(getByTestId, "c-blocked");
    fireEvent.press(getByTestId("action-sheet-item-unblock"));

    expect(customerRow(UNSAFE_root, "c-blocked")?.props.onLongPress).toBeUndefined();
    longPress(getByTestId, "c-blocked");
    expect(queryByTestId("action-sheet-item-unblock")).toBeNull();

    // A DIFFERENT row is untouched — the guard is per row.
    expect(customerRow(UNSAFE_root, "c-named")?.props.onLongPress).toBeDefined();
    longPress(getByTestId, "c-named");
    expect(getByTestId("action-sheet-item-block")).toBeTruthy();
  });

  it("re-arms the row once the mutation settles", () => {
    const { getByTestId, UNSAFE_root } = render(<CustomersScreen />);
    longPress(getByTestId, "c-blocked");
    fireEvent.press(getByTestId("action-sheet-item-unblock"));
    act(() => (mockUnblock.mock.calls[0][1].onSuccess as () => void)());
    expect(customerRow(UNSAFE_root, "c-blocked")?.props.onLongPress).toBeDefined();
  });

  it("releases the row on failure too, so a failed action is retryable", () => {
    const { getByTestId, UNSAFE_root } = render(<CustomersScreen />);
    longPress(getByTestId, "c-blocked");
    fireEvent.press(getByTestId("action-sheet-item-unblock"));
    act(() => (mockUnblock.mock.calls[0][1].onError as () => void)());
    expect(customerRow(UNSAFE_root, "c-blocked")?.props.onLongPress).toBeDefined();
  });

  // `settleCallbacks` owns the haptics — the screen must not fire its own, or
  // every action buzzes twice.
  it("fires the success and failure haptics exactly once each", () => {
    const { getByTestId } = render(<CustomersScreen />);
    longPress(getByTestId, "c-blocked");
    fireEvent.press(getByTestId("action-sheet-item-unblock"));
    act(() => (mockUnblock.mock.calls[0][1].onSuccess as () => void)());
    expect(adminHaptics.actionSucceeded).toHaveBeenCalledTimes(1);

    longPress(getByTestId, "c-blocked");
    fireEvent.press(getByTestId("action-sheet-item-unblock"));
    act(() => (mockUnblock.mock.calls[1][1].onError as () => void)());
    expect(adminHaptics.actionFailed).toHaveBeenCalledTimes(1);
  });

  // No optimistic hide: `useBlockCustomer`/`useUnblockCustomer` invalidate
  // ["customers"], so the refetch is authoritative about this list's contents.
  it("leaves the row on screen after Unblock", () => {
    const { getByTestId } = render(<CustomersScreen />);
    longPress(getByTestId, "c-blocked");
    fireEvent.press(getByTestId("action-sheet-item-unblock"));
    expect(getByTestId("customer-row-c-blocked")).toBeTruthy();
  });
});

/**
 * The bug Orders shipped, reproduced against this screen.
 *
 * A sheet backed out of without submitting used to leave its target pinned
 * for the life of the screen. `BlockReasonSheet` is always mounted (gorhom
 * renders through a portal), so a pinned target is a LIVE, armed submit for a
 * customer the merchant explicitly walked away from.
 */
describe("Customers — the block target does not outlive its sheet", () => {
  it("leaves nothing armed after a dismiss-without-submit", () => {
    const { getByTestId, getByLabelText, queryByTestId } = render(<CustomersScreen />);
    longPress(getByTestId, "c-named");
    fireEvent.press(getByTestId("action-sheet-item-block"));
    expect(queryByTestId("block-sheet-customer")).not.toBeNull();

    fireEvent.press(getByLabelText("Cancel"));
    // The identity line is gone with the target.
    expect(queryByTestId("block-sheet-customer")).toBeNull();
  });

  it("does not block the backed-out customer if the mounted submit is driven", () => {
    const { getByTestId, getByLabelText } = render(<CustomersScreen />);
    longPress(getByTestId, "c-named");
    fireEvent.press(getByTestId("action-sheet-item-block"));
    fireEvent.press(getByLabelText("Cancel"));

    fireEvent.changeText(getByLabelText("Block reason"), "leftover");
    fireEvent.press(getByLabelText("Block customer"));
    expect(mockBlock).not.toHaveBeenCalled();
  });

  it("names B, not A, when Block is opened on a second customer", () => {
    const { getByTestId, getByLabelText } = render(<CustomersScreen />);
    longPress(getByTestId, "c-named");
    fireEvent.press(getByTestId("action-sheet-item-block"));
    fireEvent.press(getByLabelText("Cancel"));

    longPress(getByTestId, "c-nophone");
    fireEvent.press(getByTestId("action-sheet-item-block"));
    expect(getByTestId("block-sheet-customer")).toHaveTextContent("Alan Turing");

    fireEvent.changeText(getByLabelText("Block reason"), "Fraud");
    fireEvent.press(getByLabelText("Block customer"));
    expect(mockBlock.mock.calls[0][0]).toEqual({ id: "c-nophone", reason: "Fraud" });
  });
});

describe("Customers — the block error does not leak between customers", () => {
  const ERROR_COPY = "Couldn't block this customer. Try again.";

  it("shows no error on first open, despite a sticky mutation error", () => {
    const { getByTestId, queryByText } = render(<CustomersScreen />);
    longPress(getByTestId, "c-named");
    fireEvent.press(getByTestId("action-sheet-item-block"));
    expect(queryByText(ERROR_COPY)).toBeNull();
  });

  it("keeps the sheet open on failure and clears the error for the NEXT customer", () => {
    const { getByTestId, getByLabelText, getByText, queryByText } = render(<CustomersScreen />);

    longPress(getByTestId, "c-named");
    fireEvent.press(getByTestId("action-sheet-item-block"));
    fireEvent.changeText(getByLabelText("Block reason"), "Fraud");
    fireEvent.press(getByLabelText("Block customer"));
    act(() => (mockBlock.mock.calls[0][1].onError as () => void)());
    expect(getByText(ERROR_COPY)).toBeTruthy();

    fireEvent.press(getByLabelText("Cancel"));
    longPress(getByTestId, "c-nophone");
    fireEvent.press(getByTestId("action-sheet-item-block"));
    expect(queryByText(ERROR_COPY)).toBeNull();
  });
});

describe("Customers — rows", () => {
  it("opens the customer on tap", () => {
    const { getByTestId } = render(<CustomersScreen />);
    fireEvent.press(getByTestId("customer-row-c-named"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/customers/c-named");
  });

  it("tells a screen-reader user the row has more actions", () => {
    const { getByTestId } = render(<CustomersScreen />);
    expect(getByTestId("customer-row-c-named").props.accessibilityHint).toBe(
      "Long press for more actions",
    );
  });
});
