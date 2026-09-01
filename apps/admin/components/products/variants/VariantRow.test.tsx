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
  optionNames: string[] = ["Color"],
) {
  render(
    <table>
      <tbody>
        <VariantRow
          variant={{ ...baseVariant, ...override }}
          index={0}
          optionNames={optionNames}
          currencyCode="USD"
          media={mediaList}
          onPatch={onPatch}
        />
      </tbody>
    </table>,
  );
  return onPatch;
}

/** Weight, dimensions and the image now live behind the row's disclosure. */
function openDetails(): void {
  fireEvent.click(screen.getByRole("button", { name: /weight, dimensions and image/i }));
}

describe("VariantRow", () => {
  it("commits sku on blur and weight on blur, skips no-op change", () => {
    const onPatch = renderRow();
    const sku = screen.getByLabelText("SKU");
    fireEvent.change(sku, { target: { value: "SKU-2" } });
    fireEvent.blur(sku);
    expect(onPatch).toHaveBeenCalledWith({ sku: "SKU-2" });

    openDetails();
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
    openDetails();
    const weight = screen.getByLabelText("Weight");
    fireEvent.change(weight, { target: { value: "xyz" } });
    fireEvent.blur(weight);
    expect(onPatch).not.toHaveBeenCalled();
  });

  it("selecting an image in the details panel emits variantImageId", () => {
    const onPatch = renderRow();
    openDetails();
    const thumb = screen.getByRole("radio", { name: /pic/i });
    fireEvent.click(thumb);
    expect(onPatch).toHaveBeenCalledWith({ variantImageId: "m1" });
  });

  it("renders current media image when variantImageId matches", () => {
    renderRow({ variantImageId: "m1" });
    openDetails();
    const img = screen.getByAltText("pic") as HTMLImageElement;
    expect(img.src).toContain("a.jpg");
  });

  it("names the row by its option values", () => {
    renderRow();
    expect(screen.getByText("Red")).toBeInTheDocument();
  });

  // 8 of the-bondi-store's 12 products have variants with no options at
  // all. Those rows have nothing to compose a name from, and must still be
  // tellable apart rather than rendering as blank cells.
  it("falls back to its position when the variant has no options", () => {
    render(
      <table>
        <tbody>
          <VariantRow
            variant={{ ...baseVariant, optionValues: [] }}
            index={2}
            optionNames={[]}
            currencyCode="USD"
            media={media}
            onPatch={vi.fn()}
          />
        </tbody>
      </table>,
    );
    expect(screen.getByText("Variant 3")).toBeInTheDocument();
  });

  it("does not invent an option name for an unnamed variant", () => {
    render(
      <table>
        <tbody>
          <VariantRow
            variant={{ ...baseVariant, optionValues: [] }}
            index={0}
            optionNames={[]}
            currencyCode="USD"
            media={media}
            onPatch={vi.fn()}
          />
        </tbody>
      </table>,
    );
    expect(screen.queryByText(/option a/i)).not.toBeInTheDocument();
    expect(screen.queryByText("—")).not.toBeInTheDocument();
  });

  it("collapses the details panel again on a second click", () => {
    renderRow();
    openDetails();
    expect(screen.getByLabelText("Weight")).toBeInTheDocument();
    openDetails();
    expect(screen.queryByLabelText("Weight")).not.toBeInTheDocument();
  });
});
