import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { VariantRow } from "./VariantRow";
import type { VariantDraft } from "./VariantMatrixTable";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";

const baseVariant: VariantDraft = {
  id: "v1",
  key: "Red|M",
  price: "19.99",
  sku: "SKU-1",
  stock: 5,
  weight: 0.25,
  variantImageId: null,
  optionValues: [{ optionName: "Color", value: "Red" }],
};

const media: AdminMediaResponse[] = [
  {
    id: "m1",
    url: "https://cdn.test/a.jpg",
    storage_key: "k",
    alt: "pic",
    position: 0,
    media_type: "image",
    variant_id: null,
    width: 10,
    height: 10,
    bytes: 1,
  },
];

function renderRow(
  override: Partial<VariantDraft> = {},
  onPatch = vi.fn(),
  mediaList: AdminMediaResponse[] = media,
) {
  render(
    <table>
      <tbody>
        <VariantRow
          variant={{ ...baseVariant, ...override }}
          optionNames={["Color"]}
          currencyCode="USD"
          media={mediaList}
          onPatch={onPatch}
        />
      </tbody>
    </table>,
  );
  return onPatch;
}

describe("VariantRow", () => {
  it("commits sku on blur and weight on blur, skips no-op change", () => {
    const onPatch = renderRow();
    const sku = screen.getByLabelText("SKU");
    fireEvent.change(sku, { target: { value: "SKU-2" } });
    fireEvent.blur(sku);
    expect(onPatch).toHaveBeenCalledWith({ sku: "SKU-2" });

    const weight = screen.getByLabelText("Weight");
    fireEvent.change(weight, { target: { value: "1.5" } });
    fireEvent.blur(weight);
    expect(onPatch).toHaveBeenCalledWith({ weight: 1.5 });

    // no-op: same value → no extra calls for price
    onPatch.mockClear();
    const price = screen.getByLabelText("Price");
    fireEvent.blur(price);
    expect(onPatch).not.toHaveBeenCalled();
  });

  it("ignores invalid stock/weight on blur (NaN branch)", () => {
    const onPatch = renderRow();
    const stock = screen.getByLabelText("Stock");
    fireEvent.change(stock, { target: { value: "abc" } });
    fireEvent.blur(stock);
    const weight = screen.getByLabelText("Weight");
    fireEvent.change(weight, { target: { value: "xyz" } });
    fireEvent.blur(weight);
    expect(onPatch).not.toHaveBeenCalled();
  });

  it("toggles image picker and selecting an image emits variantImageId", () => {
    const onPatch = renderRow();
    const imgBtn = screen.getByRole("button", { name: /variant image/i });
    fireEvent.click(imgBtn);
    // Picker lists media thumbnails — click the first media option
    const thumb = screen.getByRole("radio", { name: /pic/i });
    fireEvent.click(thumb);
    expect(onPatch).toHaveBeenCalledWith({ variantImageId: "m1" });
  });

  it("renders current media image when variantImageId matches", () => {
    renderRow({ variantImageId: "m1" });
    const img = screen.getByAltText("pic") as HTMLImageElement;
    expect(img.src).toContain("a.jpg");
  });
});
