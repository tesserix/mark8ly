// Products = the catalogue screen, brought onto the increment-3 native-UX
// kit (inc3 Task 2): collapsing header, per-row status swipes, a four-item
// long-press menu, and the Archived filter chip that makes Archive
// reversible from the phone.
//
// Two judgement calls carry the real risk here and are pinned hardest:
//
//  1. WHICH gesture is legal for a given product. `PATCH /products/:id
//     {status}` is idempotent with no state machine behind it, so nothing
//     here 409s — but an armed gesture that can only no-op is still worse
//     than no gesture, and Archive is the one action a merchant cannot undo
//     with a thumb. The ARCHIVED fixture is what exposes a missing gate: a
//     draft/active-only fixture passes against broken code.
//  2. That Archive can be walked back. The long-press menu can archive a
//     product; the Archived chip is the only route back to it. They ship
//     together or not at all.
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
// this screen needs BOTH surfaces at once and the global mock carries
// neither in full: `Animated.FlatList` (the virtualized list that drives the
// CollapsingHeader) and the `FadeIn` entering animation the list wrapper has
// used since inc2. Same shape as filter-chips-rollout.test.tsx's, which
// renders this screen for the same reason. This is a test-environment gap,
// not a semantics change.
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
    // Ref-backed, like the global mock: a real shared value is stable across
    // re-renders, and a fresh literal each render would reset both the header
    // and any in-progress SwipeRow drag.
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

// --- controllable product list -----------------------------------------
// The params the hook is called with ARE the assertion for the filter chips
// — including the Archived one, whose whole job is to reach `status=archived`.
interface ListParams {
  status?: string;
  search?: string;
}
const mockListCalls: (ListParams | undefined)[] = [];
let mockProducts: unknown[] = [];
let mockIsError = false;
let mockIsLoading = false;
/**
 * Forces `meta.total` apart from the number of rows.
 *
 * Null (the default) keeps the two equal, which is what a small store looks
 * like — but ONLY that. `useProducts` pins `page_size=100` with no
 * pagination, so the real 162-product store returns `total: 162` over 100
 * rows, and a fixture that computes `total` from `rows.length` makes the two
 * definitionally equal and cannot express the case at all.
 */
let mockTotalOverride: number | null = null;
const mockRefetch = jest.fn();

jest.mock("@/lib/hooks/use-products", () => ({
  useProducts: (params?: ListParams) => {
    mockListCalls.push(params);
    const rows = params?.status
      ? mockProducts.filter((p) => (p as { status: string }).status === params.status)
      : mockProducts;
    return {
      data: mockIsError
        ? undefined
        : {
            data: rows,
            meta: {
              page: 1,
              page_size: 100,
              total: mockTotalOverride ?? rows.length,
              total_pages: 1,
            },
          },
      isLoading: mockIsLoading,
      isRefetching: false,
      isError: mockIsError,
      refetch: mockRefetch,
    };
  },
}));

const mockSetStatus = jest.fn();
jest.mock("@/lib/admin-api/product-status", () => ({
  useSetProductStatus: () => ({ mutate: mockSetStatus, isPending: false }),
}));

import { act, fireEvent, render } from "@testing-library/react-native";
import { Alert, StyleSheet } from "react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import ProductsScreen from "../app/(tabs)/products/index";
import { ProductRow } from "@/components/ProductRow";
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
import type { Product } from "@repo/mobile-shared/api/types";

function product(over: Partial<Product> = {}): Product {
  return {
    id: "p-draft",
    title: "Bondi Linen Shirt",
    handle: "bondi-linen-shirt",
    status: "draft",
    variants: [],
    media: [],
    categories: [],
    options: [],
    tags: [],
    ...over,
  } as unknown as Product;
}

