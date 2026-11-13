import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { AdminCategory } from "@/lib/api/marketplace-api";

const renameCategory = vi.fn();
vi.mock("@/app/(admin)/products/category-actions", () => ({
  renameCategory: (...a: unknown[]) => renameCategory(...a),
  setCategoryFeatured: vi.fn(),
}));
const toastError = vi.fn();
vi.mock("@/components/feedback/Toaster", () => ({
  useToast: () => ({ toast: { success: vi.fn(), error: toastError, info: vi.fn() } }),
}));

import { CategoriesFeaturedManager } from "./CategoriesFeaturedManager";

const car = { id: "c1", name: "car", slug: "car", featured: false } as AdminCategory;

describe("CategoriesFeaturedManager rename", () => {
  beforeEach(() => {
    renameCategory.mockReset();
    toastError.mockReset();
  });

  it("renames on Enter and keeps the slug", async () => {
    renameCategory.mockResolvedValue({ ok: true });
    render(<CategoriesFeaturedManager storeId="s1" categories={[car]} />);
    await userEvent.click(screen.getByRole("button", { name: "Rename car" }));
    const input = screen.getByLabelText("New name for car");
    await userEvent.clear(input);
    await userEvent.type(input, "Car designs{Enter}");

    await waitFor(() => expect(renameCategory).toHaveBeenCalledWith("s1", "c1", "Car designs"));
    expect(screen.getByText("Car designs")).toBeInTheDocument();
    expect(screen.getByText("car")).toBeInTheDocument(); // slug column unchanged
  });

  it("puts the old name back when the save fails", async () => {
    renameCategory.mockResolvedValue({ ok: false, error: "nope" });
    render(<CategoriesFeaturedManager storeId="s1" categories={[car]} />);
    await userEvent.click(screen.getByRole("button", { name: "Rename car" }));
    const input = screen.getByLabelText("New name for car");
    await userEvent.clear(input);
    await userEvent.type(input, "Cars{Enter}");

    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(screen.queryByText("Cars")).not.toBeInTheDocument();
  });
});
