import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ProductFormTabs } from "./ProductFormTabs";

describe("ProductFormTabs", () => {
  it("renders all tabs with badges and marks active", () => {
    render(
      <ProductFormTabs
        active="media"
        onChange={() => {}}
        badges={{ media: 3, variants: 0 }}
      />,
    );
    const mediaTab = screen.getByRole("tab", { name: /media/i });
    expect(mediaTab).toHaveAttribute("aria-selected", "true");
    expect(mediaTab).toHaveTextContent("3");
    // badge=0 should not render
    const variantsTab = screen.getByRole("tab", { name: /^variants$/i });
    expect(variantsTab).not.toHaveTextContent("0");
  });

  it("click changes tab; disabled tab click is ignored", () => {
    const onChange = vi.fn();
    render(
      <ProductFormTabs
        active="general"
        onChange={onChange}
        disabled={{ variants: "Add options first" }}
      />,
    );
    fireEvent.click(screen.getByRole("tab", { name: /options/i }));
    expect(onChange).toHaveBeenCalledWith("options");
    onChange.mockClear();
    fireEvent.click(screen.getByRole("tab", { name: /variants/i }));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("keyboard nav: Arrow keys, Home, End, skipping disabled", () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <ProductFormTabs
        active="general"
        onChange={onChange}
        disabled={{ variants: "nope" }}
      />,
    );
    const general = screen.getByRole("tab", { name: /general/i });
    fireEvent.keyDown(general, { key: "ArrowRight" });
    expect(onChange).toHaveBeenLastCalledWith("media");
    fireEvent.keyDown(general, { key: "ArrowLeft" });
    // wraps to variants (disabled) -> ignored
    expect(onChange).toHaveBeenCalledTimes(1);
    fireEvent.keyDown(general, { key: "End" });
    expect(onChange).toHaveBeenLastCalledWith("variants");
    fireEvent.keyDown(general, { key: "Home" });
    expect(onChange).toHaveBeenLastCalledWith("general");
    // unknown key is no-op
    fireEvent.keyDown(general, { key: "Space" });
    expect(onChange).toHaveBeenCalledTimes(3);
    rerender(<ProductFormTabs active="options" onChange={onChange} />);
    fireEvent.keyDown(screen.getByRole("tab", { name: /options/i }), { key: "ArrowLeft" });
    expect(onChange).toHaveBeenLastCalledWith("media");
  });
});