const DRAFT = product();
const ACTIVE = product({ id: "p-active", title: "Coogee Tote", status: "active" });
// The fixture that earns its keep: an archived product is the one state
// where BOTH swipes and the menu's Archive are illegal, so a two-status
// fixture would pass against code that gates neither.
const ARCHIVED = product({ id: "p-archived", title: "Manly Cap", status: "archived" });

beforeEach(() => {
  jest.clearAllMocks();
  mockListCalls.length = 0;
  mockProducts = [DRAFT, ACTIVE, ARCHIVED];
  mockIsError = false;
  mockIsLoading = false;
  mockTotalOverride = null;
});

/** Opens the long-press menu on a given row. */
function longPress(getByTestId: (id: string) => unknown, id: string) {
  fireEvent(getByTestId(`product-row-${id}`) as never, "longPress");
}

/** Every mounted `ProductRow`, so its props can be read directly. */
function productRows(root: Root) {
  return root.findAllByType(ProductRow);
}

/**
 * One row's mounted `ProductRow` element.
 *
 * The busy guard is asserted on `onLongPress` THROUGH THIS, not through
 * "firing longPress produced no sheet": `fireEvent(el, "longPress")` against
 * an element with no handler is a silent no-op, so that proxy passes both
 * when the guard holds and when the event never dispatched at all.
 */
function productRow(root: Root, id: string) {
  return productRows(root).find(
    (n) => (n.props as { product: Product }).product.id === id,
  );
}

/** The destructive button the Archive confirm hands to `Alert.alert`. */
function archiveConfirmButton(spy: jest.SpyInstance) {
  const buttons = spy.mock.calls[0]?.[2] as
    | { text: string; style?: string; onPress?: () => void }[]
    | undefined;
  return buttons?.find((b) => b.style === "destructive");
}

describe("Products — header", () => {
  it("titles the screen with the editorial collapsing header", () => {
    const { getAllByText } = render(<ProductsScreen />);
    // CollapsingHeader mounts both the expanded and the collapsed layer.
    expect(getAllByText("Products").length).toBeGreaterThan(0);
    expect(getAllByText("CATALOGUE").length).toBeGreaterThan(0);
  });

  it("carries the size of the filtered scope in the right slot", () => {
    const { getByTestId } = render(<ProductsScreen />);
    expect(getByTestId("products-count")).toHaveTextContent("3 total");
  });

  it("counts the FILTERED scope, not a stale whole-catalogue number", () => {
    const { getByTestId } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("filter-chip-active-target"));
    expect(getByTestId("products-count")).toHaveTextContent("1 total");
  });

  /**
   * The count describes THE FILTER, never the rows.
   *
   * `useProducts` pins `page_size=100` with no pagination, so above 100 the
   * two genuinely differ — the real 162-product store renders 100 rows. A
   * slot reading "162 products" over them is read as a list length and is
   * wrong by 62; "162 total" is true of the scope the merchant asked for
   * whatever the page ceiling is.
   *
   * Nothing else in this file can catch that: `meta.total` is normally
   * `rows.length` by construction, which makes the claim unfalsifiable.
   */
  it("reports the size of the FILTER, not the number of rows it can show", () => {
    mockTotalOverride = 162;
    const { getByTestId, UNSAFE_root } = render(<ProductsScreen />);
    expect(productRows(UNSAFE_root)).toHaveLength(3);
    expect(getByTestId("products-count")).toHaveTextContent("162 total");
  });

  it("hides the count on an empty catalogue rather than showing a zero", () => {
    mockProducts = [];
    const { queryByTestId } = render(<ProductsScreen />);
    expect(queryByTestId("products-count")).toBeNull();
  });

  // `CollapsingHeader` sizes its container for ONE line of a `rightSlot` it
  // does not otherwise own, and `Text` does not default `numberOfLines` — so
  // this is the slot's own contract and it reddens the moment the prop goes.
  //
  // The font-scale cap that used to be asserted alongside it is NOT the
  // screen's contract: `Text` applies `MAX_FONT_SCALE` at the chokepoint and
  // `CollapsingHeader` derives its arithmetic from that same constant, so
  // deleting the (now removed) explicit prop left the assertion green. It
  // asserted a framework default.
  it("keeps the count slot to the single line the header sizes for", () => {
    const { getByTestId } = render(<ProductsScreen />);
    expect(getByTestId("products-count").props.numberOfLines).toBe(1);
  });

  // Products is a TAB ROOT: no `onBack`, no `leadingSlot`, therefore no nav
  // row and geometry identical to the two tab-root callers that shipped
  // before the back affordance existed.
  it("renders no back affordance and therefore no nav row", () => {
    const { queryByTestId } = render(<ProductsScreen />);
    expect(queryByTestId("collapsing-header-nav-row")).toBeNull();
    expect(queryByTestId("collapsing-header-leading")).toBeNull();
  });

  it("drives the header from the list's own scroll offset", () => {
    const { getByTestId } = render(<ProductsScreen />);
    const list = getByTestId("products-list");
    expect(list.props.onScroll).toBeTruthy();
    expect(list.props.scrollEventThrottle).toBe(16);
    // The search field is PINNED above the list, so the list starts at the
    // top — never parked past a hidden field (proven undiscoverable on
    // device and reverted on Orders).
    expect(list.props.contentOffset?.y ?? 0).toBe(0);
    expect(list.props.ListHeaderComponent).toBeFalsy();
  });
});

