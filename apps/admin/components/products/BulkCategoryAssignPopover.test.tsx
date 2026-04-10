import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { BulkCategoryAssignPopover } from "./BulkCategoryAssignPopover";
import type { AdminCategory } from "@/lib/api/marketplace-api";

const categories: AdminCategory[] = [
  { id: "c1", store_id: "s1", parent_id: null, name: "Shoes", slug: "shoes", description: null, image_url: null, position: 0, is_active: true },
  { id: "c2", store_id: "s1", parent_id: null, name: "Bags", slug: "bags", description: null, image_url: null, position: 1, is_active: true },
];

describe("BulkCategoryAssignPopover", () => {
  const onApply = vi.fn();
  const onClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the category picker when open", () => {
    render(
      <BulkCategoryAssignPopover
        isOpen
        categories={categories}
        count={5}
        onApply={onApply}
        onClose={onClose}
      />,
    );

    expect(screen.getByLabelText(/add category/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /apply to 5/i })).toBeInTheDocument();
  });

  it("selecting a category and applying calls onApply with ids", async () => {
    const user = userEvent.setup();

    render(
      <BulkCategoryAssignPopover
        isOpen
        categories={categories}
        count={3}
        onApply={onApply}
        onClose={onClose}
      />,
    );

    // Focus the input to open the dropdown
    const input = screen.getByLabelText(/add category/i);
    await user.click(input);

    // Click on "Shoes" from the dropdown
    const shoesOption = screen.getByText("Shoes");
    await user.click(shoesOption);

    // Click apply
    await user.click(screen.getByRole("button", { name: /apply to 3/i }));

    expect(onApply).toHaveBeenCalledWith(["c1"]);
  });

  it("does not render when not open", () => {
    const { container } = render(
      <BulkCategoryAssignPopover
        isOpen={false}
        categories={categories}
        count={3}
        onApply={onApply}
        onClose={onClose}
      />,
    );

    expect(container.innerHTML).toBe("");
  });
});
