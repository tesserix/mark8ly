// Products — Edit price / Adjust stock from the long-press menu (inc3 Task 12).
//
// This is the first place in the app where a merchant types a NUMBER at a
// row, and the first that has to resolve a product to one of its VARIANTS
// before it can send anything: `PATCH /products/:id/variants/:variantId` is
// per-variant, and 8 of the store's 12 active products have 2-5 variants.
//
// The three fixtures below are the whole point of this file:
//
//   ZERO   — no variants at all. The only fixture that can prove the two new
//            menu items are DISABLED rather than dropped (dropping them would
//            resize the sheet under the merchant's thumb) and that neither
//            opens anything.
//   SINGLE — one variant. The common case, which must not cost an extra tap:
//            no picker, straight to the numeric sheet.
//   MULTI  — three variants, deliberately out of position order. A
//            single-variant-only fixture passes against a completely absent
//            picker, so it cannot be the only one.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("expo-image", () => {
  const { View } = require("react-native");
  return { Image: View };
});

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

// Same local reanimated mock products-screen.test.tsx carries, and for the
// same reason: this screen needs BOTH `Animated.FlatList` and the `FadeIn`
// entering animation, and the global mock carries neither in full.
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
  return {
    __esModule: true,
    default: { View, FlatList },
    FadeIn: new ChainableAnimation(),
    Easing: { bezier: () => (t: number) => t },
    Extrapolation: { CLAMP: "clamp", EXTEND: "extend", IDENTITY: "identity" },
    interpolate: (value: number) => value,
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

let mockProducts: unknown[] = [];
jest.mock("@/lib/hooks/use-products", () => ({
  useProducts: () => ({
    data: {
      data: mockProducts,
      meta: { page: 1, page_size: 100, total: mockProducts.length, total_pages: 1 },
    },
    isLoading: false,
    isRefetching: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("@/lib/admin-api/product-status", () => ({
  useSetProductStatus: () => ({ mutate: jest.fn(), isPending: false }),
}));

const mockQuickEdit = jest.fn();
jest.mock("@/lib/admin-api/variant-quick-edit", () => ({
  useQuickEditVariant: () => ({ mutate: mockQuickEdit, isPending: false }),
}));

import { act, fireEvent, render } from "@testing-library/react-native";
import ProductsScreen from "../app/(tabs)/products/index";
import { ProductRow } from "@/components/ProductRow";
import { actionSheetHeight } from "@/components/ui/ActionSheet";
import { MAX_FONT_SCALE } from "@/components/ui/Text";
import { sortVariants, variantLabel } from "@/components/products/variant-identity";
import { validateVariantValue } from "@/components/products/VariantValueSheet";
import { theme } from "@/lib/theme";
import type { Product, ProductVariant } from "@repo/mobile-shared/api/types";

function variant(over: Partial<ProductVariant> = {}): ProductVariant {
  return {
    id: "v-1",
    sku: "BONDI-1",
    price: 49.5,
    currency_code: "AUD",
    inventory_quantity: 12,
    inventory_policy: "deny",
    option_values: [],
    position: 0,
    ...over,
  } as ProductVariant;
}

function product(over: Partial<Product> = {}): Product {
  return {
    id: "p-zero",
    title: "Bondi Linen Shirt",
    handle: "bondi-linen-shirt",
    status: "active",
    variants: [],
    media: [],
    categories: [],
    options: [],
    tags: [],
    ...over,
  } as unknown as Product;
}

/** No variants — the endpoint has nothing to address. */
const ZERO = product();

/** One variant: no picker, straight to the numeric sheet. */
const SINGLE = product({
  id: "p-single",
  title: "Coogee Tote",
  variants: [variant({ id: "v-single", sku: "COOGEE-TOTE", price: 89, inventory_quantity: 4 })],
});

/**
 * Three variants, returned 2,0,1 — the wire genuinely does this (a real
 * product came back 2,3,4,0,1), so a picker that trusts array order lists
 * them in a different sequence from every other surface in the app.
 */
const MULTI = product({
  id: "p-multi",
  title: "Manly Cap",
  variants: [
    variant({
      id: "v-blue-l",
      sku: "CAP-BL-L",
      price: 30,
      inventory_quantity: 2,
      position: 2,
      option_values: [
        { option_name: "Colour", option_value_id: "ov1", value: "Blue" },
        { option_name: "Size", option_value_id: "ov2", value: "L" },
      ],
    }),
    variant({
      id: "v-blue-s",
      sku: "CAP-BL-S",
      price: 25,
      inventory_quantity: 9,
      position: 0,
      option_values: [
        { option_name: "Colour", option_value_id: "ov1", value: "Blue" },
        { option_name: "Size", option_value_id: "ov3", value: "S" },
      ],
    }),
    variant({
      id: "v-sand-m",
      sku: "CAP-SD-M",
      price: 27.5,
      inventory_quantity: 0,
      position: 1,
      option_values: [
        { option_name: "Colour", option_value_id: "ov4", value: "Sand" },
        { option_name: "Size", option_value_id: "ov5", value: "M" },
      ],
    }),
  ],
});

beforeEach(() => {
  jest.clearAllMocks();
  mockProducts = [ZERO, SINGLE, MULTI];
});

function longPress(getByTestId: (id: string) => unknown, id: string) {
  fireEvent(getByTestId(`product-row-${id}`) as never, "longPress");
}

/** Long-press a row and take the named action off the menu. */
function menuAction(root: ReturnType<typeof render>, id: string, key: string) {
  longPress(root.getByTestId, id);
  fireEvent.press(root.getByTestId(`action-sheet-item-${key}`));
}

function productRow(root: ReturnType<typeof render>, id: string) {
  return root.UNSAFE_root
    .findAllByType(ProductRow)
    .find((n) => (n.props as { product: Product }).product.id === id);
}

// --- the pure pieces ----------------------------------------------------

describe("variant identity", () => {
  it("sorts by position, never by array order", () => {
    expect(sortVariants(MULTI.variants).map((v) => v.id)).toEqual([
      "v-blue-s",
      "v-sand-m",
      "v-blue-l",
    ]);
  });

  it("does not mutate the array it was given", () => {
    const original = MULTI.variants.map((v) => v.id);
    sortVariants(MULTI.variants);
    expect(MULTI.variants.map((v) => v.id)).toEqual(original);
  });

  // A picker whose three rows all read "Default" is useless — it is exactly
  // the failure this naming exists to prevent.
  it("names a variant by its option values and its SKU", () => {
    expect(variantLabel(MULTI.variants[0]!)).toBe("Blue / L · CAP-BL-L");
  });

  it("falls back to the SKU when a product has no options", () => {
    expect(variantLabel(SINGLE.variants[0]!)).toBe("COOGEE-TOTE");
  });

  it("still says something distinguishing when there is neither", () => {
    expect(variantLabel(variant({ sku: "  ", option_values: [], position: 2 }))).toBe(
      "Variant 3",
    );
  });
});

describe("validateVariantValue", () => {
  it("accepts a decimal price and a whole quantity", () => {
    expect(validateVariantValue("price", "24.50")).toEqual({ value: 24.5, error: null });
    expect(validateVariantValue("inventory_quantity", "7")).toEqual({ value: 7, error: null });
  });

  it("accepts zero for both", () => {
    expect(validateVariantValue("price", "0").value).toBe(0);
    expect(validateVariantValue("inventory_quantity", "0").value).toBe(0);
  });

  // The client already knows these are wrong. A round trip to be told so is a
  // failure of the client, not a validation strategy.
  it("refuses a negative price and a negative quantity", () => {
    expect(validateVariantValue("price", "-1").value).toBeNull();
    expect(validateVariantValue("price", "-1").error).toMatch(/negative/i);
    expect(validateVariantValue("inventory_quantity", "-3").value).toBeNull();
    expect(validateVariantValue("inventory_quantity", "-3").error).toMatch(/negative/i);
  });

  it("refuses a fractional stock count", () => {
    expect(validateVariantValue("inventory_quantity", "2.5").value).toBeNull();
    expect(validateVariantValue("inventory_quantity", "2.5").error).toMatch(/whole/i);
  });

  // A currency symbol or a thousands separator must never reach the wire as
  // part of a decimal — the field is a shopspring decimal, not a display
  // string.
  it("refuses a formatted amount", () => {
    expect(validateVariantValue("price", "$49.00").value).toBeNull();
    expect(validateVariantValue("price", "1,200.00").value).toBeNull();
  });

  // A decimal-pad in a comma-decimal locale emits "24,50" and means 24.5.
  it("reads a single decimal comma as a decimal point", () => {
    expect(validateVariantValue("price", "24,50").value).toBe(24.5);
  });

  it("says nothing at all about an empty field", () => {
    expect(validateVariantValue("price", "")).toEqual({ value: null, error: null });
    expect(validateVariantValue("price", "   ")).toEqual({ value: null, error: null });
  });
});

// --- the menu -----------------------------------------------------------

describe("Products menu — six items, always", () => {
  const KEYS = ["edit", "price", "stock", "activate", "draft", "archive"];

  // `snapPoints` memoises on `items.length`. A menu that has six items on a
  // multi-variant product and four on a variant-less one resizes under the
  // merchant's thumb between two rows of the same list.
  it("renders the same six items whatever the product's variants", () => {
    const root = render(<ProductsScreen />);
    for (const id of ["p-zero", "p-single", "p-multi"]) {
      longPress(root.getByTestId, id);
      expect(root.getAllByTestId(/^action-sheet-item-/)).toHaveLength(6);
      for (const key of KEYS) expect(root.getByTestId(`action-sheet-item-${key}`)).toBeTruthy();
      // Close it before the next row, or the assertion above counts two menus.
      fireEvent.press(root.getByTestId("action-sheet-item-edit"));
    }
  });

  it("disables both new items on a product with no variants rather than dropping them", () => {
    const root = render(<ProductsScreen />);
    longPress(root.getByTestId, "p-zero");
    expect(root.getByTestId("action-sheet-item-price").props.accessibilityState.disabled).toBe(
      true,
    );
    expect(root.getByTestId("action-sheet-item-stock").props.accessibilityState.disabled).toBe(
      true,
    );
  });

  it("opens nothing when a disabled item is pressed", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-zero", "price");
    expect(root.queryByTestId("variant-value-input")).toBeNull();
    expect(root.queryAllByTestId(/^action-sheet-item-variant-/)).toHaveLength(0);
    expect(mockQuickEdit).not.toHaveBeenCalled();
  });

  it("leaves both items live on a product that has variants", () => {
    const root = render(<ProductsScreen />);
    longPress(root.getByTestId, "p-single");
    expect(
      root.getByTestId("action-sheet-item-price").props.accessibilityState.disabled,
    ).toBeFalsy();
    expect(
      root.getByTestId("action-sheet-item-stock").props.accessibilityState.disabled,
    ).toBeFalsy();
  });

  /**
   * The six-item menu at accessibility text sizes.
   *
   * Task 2's four-item menu was ~64pt short at AX sizes and its last row fell
   * below the fold; the budget now scales with Dynamic Type. Six items is the
   * tallest menu in the app, so it is the one that would hit the notch clamp
   * first — and past the clamp the last rows are reachable only by scrolling
   * inside a sheet that gives no hint it scrolls.
   */
  it("budgets all six rows at the largest text size the app will draw", () => {
    const geometry = {
      hasTitle: true,
      bottomPadding: theme.spacing.xl,
      windowHeight: 852,
      topInset: 59,
    };
    const rowAtMax = Math.max(
      theme.row.minHeightSingle,
      (theme.text.body.lineHeight ?? 24) * MAX_FONT_SCALE + theme.row.paddingV * 2,
    );
    const budget = actionSheetHeight({ ...geometry, itemCount: 6, fontScale: MAX_FONT_SCALE });
    const clamp = geometry.windowHeight - geometry.topInset - theme.spacing.xxl;

    // Unclamped: every row is inside the sheet's own height, not past it.
    expect(budget).toBeLessThanOrEqual(clamp);
    expect(budget).toBeGreaterThanOrEqual(6 * rowAtMax);
    // And it really did grow with the two new items.
    expect(budget).toBeGreaterThan(
      actionSheetHeight({ ...geometry, itemCount: 4, fontScale: MAX_FONT_SCALE }),
    );
  });
});

// --- variant resolution -------------------------------------------------

describe("Products — variant resolution", () => {
  it("skips the picker entirely for a single-variant product", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-single", "price");

    expect(root.queryAllByTestId(/^action-sheet-item-variant-/)).toHaveLength(0);
    expect(root.getByTestId("variant-value-input").props.value).toBe("89");
  });

  it("asks which variant first on a multi-variant product", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-multi", "stock");

    // The picker, and NOT the numeric sheet — the merchant has not yet said
    // which of the three they mean.
    expect(root.queryAllByTestId(/^action-sheet-item-variant-/)).toHaveLength(3);
    expect(root.queryByTestId("variant-value-input")).toBeNull();
  });

  it("names each variant in the picker, in position order", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-multi", "stock");

    const labels = root
      .queryAllByTestId(/^action-sheet-item-variant-/)
      .map((node) => node.props.accessibilityLabel);
    expect(labels).toEqual([
      "Blue / S · CAP-BL-S",
      "Sand / M · CAP-SD-M",
      "Blue / L · CAP-BL-L",
    ]);
  });

  it("opens the numeric sheet on the variant that was chosen", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-multi", "stock");
    fireEvent.press(root.getByTestId("action-sheet-item-variant-v-sand-m"));

    // Sand / M's own quantity — 0, the value most easily lost to a
    // truthiness check on the way to the field.
    expect(root.getByTestId("variant-value-input").props.value).toBe("0");
    expect(root.getByTestId("variant-value-context")).toHaveTextContent(/Sand \/ M/);
  });
});

// --- the numeric sheet --------------------------------------------------

describe("Products — the numeric sheet", () => {
  function openPrice(root: ReturnType<typeof render>) {
    menuAction(root, "p-single", "price");
  }

  it("pre-fills the current value and selects it so it can be overtyped", () => {
    const root = render(<ProductsScreen />);
    openPrice(root);
    const input = root.getByTestId("variant-value-input");
    expect(input.props.value).toBe("89");
    expect(input.props.selectTextOnFocus).toBe(true);
    expect(input.props.autoFocus).toBe(true);
  });

  it("uses the keypad that matches the field", () => {
    const root = render(<ProductsScreen />);
    openPrice(root);
    expect(root.getByTestId("variant-value-input").props.keyboardType).toBe("decimal-pad");

    fireEvent.press(root.getByTestId("variant-value-dismiss"));
    menuAction(root, "p-single", "stock");
    expect(root.getByTestId("variant-value-input").props.keyboardType).toBe("number-pad");
  });

  it("refuses to submit an unchanged value", () => {
    const root = render(<ProductsScreen />);
    openPrice(root);
    expect(root.getByTestId("variant-value-submit").props.accessibilityState.disabled).toBe(true);
  });

  it("refuses to submit an empty field", () => {
    const root = render(<ProductsScreen />);
    openPrice(root);
    fireEvent.changeText(root.getByTestId("variant-value-input"), "");
    expect(root.getByTestId("variant-value-submit").props.accessibilityState.disabled).toBe(true);
  });

  it("arms submit once the value actually changes", () => {
    const root = render(<ProductsScreen />);
    openPrice(root);
    fireEvent.changeText(root.getByTestId("variant-value-input"), "94.25");
    expect(
      root.getByTestId("variant-value-submit").props.accessibilityState.disabled,
    ).toBeFalsy();
  });

  // Refused IN THE HAND, with no request. The server would refuse it too, but
  // a round trip to learn what the client already knew is a failure of the
  // client.
  it("rejects a negative price inline and sends nothing", () => {
    const root = render(<ProductsScreen />);
    openPrice(root);
    fireEvent.changeText(root.getByTestId("variant-value-input"), "-5");

    expect(root.getByTestId("variant-value-error")).toHaveTextContent(/negative/i);
    fireEvent.press(root.getByTestId("variant-value-submit"));
    expect(mockQuickEdit).not.toHaveBeenCalled();
    // And the sheet stays open on the value the merchant typed, so it can be
    // corrected rather than retyped from scratch.
    expect(root.getByTestId("variant-value-input").props.value).toBe("-5");
  });

  it("rejects a negative stock count inline and sends nothing", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-single", "stock");
    fireEvent.changeText(root.getByTestId("variant-value-input"), "-2");

    expect(root.getByTestId("variant-value-error")).toHaveTextContent(/negative/i);
    fireEvent.press(root.getByTestId("variant-value-submit"));
    expect(mockQuickEdit).not.toHaveBeenCalled();
  });

  it("sends ONLY the field the merchant changed", () => {
    const root = render(<ProductsScreen />);
    openPrice(root);
    fireEvent.changeText(root.getByTestId("variant-value-input"), "94.25");
    fireEvent.press(root.getByTestId("variant-value-submit"));

    expect(mockQuickEdit).toHaveBeenCalledTimes(1);
    expect(mockQuickEdit.mock.calls[0]![0]).toEqual({
      productId: "p-single",
      variantId: "v-single",
      field: "price",
      value: 94.25,
    });
  });

  it("sends the stock quantity as a number, on the chosen variant", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-multi", "stock");
    fireEvent.press(root.getByTestId("action-sheet-item-variant-v-blue-l"));
    fireEvent.changeText(root.getByTestId("variant-value-input"), "18");
    fireEvent.press(root.getByTestId("variant-value-submit"));

    expect(mockQuickEdit.mock.calls[0]![0]).toEqual({
      productId: "p-multi",
      variantId: "v-blue-l",
      field: "inventory_quantity",
      value: 18,
    });
  });

  it("closes itself once the edit lands", () => {
    const root = render(<ProductsScreen />);
    openPrice(root);
    fireEvent.changeText(root.getByTestId("variant-value-input"), "94.25");
    fireEvent.press(root.getByTestId("variant-value-submit"));
    act(() => (mockQuickEdit.mock.calls[0]![1].onSuccess as () => void)());

    expect(root.queryByTestId("variant-value-input")).toBeNull();
  });

  // No optimistic update: the row still reads the old price until the
  // invalidated ["products"] query refetches. Asserting the row DIDN'T move
  // is what stops someone adding a local overlay and re-importing the
  // Dashboard's watermark bug.
  it("does not repaint the row itself — the refetch is the authority", () => {
    const root = render(<ProductsScreen />);
    openPrice(root);
    fireEvent.changeText(root.getByTestId("variant-value-input"), "94.25");
    fireEvent.press(root.getByTestId("variant-value-submit"));
    act(() => (mockQuickEdit.mock.calls[0]![1].onSuccess as () => void)());

    expect(root.getByTestId("product-row-p-single").props.accessibilityLabel).toContain("89");
  });
});