describe("Products — filters", () => {
  it("offers an Archived filter chip", () => {
    const { getByTestId } = render(<ProductsScreen />);
    expect(getByTestId("filter-chip-archived")).toBeTruthy();
  });

  it("renders the full filter set as pill chips", () => {
    const { getByTestId } = render(<ProductsScreen />);
    for (const key of ["all", "active", "draft", "archived"]) {
      expect(getByTestId(`filter-chip-${key}`)).toBeTruthy();
    }
  });

  // Without this the Archive action is one-way from the phone: the archived
  // product is absent from Active and Draft, and the merchant has no chip
  // that asks for it.
  it("asks the backend for status=archived when that chip is tapped", () => {
    const { getByTestId } = render(<ProductsScreen />);
    mockListCalls.length = 0;
    fireEvent.press(getByTestId("filter-chip-archived-target"));
    expect(mockListCalls).toContainEqual({ status: "archived" });
  });

  it("finds the archived product again under that chip", () => {
    const { getByTestId, queryByTestId } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("filter-chip-active-target"));
    expect(queryByTestId("product-row-p-archived")).toBeNull();

    fireEvent.press(getByTestId("filter-chip-archived-target"));
    expect(getByTestId("product-row-p-archived")).toBeTruthy();
  });

  it("fires the selection haptic on a filter change", () => {
    const { getByTestId } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("filter-chip-archived-target"));
    expect(adminHaptics.selectionChanged).toHaveBeenCalled();
  });
});

