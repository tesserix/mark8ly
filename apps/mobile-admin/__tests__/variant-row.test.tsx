// See section-disclosure.test.tsx for why both mocks below are needed.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

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

import { render, fireEvent } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import { VariantRow, stockTone, LOW_STOCK } from "@/components/products/VariantRow";
import { formatMoney } from "@/lib/product-display";
import { theme } from "@/lib/theme";

const BASE_VARIANT = {
  id: "v1",
  sku: "SKU-123",
  price: 34,
  currency_code: "AUD",
  inventory_quantity: 12,
  inventory_policy: "deny",
  option_values: [{ option_name: "Size", option_value_id: "a", value: "M" }],
  position: 0,
} as const;

describe("stockTone", () => {
  it("maps 0 to muted / Out of stock", () => {
    expect(stockTone(0)).toEqual({ tone: "muted", label: "Out of stock" });
  });

  it("maps the LOW_STOCK boundary itself to warning", () => {
    const result = stockTone(LOW_STOCK);
    expect(result.tone).toBe("warning");
    expect(result.label).toBe(`Low: ${LOW_STOCK}`);
  });

  it("maps LOW_STOCK + 1 to muted (not success) — healthy stock must not spend the moss accent", () => {
    const result = stockTone(LOW_STOCK + 1);
    expect(result.tone).toBe("muted");
    expect(result.label).toBe(`${LOW_STOCK + 1} in stock`);
  });

  it("distinguishes out-of-stock from healthy-stock by label even though both are muted", () => {
    const outOfStock = stockTone(0);
    const healthy = stockTone(LOW_STOCK + 1);
    expect(outOfStock.tone).toBe("muted");
    expect(healthy.tone).toBe("muted");
    expect(outOfStock.label).not.toBe(healthy.label);
  });
});

describe("VariantRow", () => {
  it("starts collapsed — the VariantEditor body is not rendered", () => {
    const { queryByLabelText } = render(
      <VariantRow variant={BASE_VARIANT as never} onUpdate={jest.fn()} />,
    );
    expect(queryByLabelText("SKU")).toBeNull();
  });

  it("expands into the VariantEditor body on tap", () => {
    const { getByRole, getByLabelText } = render(
      <VariantRow variant={BASE_VARIANT as never} onUpdate={jest.fn()} />,
    );
    fireEvent.press(getByRole("button"));
    expect(getByLabelText("SKU")).toBeTruthy();
  });

  it("shows the variant label and price · stock · sku caption in the summary", () => {
    const { getByText } = render(
      <VariantRow variant={BASE_VARIANT as never} onUpdate={jest.fn()} />,
    );
    expect(getByText("M")).toBeTruthy();
    expect(
      getByText(`${formatMoney(34, "AUD")} · 12 in stock · SKU-123`),
    ).toBeTruthy();
  });

  it("shows 'Default variant' for a sole variant with no option values", () => {
    const soleVariant = { ...BASE_VARIANT, option_values: [] };
    const { getByText } = render(
      <VariantRow variant={soleVariant as never} onUpdate={jest.fn()} />,
    );
    expect(getByText("Default variant")).toBeTruthy();
  });

  it("auto-expands when defaultOpen is passed (the sole-variant case)", () => {
    const soleVariant = { ...BASE_VARIANT, option_values: [] };
    const { getByLabelText } = render(
      <VariantRow variant={soleVariant as never} onUpdate={jest.fn()} defaultOpen />,
    );
    expect(getByLabelText("SKU")).toBeTruthy();
  });

  it("stays collapsed without defaultOpen even for a sole variant", () => {
    const soleVariant = { ...BASE_VARIANT, option_values: [] };
    const { queryByLabelText } = render(
      <VariantRow variant={soleVariant as never} onUpdate={jest.fn()} />,
    );
    expect(queryByLabelText("SKU")).toBeNull();
  });

  it("sizes the summary row at the 64pt row-scale minHeight with the 20pt row gutter", () => {
    const { getByRole } = render(
      <VariantRow variant={BASE_VARIANT as never} onUpdate={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByRole("button").props.style);
    expect(style.minHeight).toBe(theme.row.minHeightSingle);
    expect(style.paddingHorizontal).toBe(theme.row.paddingH);
  });
});

describe("VariantRow — reduced motion", () => {
  const reanimated = jest.requireMock("react-native-reanimated") as {
    useReducedMotion: jest.Mock;
  };

  afterEach(() => {
    reanimated.useReducedMotion.mockReturnValue(false);
  });

  it("passes an undefined (instant) entering animation on the body when reduced motion is enabled", () => {
    reanimated.useReducedMotion.mockReturnValue(true);
    const { getByTestId } = render(
      <VariantRow variant={BASE_VARIANT as never} onUpdate={jest.fn()} defaultOpen />,
    );
    expect(getByTestId("variant-row-body").props.entering).toBeUndefined();
  });

  it("passes a real entering animation on the body when reduced motion is off", () => {
    reanimated.useReducedMotion.mockReturnValue(false);
    const { getByTestId } = render(
      <VariantRow variant={BASE_VARIANT as never} onUpdate={jest.fn()} defaultOpen />,
    );
    expect(getByTestId("variant-row-body").props.entering).toBeDefined();
  });
});