// --- state hygiene ------------------------------------------------------

describe("Products — sheet state hygiene", () => {
  /**
   * Orders shipped exactly this bug: a sheet backed out of without submitting
   * left its target pinned, so opening on a DIFFERENT row showed the previous
   * row's context. It cannot be caught by a single-row test.
   */
  it("clears its target when backed out of without submitting", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-single", "price");
    expect(root.getByTestId("variant-value-input").props.value).toBe("89");

    fireEvent.press(root.getByTestId("variant-value-dismiss"));
    expect(root.queryByTestId("variant-value-input")).toBeNull();

    menuAction(root, "p-multi", "stock");
    fireEvent.press(root.getByTestId("action-sheet-item-variant-v-blue-s"));
    expect(root.getByTestId("variant-value-input").props.value).toBe("9");
    expect(root.getByTestId("variant-value-context")).toHaveTextContent(/Manly Cap/);
  });

  // The picker is a step, not a layer: once it has been answered it must be
  // gone. Three live variant rows left mounted underneath the numeric sheet
  // are three targets a stray tap can re-aim it at mid-edit.
  it("retires the picker as soon as it has been answered", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-multi", "stock");
    expect(root.queryAllByTestId(/^action-sheet-item-variant-/)).toHaveLength(3);

    fireEvent.press(root.getByTestId("action-sheet-item-variant-v-blue-s"));
    expect(root.queryAllByTestId(/^action-sheet-item-variant-/)).toHaveLength(0);
    expect(root.getByTestId("variant-value-input")).toBeTruthy();
  });

  /**
   * react-query never resets a mutation error. Binding the sheet to
   * `mutation.error` means one failed edit greets the merchant on every
   * subsequent sheet, on a different product, for the life of the screen.
   * This is the test that fails if someone "simplifies" the local state away.
   */
  it("does not carry a failure into the next sheet", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-single", "price");
    fireEvent.changeText(root.getByTestId("variant-value-input"), "94.25");
    fireEvent.press(root.getByTestId("variant-value-submit"));
    act(() =>
      (mockQuickEdit.mock.calls[0]![1].onError as (e: unknown) => void)(new Error("nope")),
    );

    // It stays open, explaining itself, so the merchant can retry the value
    // they already typed.
    expect(root.getByTestId("variant-value-submit-error")).toBeTruthy();
    expect(root.getByTestId("variant-value-input").props.value).toBe("94.25");

    fireEvent.press(root.getByTestId("variant-value-dismiss"));
    menuAction(root, "p-multi", "price");
    fireEvent.press(root.getByTestId("action-sheet-item-variant-v-blue-s"));
    expect(root.queryByTestId("variant-value-submit-error")).toBeNull();
  });

  // A retry starts clean. Leaving the previous attempt's reason on screen
  // while the new one is in flight reads as though the retry has already
  // failed, on the one surface where the merchant is waiting for an answer.
  it("retires the reason the moment the merchant retries", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-single", "price");
    fireEvent.changeText(root.getByTestId("variant-value-input"), "94.25");
    fireEvent.press(root.getByTestId("variant-value-submit"));
    act(() =>
      (mockQuickEdit.mock.calls[0]![1].onError as (e: unknown) => void)(new Error("nope")),
    );
    expect(root.getByTestId("variant-value-submit-error")).toBeTruthy();

    fireEvent.press(root.getByTestId("variant-value-submit"));
    expect(root.queryByTestId("variant-value-submit-error")).toBeNull();
  });
});