describe("Products — swipe", () => {
  it("offers Activate on the LEADING edge for a draft product, in the accent tone", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-p-draft")?.props as RowActions;
    expect(row.leadingActions).toHaveLength(1);
    expect(row.leadingActions?.[0]).toMatchObject({
      key: "activate",
      tone: CONSTRUCTIVE_TONE,
    });
  });

  it("offers Set to draft on the TRAILING edge for an active product, in the NEUTRAL tone", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-p-active")?.props as RowActions;
    expect(row.trailingActions).toHaveLength(1);
    expect(row.trailingActions?.[0]).toMatchObject({ key: "draft", tone: "neutral" });
  });

  // Dismissive, not destructive: reversible, idempotent, and the trailing
  // edge is a POSITION not a tone. Painting it oxblood would tell a
  // merchant's thumb that unpublishing is as final as cancelling an order.
  it("does not paint Set to draft as destructive", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-p-active")?.props as RowActions;
    expect(row.trailingActions?.[0].tone).not.toBe(DESTRUCTIVE_TONE);
  });

  it("offers no Activate on an already-active product", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-p-active")?.props as RowActions;
    expect(row.leadingActions ?? []).toHaveLength(0);
  });

  it("offers no Set to draft on a product that is already a draft", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-p-draft")?.props as RowActions;
    expect(row.trailingActions ?? []).toHaveLength(0);
  });

  // An ARCHIVED product keeps its leading Activate: re-activating is the
  // whole point of the Archived chip, and a one-thumb motion is the most
  // direct route back. It gets no trailing Draft — the menu covers
  // archived → draft, and a second gesture on a row a merchant is recovering
  // is noise.
  it("still offers Activate on an archived product — that is the way back", () => {
    const { UNSAFE_root, getByTestId } = render(<ProductsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-p-archived")?.props as RowActions;
    expect(row.leadingActions).toHaveLength(1);
    expect(row.leadingActions?.[0]).toMatchObject({
      key: "activate",
      tone: CONSTRUCTIVE_TONE,
    });
    expect(row.trailingActions ?? []).toHaveLength(0);

    fireEvent.press(getByTestId("swipe-p-archived-action-activate"));
    expect(mockSetStatus.mock.calls[0][0]).toEqual({ id: "p-archived", status: "active" });
  });

  it("holds the app-wide swipe convention on every row", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    assertSwipeConvention(UNSAFE_root);
  });

  it("never opts any action into full-swipe auto-fire", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    assertNoAutoFire(UNSAFE_root);
  });

  // Tone → paint, so a tone swap is caught at the pixel and not only at the
  // prop. Moss is this screen's ONE accent and it is spent on Activate.
  it("paints Activate moss and Set to draft in the neutral sink", () => {
    const { getByTestId } = render(<ProductsScreen />);
    const activate = StyleSheet.flatten(
      getByTestId("swipe-p-draft-action-activate").props.style,
    );
    const draft = StyleSheet.flatten(getByTestId("swipe-p-active-action-draft").props.style);
    expect(activate.backgroundColor).toBe(theme.colors.accent);
    expect(draft.backgroundColor).toBe(theme.colors.sink);
    expect(draft.backgroundColor).not.toBe(theme.colors.danger);
  });

  it("Activate sets the product active directly — the PATCH needs no extra input", () => {
    const { getByTestId } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("swipe-p-draft-action-activate"));
    expect(mockSetStatus).toHaveBeenCalledTimes(1);
    expect(mockSetStatus.mock.calls[0][0]).toEqual({ id: "p-draft", status: "active" });
  });

  it("Set to draft unpublishes the product directly", () => {
    const { getByTestId } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("swipe-p-active-action-draft"));
    expect(mockSetStatus).toHaveBeenCalledTimes(1);
    expect(mockSetStatus.mock.calls[0][0]).toEqual({ id: "p-active", status: "draft" });
  });
});

