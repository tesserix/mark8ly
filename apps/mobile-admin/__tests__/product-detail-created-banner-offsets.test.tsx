// Whole-branch review bug: the "next steps" banner's jump-to-section
// navigation is broken because the three instrumented section `onLayout`
// views (photos/options/variants) used to sit inside bare, style-less
// "Movement" <View> wrappers instead of being direct children of the
// ScrollView content. React Native's `onLayout` reports `layout.y` relative
// to the IMMEDIATE PARENT, so the offsets registered via
// `registerSectionOffset` were parent-relative, not page-relative —
// `jumpTo("options")` landed at the top of the page instead of the Options
// section, and `jumpTo("variants")` landed a full movement too high.
//
// __tests__/use-created-banner.test.tsx hand-feeds offsets straight into
// `registerSectionOffset` — it exercises the jump math, never the real
// onLayout nesting, so it cannot catch this class of bug.
//
// jest/react-test-renderer never runs a real (Yoga) layout pass, so there is
// no way to make RN *compute* a parent-relative `layout.y` for us — any `y`
// fired via `fireEvent(node, "layout", {...})` is whatever the test author
// supplies, regardless of the tree. A test that hand-picks "plausible" `y`s
// for each section (as the buggy narrative describes) would pass or fail by
// the author's say-so, not by the tree's actual shape — exactly the flaw
// this task calls out.
//
// So this test derives each fired `y` from a fact React genuinely computed
// from the REAL rendered tree: each onLayout view's index among its own
// ACTUAL immediate parent's children (`node.parent.children.indexOf(node)`).
// That index *is* what onLayout's "relative to immediate parent" contract
// would report if the parent's own children were laid out as a simple
// vertical stack starting at 0 — which is exactly the shape of the bug:
// on the buggy tree "options" and "variants" share Movement-2 as their
// parent, so both offsets collapse toward Movement-2's own local index
// space (options a low index, variants only slightly higher) instead of the
// page's; on the fixed tree all three share the ScrollView content as their
// parent, so their indices are the page's real top-to-bottom order.

// `@/components/ui`'s barrel re-exports BackHeader/SearchField, which import
// icons from lucide-react-native's ESM build — not covered by jest-expo's
// default transformIgnorePatterns. Stub every icon export with a no-op
// component (same fix as new-product.test.tsx / variant-row.test.tsx).
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

// VariantRow imports react-native-reanimated directly (Animated, FadeIn,
// FadeOut, useReducedMotion) and disclosure-motion.ts pulls in Easing,
// useAnimatedStyle and withTiming. The real module (and its shipped mock.js)
// needs the native Worklets module, which throws under jest. Hand-roll the
// same minimal virtual mock __tests__/variant-row.test.tsx uses.
jest.mock("react-native-reanimated", () => {
  const { View } = require("react-native");
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
    default: { View },
    FadeIn: new ChainableAnimation(),
    FadeOut: new ChainableAnimation(),
    Easing: { bezier: () => (t: number) => t },
    useReducedMotion: jest.fn(() => false),
    useAnimatedStyle: (factory: () => unknown) => factory(),
    withTiming: (
      toValue: unknown,
      _config?: unknown,
      callback?: (finished: boolean) => void,
    ) => {
      callback?.(true);
      return toValue;
    },
  };
});

// OptionsEditor -> OptionBuilderSheet and CategoryField -> CategoryPickerSheet
// both render a real @gorhom/bottom-sheet BottomSheetModal, which needs the
// native Worklets module. Reuse the shared virtual mock new-product.test.tsx
// uses (kept in its own module, not inlined, because nativewind's babel
// transform instruments JSX with a `_ReactNativeCSSInterop` reference that
// jest.mock() factories are hoisted above).
jest.mock("@gorhom/bottom-sheet", () => require("../lib/test-support/gorhom-bottom-sheet-mock"));

jest.mock("expo-haptics", () => ({
  selectionAsync: jest.fn(),
  notificationAsync: jest.fn(),
  NotificationFeedbackType: { Success: "success" },
}));

// Screen/ImageViewer/useDockClearance all call useSafeAreaInsets(), which
// throws without a <SafeAreaProvider> ancestor. react-native-safe-area-context
// ships an official jest mock for exactly this (same fix as
// __tests__/new-product.test.tsx and __tests__/TenantGate.test.tsx).
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

jest.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ id: "product-1", created: "1" }),
  useRouter: () => ({ back: jest.fn(), replace: jest.fn(), push: jest.fn() }),
}));

jest.mock("@/lib/hooks/use-products", () => ({
  useProduct: () => ({ data: PRODUCT, isLoading: false, error: undefined }),
}));

const mockUpdateMutate = jest.fn();
jest.mock("@/lib/admin-api/product-crud", () => ({
  useUpdateProduct: () => ({ mutate: mockUpdateMutate, isPending: false }),
  useDeleteMedia: () => ({ mutate: jest.fn(), isPending: false }),
  useUpdateVariant: () => ({ mutate: jest.fn(), isPending: false }),
  useAddProductMedia: () => ({ mutate: jest.fn(), isPending: false }),
  useUpdateMedia: () => ({ mutate: jest.fn(), isPending: false }),
  useCategories: () => ({
    data: [],
    isLoading: false,
    error: undefined,
    refetch: jest.fn(),
  }),
}));

