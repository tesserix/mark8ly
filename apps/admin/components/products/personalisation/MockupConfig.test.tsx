import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

import { MockupConfig } from "./MockupConfig";

// The control that made the 2D composite reachable (#966).
//
// The storefront side shipped first and could never be used: the
// database, the API and the types all supported mockup_storage_key and
// print_area, and nothing in the admin could set either. These pin the
// behaviours that decide whether a merchant can configure it at all,
// and whether what they configure is valid.

const media = [
  { id: "m1", url: "/a.jpg", storage_key: "tenants/t1/products/media/a/a.jpg", alt: "Front" },
  { id: "m2", url: "/b.jpg", storage_key: "tenants/t1/products/media/b/b.jpg", alt: "Back" },
];

describe("MockupConfig", () => {
  it("tells the merchant what to do when the product has no images", () => {
    render(<MockupConfig media={[]} onSave={vi.fn()} />);
    expect(screen.getByText(/Add a product image first/i)).toBeInTheDocument();
  });

  it("saves the selected image and the rectangle together", () => {
    const onSave = vi.fn();
    render(<MockupConfig media={media} onSave={onSave} />);

    fireEvent.click(screen.getAllByRole("button", { pressed: false })[0]!);
    fireEvent.click(screen.getByRole("button", { name: /save mockup/i }));

    expect(onSave).toHaveBeenCalledWith({
      mockup_storage_key: media[0]!.storage_key,
      // Defaults to a centred half-size box rather than nothing, so a
      // merchant who just picks an image gets something usable.
      print_area: { x: 25, y: 25, w: 50, h: 50 },
    });
  });

  it("clears both columns together when no image is selected", () => {
    // Half a pair is not a feature: the storefront drops a mockup with
    // no rectangle anyway, so clearing has to null both.
    const onSave = vi.fn();
    render(
      <MockupConfig
        media={media}
        mockupStorageKey={media[0]!.storage_key}
        printArea={{ x: 1, y: 2, w: 3, h: 4 }}
        onSave={onSave}
      />,
    );
    // Deselect the currently-chosen image.
    fireEvent.click(screen.getByRole("button", { pressed: true }));
    fireEvent.click(screen.getByRole("button", { name: /remove mockup/i }));

    expect(onSave).toHaveBeenCalledWith({
      mockup_storage_key: null,
      print_area: null,
    });
  });

  it("refuses a zero-width rectangle rather than saving an invisible print area", () => {
    const onSave = vi.fn();
    render(
      <MockupConfig
        media={media}
        mockupStorageKey={media[0]!.storage_key}
        printArea={{ x: 10, y: 10, w: 50, h: 50 }}
        onSave={onSave}
      />,
    );
    fireEvent.change(screen.getByLabelText(/width %/i), { target: { value: "0" } });

    const save = screen.getByRole("button", { name: /save mockup/i });
    expect(save).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent(/above zero/i);
    fireEvent.click(save);
    expect(onSave).not.toHaveBeenCalled();
  });

  it("clamps an out-of-range percentage on save", () => {
    // The rectangle reaches the storefront and positions the buyer's
    // photo; >100 would push it off the mockup and across the page.
    const onSave = vi.fn();
    render(
      <MockupConfig
        media={media}
        mockupStorageKey={media[0]!.storage_key}
        printArea={{ x: 10, y: 10, w: 50, h: 50 }}
        onSave={onSave}
      />,
    );
    fireEvent.change(screen.getByLabelText(/left %/i), { target: { value: "150" } });
    fireEvent.change(screen.getByLabelText(/top %/i), { target: { value: "-20" } });
    fireEvent.click(screen.getByRole("button", { name: /save mockup/i }));

    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ print_area: expect.objectContaining({ x: 100, y: 0 }) }),
    );
  });
});