describe("Products — long-press menu", () => {
  const KEYS = ["edit", "activate", "draft", "archive"];

  // `snapPoints` memoises on `items.length`, so a dropped item resizes the
  // sheet under the merchant's thumb. Illegal actions are DISABLED, never
  // dropped. (Task 12 takes this to six — update the number here with it.)
  it("always renders four items so the sheet never resizes", () => {
    const { getByTestId, getAllByTestId, queryAllByTestId } = render(<ProductsScreen />);

    longPress(getByTestId, "p-active");
    expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(4);
    for (const key of KEYS) expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();

    // The same four on the state where THREE of them are illegal.
    fireEvent.press(getByTestId("action-sheet-item-edit"));
    expect(queryAllByTestId(/^action-sheet-item-/)).toHaveLength(0);
    longPress(getByTestId, "p-archived");
    expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(4);
    for (const key of KEYS) expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();
  });

  it("disables Activate on an already-active product rather than dropping it", () => {
    const { getByTestId } = render(<ProductsScreen />);
    longPress(getByTestId, "p-active");
    expect(getByTestId("action-sheet-item-activate").props.accessibilityState.disabled).toBe(
      true,
    );
    fireEvent.press(getByTestId("action-sheet-item-activate"));
    expect(mockSetStatus).not.toHaveBeenCalled();
  });

  it("disables Set to draft on a product that is already a draft", () => {
    const { getByTestId } = render(<ProductsScreen />);
    longPress(getByTestId, "p-draft");
    expect(getByTestId("action-sheet-item-draft").props.accessibilityState.disabled).toBe(true);
    fireEvent.press(getByTestId("action-sheet-item-draft"));
    expect(mockSetStatus).not.toHaveBeenCalled();
  });

  it("disables Archive on a product that is already archived", () => {
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const { getByTestId } = render(<ProductsScreen />);
    longPress(getByTestId, "p-archived");
    expect(getByTestId("action-sheet-item-archive").props.accessibilityState.disabled).toBe(
      true,
    );
    fireEvent.press(getByTestId("action-sheet-item-archive"));
    expect(alertSpy).not.toHaveBeenCalled();
    expect(mockSetStatus).not.toHaveBeenCalled();
    alertSpy.mockRestore();
  });

  it("re-activates an archived product from the menu", () => {
    const { getByTestId } = render(<ProductsScreen />);
    longPress(getByTestId, "p-archived");
    fireEvent.press(getByTestId("action-sheet-item-activate"));
    expect(mockSetStatus).toHaveBeenCalledTimes(1);
    expect(mockSetStatus.mock.calls[0][0]).toEqual({ id: "p-archived", status: "active" });
  });

  it("Edit opens the product detail screen", () => {
    const { getByTestId } = render(<ProductsScreen />);
    longPress(getByTestId, "p-active");
    fireEvent.press(getByTestId("action-sheet-item-edit"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/products/p-active");
  });

  // Tone → paint, modelled on the swipe suite's own paint assertion: the old
  // version of this test only asserted the row EXISTED, so deleting
  // `tone: "danger"` left Archive sitting in default ink with the test green.
  it("paints Archive as the destructive item, not merely labels it", () => {
    const { getByText, getByTestId } = render(<ProductsScreen />);
    longPress(getByTestId, "p-active");
    // Danger ink, the same treatment Orders' "Cancel order" carries.
    expect(StyleSheet.flatten(getByText("Archive").props.style).color).toBe(
      theme.colors.danger,
    );
    // And discriminating: a sibling in the same sheet is NOT painted danger,
    // so this cannot pass by every label happening to be oxblood.
    expect(StyleSheet.flatten(getByText("Edit").props.style)?.color).not.toBe(
      theme.colors.danger,
    );
  });

  it("puts Archive behind a confirm and does not fire the mutation until it is accepted", () => {
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const { getByTestId } = render(<ProductsScreen />);

    longPress(getByTestId, "p-active");
    fireEvent.press(getByTestId("action-sheet-item-archive"));
    expect(alertSpy).toHaveBeenCalledTimes(1);
    expect(mockSetStatus).not.toHaveBeenCalled();

    act(() => archiveConfirmButton(alertSpy)?.onPress?.());
    expect(mockSetStatus).toHaveBeenCalledTimes(1);
    expect(mockSetStatus.mock.calls[0][0]).toEqual({ id: "p-active", status: "archived" });
    alertSpy.mockRestore();
  });

  // Cancelling must be inert — it carries no onPress at all, so there is
  // nothing that could fire even by accident.
  it("does nothing at all when the confirm is cancelled", () => {
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const { getByTestId } = render(<ProductsScreen />);

    longPress(getByTestId, "p-active");
    fireEvent.press(getByTestId("action-sheet-item-archive"));

    const buttons = alertSpy.mock.calls[0][2] as {
      text: string;
      style?: string;
      onPress?: () => void;
    }[];
    const cancel = buttons.find((b) => b.style === "cancel");
    expect(cancel).toBeTruthy();
    expect(cancel?.onPress).toBeUndefined();
    expect(mockSetStatus).not.toHaveBeenCalled();
    alertSpy.mockRestore();
  });

  // The copy is the only place the merchant is told Archive is reversible,
  // and it names the chip that reverses it. If the chip is ever removed this
  // sentence becomes a lie, so it is asserted rather than left to review.
  it("tells the merchant where to find the product again", () => {
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const { getByTestId } = render(<ProductsScreen />);

    longPress(getByTestId, "p-active");
    fireEvent.press(getByTestId("action-sheet-item-archive"));
    expect(alertSpy.mock.calls[0][1]).toContain("Archived");
    expect(alertSpy.mock.calls[0][1]).toContain("Coogee Tote");
    alertSpy.mockRestore();
  });
});

describe("Products — no optimistic hide", () => {
  // `useSetProductStatus` invalidates ["products"], which prefix-matches
  // ["products", status, search] — the list refetches itself and is the
  // authority on its own contents. A local hide here would re-import the
  // watermark bug the Dashboard needed four rounds to fix.
  it("leaves the row on screen after Activate", () => {
    const { getByTestId } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("swipe-p-draft-action-activate"));
    expect(getByTestId("product-row-p-draft")).toBeTruthy();
  });

  it("suppresses BOTH the swipe and the long-press while that row's request is open", () => {
    const { getByTestId, queryByTestId, UNSAFE_root } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("swipe-p-draft-action-activate"));

    expect(swipeRow(UNSAFE_root, "swipe-p-draft")?.props.enabled).toBe(false);
    // The long-press half asserted DIRECTLY on the prop. This is the NEW half
    // of the guard for this screen family — `SwipeRow.enabled` does not reach
    // `onLongPress`, which is the whole reason the constraint exists — so it
    // gets the unambiguous assertion rather than the "no sheet appeared"
    // proxy (which passes whether the handler is absent or the event simply
    // never dispatched). The proxy is kept as a second line below.
    expect(productRow(UNSAFE_root, "p-draft")?.props.onLongPress).toBeUndefined();
    longPress(getByTestId, "p-draft");
    expect(queryByTestId("action-sheet-item-archive")).toBeNull();

    // A DIFFERENT row is untouched — the guard is per row.
    expect(swipeRow(UNSAFE_root, "swipe-p-active")?.props.enabled).not.toBe(false);
    expect(productRow(UNSAFE_root, "p-active")?.props.onLongPress).toBeDefined();
    longPress(getByTestId, "p-active");
    expect(getByTestId("action-sheet-item-archive")).toBeTruthy();
  });

  it("re-enables the row once the mutation settles", () => {
    const { getByTestId, UNSAFE_root } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("swipe-p-draft-action-activate"));
    act(() => (mockSetStatus.mock.calls[0][1].onSuccess as () => void)());

    expect(swipeRow(UNSAFE_root, "swipe-p-draft")?.props.enabled).not.toBe(false);
    expect(productRow(UNSAFE_root, "p-draft")?.props.onLongPress).toBeDefined();
    longPress(getByTestId, "p-draft");
    expect(getByTestId("action-sheet-item-archive")).toBeTruthy();
  });

  it("releases a row on failure too, so a failed action is retryable", () => {
    const { getByTestId, UNSAFE_root } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("swipe-p-draft-action-activate"));
    expect(swipeRow(UNSAFE_root, "swipe-p-draft")?.props.enabled).toBe(false);

    act(() => (mockSetStatus.mock.calls[0][1].onError as (e: Error) => void)(new Error("500")));
    expect(swipeRow(UNSAFE_root, "swipe-p-draft")?.props.enabled).not.toBe(false);
  });

  // `settleCallbacks` owns the haptics — the screen must not fire its own,
  // or every action buzzes twice.
  it("fires the success and failure haptics exactly once each", () => {
    const { getByTestId } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("swipe-p-draft-action-activate"));
    act(() => (mockSetStatus.mock.calls[0][1].onSuccess as () => void)());
    expect(adminHaptics.actionSucceeded).toHaveBeenCalledTimes(1);

    fireEvent.press(getByTestId("swipe-p-draft-action-activate"));
    act(() => (mockSetStatus.mock.calls[1][1].onError as (e: Error) => void)(new Error("x")));
    expect(adminHaptics.actionFailed).toHaveBeenCalledTimes(1);
  });

  it("guards two in-flight rows at once", () => {
    const { getByTestId, UNSAFE_root } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("swipe-p-draft-action-activate"));
    fireEvent.press(getByTestId("swipe-p-active-action-draft"));

    expect(mockSetStatus).toHaveBeenCalledTimes(2);
    expect(swipeRow(UNSAFE_root, "swipe-p-draft")?.props.enabled).toBe(false);
    expect(swipeRow(UNSAFE_root, "swipe-p-active")?.props.enabled).toBe(false);

    act(() => (mockSetStatus.mock.calls[0][1].onSuccess as () => void)());
    expect(swipeRow(UNSAFE_root, "swipe-p-draft")?.props.enabled).not.toBe(false);
    expect(swipeRow(UNSAFE_root, "swipe-p-active")?.props.enabled).toBe(false);
  });
});