// --- the busy guard and the failure notice ------------------------------

describe("Products — quick edit under the busy guard", () => {
  function submitPrice(root: ReturnType<typeof render>) {
    menuAction(root, "p-single", "price");
    fireEvent.changeText(root.getByTestId("variant-value-input"), "94.25");
    fireEvent.press(root.getByTestId("variant-value-submit"));
  }

  it("suppresses the row's long-press while its own edit is in flight", () => {
    const root = render(<ProductsScreen />);
    submitPrice(root);
    expect(productRow(root, "p-single")?.props.onLongPress).toBeUndefined();
    // A different row is untouched — the guard is per row.
    expect(productRow(root, "p-multi")?.props.onLongPress).toBeDefined();
  });

  it("re-arms the row once the edit settles", () => {
    const root = render(<ProductsScreen />);
    submitPrice(root);
    act(() => (mockQuickEdit.mock.calls[0]![1].onSuccess as () => void)());
    expect(productRow(root, "p-single")?.props.onLongPress).toBeDefined();
  });

  it("re-arms it on failure too, so a failed edit is retryable", () => {
    const root = render(<ProductsScreen />);
    submitPrice(root);
    act(() => (mockQuickEdit.mock.calls[0]![1].onError as (e: unknown) => void)(new Error("x")));
    expect(productRow(root, "p-single")?.props.onLongPress).toBeDefined();
  });

  // Task 14: a failure is EXPLAINED, not only felt. The action label is what
  // turns a bare haptic into a sentence naming what the merchant tried.
  it("names the action on the screen's failure notice", () => {
    const root = render(<ProductsScreen />);
    submitPrice(root);
    act(() => (mockQuickEdit.mock.calls[0]![1].onError as (e: unknown) => void)(new Error("x")));
    expect(root.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't update this price",
    );
  });

  it("names the stock action distinctly", () => {
    const root = render(<ProductsScreen />);
    menuAction(root, "p-single", "stock");
    fireEvent.changeText(root.getByTestId("variant-value-input"), "18");
    fireEvent.press(root.getByTestId("variant-value-submit"));
    act(() => (mockQuickEdit.mock.calls[0]![1].onError as (e: unknown) => void)(new Error("x")));
    expect(root.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't update this stock level",
    );
  });
});
