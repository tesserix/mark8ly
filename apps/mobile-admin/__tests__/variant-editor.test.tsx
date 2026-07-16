// `@/components/ui` barrel re-exports BackHeader/SearchField, which import
// icons from lucide-react-native's ESM build (`dist/esm/...mjs`) — not
// covered by jest-expo's default transformIgnorePatterns, so requiring it
// unmocked throws "Unexpected token 'export'". Stub every icon export with a
// no-op component; we don't assert on icon rendering here.
jest.mock("lucide-react-native", () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
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
  it("commits a weight edit on blur as weight_grams", () => {
    const onUpdate = jest.fn();
    const { getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
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
    const { getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
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
