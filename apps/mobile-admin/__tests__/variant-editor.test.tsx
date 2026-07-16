// `@/components/ui` barrel re-exports BackHeader/SearchField, which import
// icons from lucide-react-native's ESM build (`dist/esm/...mjs`) — not
// covered by jest-expo's default transformIgnorePatterns, so requiring it
// unmocked throws "Unexpected token 'export'". Stub every icon export with a
// no-op component; we don't assert on icon rendering here.
jest.mock("lucide-react-native", () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
});

// VariantEditor now nests its Shipping fields behind <SectionDisclosure>,
// which pulls react-native-reanimated. The real module (and even its shipped
// mock.js) initializes a native Worklets module at import time, which throws
// under jest — see section-disclosure.test.tsx for the full explanation.
// Hand-roll the same minimal virtual mock here.
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
    useReducedMotion: () => false,
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
import { VariantEditor, variantLabel } from "@/components/products/VariantEditor";

const VARIANT = {
  id: "v1",
  sku: "TBS-PBLR-XS-S",
  price: 199,
  currency_code: "AUD",
  inventory_quantity: 4,
  inventory_policy: "deny",
  option_values: [],
  position: 0,
} as const;

describe("variantLabel", () => {
  it("uses option values when present", () => {
    expect(
      variantLabel({
        ...VARIANT,
        option_values: [
          { option_name: "Size", option_value_id: "a", value: "M" },
          { option_name: "Colour", option_value_id: "b", value: "Blue" },
        ],
      } as never),
    ).toBe("M / Blue");
  });

  it("falls back to SKU when the variant has no option values", () => {
    expect(variantLabel(VARIANT as never)).toBe("TBS-PBLR-XS-S");
  });
});

describe("VariantEditor", () => {
  it("keeps the Shipping & dimensions fields collapsed until asked", () => {
    const { getByRole, queryByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={jest.fn()} />,
    );
    expect(queryByLabelText("Weight in grams")).toBeNull();
    fireEvent.press(getByRole("button"));
    expect(queryByLabelText("Weight in grams")).toBeTruthy();
  });

  it("commits a weight edit on blur as weight_grams", () => {
    const onUpdate = jest.fn();
    const { getByRole, getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
    fireEvent.press(getByRole("button"));
    const input = getByLabelText("Weight in grams");
    fireEvent.changeText(input, "450");
    fireEvent(input, "blur");
    expect(onUpdate).toHaveBeenCalledWith("v1", { weight_grams: 450 });
  });

  it("commits an SKU edit on blur", () => {
    const onUpdate = jest.fn();
    const { getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
    const input = getByLabelText("SKU");
    fireEvent.changeText(input, "NEW-SKU-1");
    fireEvent(input, "blur");
    expect(onUpdate).toHaveBeenCalledWith("v1", { sku: "NEW-SKU-1" });
  });

  it("does NOT fire when the value is unchanged", () => {
    const onUpdate = jest.fn();
    const { getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
    const input = getByLabelText("SKU");
    fireEvent(input, "blur");
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("does NOT fire on unparseable input rather than sending NaN", () => {
    const onUpdate = jest.fn();
    const { getByRole, getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
    fireEvent.press(getByRole("button"));
    const input = getByLabelText("Weight in grams");
    fireEvent.changeText(input, "heavy");
    fireEvent(input, "blur");
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("rejects an empty SKU — the backend requires it", () => {
    const onUpdate = jest.fn();
    const { getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
    const input = getByLabelText("SKU");
    fireEvent.changeText(input, "   ");
    fireEvent(input, "blur");
    expect(onUpdate).not.toHaveBeenCalled();
  });
});
