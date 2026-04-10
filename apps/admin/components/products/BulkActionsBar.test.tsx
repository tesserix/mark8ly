import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { BulkActionsBar } from "./BulkActionsBar";
import type { AdminCategory } from "@/lib/api/marketplace-api";

const categories: AdminCategory[] = [
  { id: "c1", store_id: "s1", parent_id: null, name: "Shoes", slug: "shoes", description: null, image_url: null, position: 0, is_active: true },
];

describe("BulkActionsBar", () => {
  const onActionComplete = vi.fn();
  const onCopyClick = vi.fn();
  const onDeleteClick = vi.fn();
  const onCategoryAssignClick = vi.fn();
  const onBulkAction = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders nothing when no items are selected", () => {
    const { container } = render(
      <BulkActionsBar
        selectedIds={[]}
        role="admin"
        storeId="s1"
        categories={categories}
        onActionComplete={onActionComplete}
        onCopyClick={onCopyClick}
        onDeleteClick={onDeleteClick}
        onCategoryAssignClick={onCategoryAssignClick}
        onBulkAction={onBulkAction}
      />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("renders the bar with count and actions when items are selected", () => {
    render(
      <BulkActionsBar
        selectedIds={["p1", "p2"]}
        role="admin"
        storeId="s1"
        categories={categories}
        onActionComplete={onActionComplete}
        onCopyClick={onCopyClick}
        onDeleteClick={onDeleteClick}
        onCategoryAssignClick={onCategoryAssignClick}
        onBulkAction={onBulkAction}
      />,
    );
    expect(screen.getByText(/2 selected/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^archive$/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^publish$/i })).toBeInTheDocument();
  });

  it("gates delete to owner role only", () => {
    const { rerender } = render(
      <BulkActionsBar
        selectedIds={["p1"]}
        role="admin"
        storeId="s1"
        categories={categories}
        onActionComplete={onActionComplete}
        onCopyClick={onCopyClick}
        onDeleteClick={onDeleteClick}
        onCategoryAssignClick={onCategoryAssignClick}
        onBulkAction={onBulkAction}
      />,
    );
    expect(screen.queryByRole("button", { name: /^delete$/i })).not.toBeInTheDocument();

    rerender(
      <BulkActionsBar
        selectedIds={["p1"]}
        role="owner"
        storeId="s1"
        categories={categories}
        onActionComplete={onActionComplete}
        onCopyClick={onCopyClick}
        onDeleteClick={onDeleteClick}
        onCategoryAssignClick={onCategoryAssignClick}
        onBulkAction={onBulkAction}
      />,
    );
    expect(screen.getByRole("button", { name: /^delete$/i })).toBeInTheDocument();
  });

  it("renders nothing for staff role", () => {
    const { container } = render(
      <BulkActionsBar
        selectedIds={["p1"]}
        role="staff"
        storeId="s1"
        categories={categories}
        onActionComplete={onActionComplete}
        onCopyClick={onCopyClick}
        onDeleteClick={onDeleteClick}
        onCategoryAssignClick={onCategoryAssignClick}
        onBulkAction={onBulkAction}
      />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("calls onBulkAction with 'archive' when archive is clicked", async () => {
    const user = userEvent.setup();

    render(
      <BulkActionsBar
        selectedIds={["p1", "p2"]}
        role="admin"
        storeId="s1"
        categories={categories}
        onActionComplete={onActionComplete}
        onCopyClick={onCopyClick}
        onDeleteClick={onDeleteClick}
        onCategoryAssignClick={onCategoryAssignClick}
        onBulkAction={onBulkAction}
      />,
    );

    await user.click(screen.getByRole("button", { name: /^archive$/i }));
    expect(onBulkAction).toHaveBeenCalledWith("archive");
  });

  it("calls onDeleteClick when delete is clicked (owner)", async () => {
    const user = userEvent.setup();

    render(
      <BulkActionsBar
        selectedIds={["p1"]}
        role="owner"
        storeId="s1"
        categories={categories}
        onActionComplete={onActionComplete}
        onCopyClick={onCopyClick}
        onDeleteClick={onDeleteClick}
        onCategoryAssignClick={onCategoryAssignClick}
        onBulkAction={onBulkAction}
      />,
    );

    await user.click(screen.getByRole("button", { name: /^delete$/i }));
    expect(onDeleteClick).toHaveBeenCalled();
  });
});