import { render, fireEvent } from "@testing-library/react-native";
import { View } from "react-native";
import type { ReactTestInstance } from "react-test-renderer";
import ProductDetailScreen from "../app/(tabs)/products/[id]";

const PRODUCT = {
  id: "product-1",
  store_id: "store-1",
  handle: "ceramic-mug",
  title: "Ceramic Mug",
  description: "A mug.",
  status: "active",
  tags: [],
  categories: [],
  options: [
    {
      id: "opt-1",
      name: "Size",
      position: 0,
      values: [
        { id: "v-1", value: "S", position: 0 },
        { id: "v-2", value: "M", position: 1 },
      ],
    },
  ],
  variants: [
    {
      id: "var-1",
      sku: "MUG-S",
      price: "24.50",
      currency_code: "AUD",
      inventory_quantity: 10,
      inventory_policy: "deny",
      option_values: [{ option_name: "Size", option_value_id: "v-1", value: "S" }],
      position: 0,
    },
    {
      id: "var-2",
      sku: "MUG-M",
      price: "26.50",
      currency_code: "AUD",
      inventory_quantity: 5,
      inventory_policy: "deny",
      option_values: [{ option_name: "Size", option_value_id: "v-2", value: "M" }],
      position: 1,
    },
  ],
  media: [
    { id: "media-1", url: "https://cdn.example/mug.jpg", storage_key: "k1", position: 0 },
  ],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

/** This element's position among its OWN real immediate parent's children. */
function siblingIndex(node: ReactTestInstance): number {
  if (!node.parent) return 0;
  return node.parent.children.indexOf(node);
}

describe("product detail screen — created-banner jump offsets (real onLayout nesting)", () => {
  it("registers photos < options < variants once the sections are direct ScrollView children", () => {
    const { getByLabelText, UNSAFE_root } = render(<ProductDetailScreen />);

    // The 3 real onLayout-bearing section Views, in real render/document
    // order (findAll walks the actual rendered tree depth-first, so this
    // order — photos, then options, then variants — holds on both the buggy
    // and the fixed tree; only their PARENTAGE differs between the two).
    const layoutNodes = UNSAFE_root.findAll(
      (node) => node.type === View && typeof node.props.onLayout === "function",
    );
    expect(layoutNodes).toHaveLength(3);
    const [photosNode, optionsNode, variantsNode] = layoutNodes;

    // The structural invariant the fix establishes: all three instrumented
    // views share the SAME immediate parent (the ScrollView content), so
    // `onLayout`'s "relative to immediate parent" contract reports a real,
    // page-relative position for each. On the buggy tree, photos sits under
    // Movement-1 while options/variants sit under Movement-2 — two
    // different parents — which is the root cause this assertion pins.
    // Compared as booleans (not `.toBe(...)` on the raw instances) — on
    // failure, Jest's diff formatter tries to pretty-print the two
    // ReactTestInstance objects, which recurse through RN's internal
    // ScrollView mock getters and blow the stack.
    expect(photosNode.parent === optionsNode.parent).toBe(true);
    expect(optionsNode.parent === variantsNode.parent).toBe(true);

    // Fire the real onLayout handlers with the `y` each view's REAL parent
    // relationship actually produces: its index among its own actual
    // parent's children, scaled up so `jumpTo`'s `Math.max(y - 16, 0)` floor
    // clamp can't collapse small sibling indices (0, 1, 2…) into the same
    // clamped 0 regardless of tree shape. On the buggy tree this collapses
    // options/variants into Movement-2's small local index space (both low,
    // non-monotonic with photos' Movement-1-local index); on the fixed tree
    // it's the page's real top-to-bottom order.
    const SCALE = 100;
    fireEvent(photosNode, "layout", {
      nativeEvent: { layout: { x: 0, y: siblingIndex(photosNode) * SCALE, width: 320, height: 40 } },
    });
    fireEvent(optionsNode, "layout", {
      nativeEvent: { layout: { x: 0, y: siblingIndex(optionsNode) * SCALE, width: 320, height: 40 } },
    });
    fireEvent(variantsNode, "layout", {
      nativeEvent: {
        layout: { x: 0, y: siblingIndex(variantsNode) * SCALE, width: 320, height: 40 },
      },
    });

    // Observe the effect through the real production path: press each
    // banner chip (real jumpTo -> real scrollTo on the real ScrollView
    // instance), the same way a user hitting "Add options" would.
    const scrollViewInstance = UNSAFE_root.findAll(
      (node) => !!node.instance && typeof node.instance.scrollTo === "function",
    )[0]?.instance;
    expect(scrollViewInstance).toBeTruthy();
    const scrollToSpy = jest.spyOn(scrollViewInstance, "scrollTo").mockImplementation(() => {});

    const lastScrollY = () => (scrollToSpy.mock.calls.at(-1)?.[0] as { y?: number } | undefined)?.y;

    fireEvent.press(getByLabelText("Add photos"));
    const photosY = lastScrollY();
    fireEvent.press(getByLabelText("Add options"));
    const optionsY = lastScrollY();
    fireEvent.press(getByLabelText("Review variants"));
    const variantsY = lastScrollY();

    expect(typeof photosY).toBe("number");
    expect(typeof optionsY).toBe("number");
    expect(typeof variantsY).toBe("number");
    expect(photosY as number).toBeLessThan(optionsY as number);
    expect(optionsY as number).toBeLessThan(variantsY as number);
  });
});