// The Archived chip is routinely empty, which is what exposed this: the
// screen told a merchant with 162 products to "add your first product"
// whenever a FILTER came back empty. Active and Draft had the same lie.
describe("Products — empty states", () => {
  it("offers to add a first product only on a genuinely empty catalogue", () => {
    mockProducts = [];
    const { getByText } = render(<ProductsScreen />);
    expect(getByText("No products yet")).toBeTruthy();
    expect(getByText("Add your first product to get started.")).toBeTruthy();
  });

  it("does not claim an empty catalogue when a FILTER is simply empty", () => {
    mockProducts = [DRAFT, ACTIVE];
    const { getByTestId, getByText, queryByText } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("filter-chip-archived-target"));

    expect(queryByText("Add your first product to get started.")).toBeNull();
    expect(getByText("No archived products right now.")).toBeTruthy();
  });

  // The search text is DEBOUNCED (300ms) before it reaches either the query
  // or this copy, so this waits rather than asserting synchronously.
  it("points at the search term when the search is what came back empty", async () => {
    mockProducts = [];
    const { getByLabelText, findByText } = render(<ProductsScreen />);
    fireEvent.changeText(getByLabelText("Search products"), "zzz");
    expect(await findByText("Try a different search term.")).toBeTruthy();
  });
});

describe("Products — rows", () => {
  it("opens the product on tap", () => {
    const { getByTestId } = render(<ProductsScreen />);
    fireEvent.press(getByTestId("product-row-p-active"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/products/p-active");
  });

  /**
   * The badge and the announcement are ONE string.
   *
   * They were computed separately: the badge titleised, the row's
   * `accessibilityLabel` interpolated the raw wire value, so VoiceOver said
   * "archived" beside a chip reading "Archived" — and an unexpected server
   * status would have been announced (and painted) verbatim as
   * "out_of_stock". The `-badge` testID exists to assert this pairing
   * without matching the row's whole label string.
   */
  it("announces exactly the status its badge paints", () => {
    const { getByTestId } = render(<ProductsScreen />);
    expect(getByTestId("product-row-p-archived-badge").props.accessibilityLabel).toBe(
      "Status: Archived",
    );

    const rowLabel = getByTestId("product-row-p-archived").props.accessibilityLabel as string;
    expect(rowLabel).toContain("Archived");
    expect(rowLabel).not.toContain("archived");
  });

  it("keeps the FAB reachable for adding a product", () => {
    const { getByLabelText } = render(<ProductsScreen />);
    fireEvent.press(getByLabelText("Add new product"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/products/new");
  });
});
