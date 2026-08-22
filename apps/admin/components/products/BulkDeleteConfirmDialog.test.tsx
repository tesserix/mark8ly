import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";

vi.mock("@tesserix/web", () => ({
  AlertDialog: ({
    isOpen,
    title,
    message,
    onConfirm,
    onCancel,
    confirmLabel,
    cancelLabel,
  }: {
    isOpen: boolean;
    title: string;
    message: string;
    onConfirm?: () => void;
    onCancel?: () => void;
    confirmLabel?: string;
    cancelLabel?: string;
  }) =>
    isOpen ? (
      <div data-testid="alert-dialog">
        <h2>{title}</h2>
        <p>{message}</p>
        <button type="button" onClick={onCancel}>{cancelLabel ?? "Cancel"}</button>
        <button type="button" onClick={onConfirm}>{confirmLabel ?? "Confirm"}</button>
      </div>
    ) : null,
}));

import { BulkDeleteConfirmDialog } from "./BulkDeleteConfirmDialog";

describe("BulkDeleteConfirmDialog", () => {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the count in the confirmation message", () => {
    render(
      <BulkDeleteConfirmDialog
        isOpen
        count={5}
        onConfirm={onConfirm}
        onCancel={onCancel}
      />,
    );

    expect(screen.getByText(/5 products/)).toBeInTheDocument();
    expect(screen.getByText(/can't be undone/i)).toBeInTheDocument();
  });

  it("calls onConfirm when confirm button is clicked", async () => {
    const user = userEvent.setup();

    render(
      <BulkDeleteConfirmDialog
        isOpen
        count={3}
        onConfirm={onConfirm}
        onCancel={onCancel}
      />,
    );

    await user.click(screen.getByRole("button", { name: /delete/i }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("calls onCancel when cancel button is clicked", async () => {
    const user = userEvent.setup();

    render(
      <BulkDeleteConfirmDialog
        isOpen
        count={3}
        onConfirm={onConfirm}
        onCancel={onCancel}
      />,
    );

    await user.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("does not render when isOpen is false", () => {
    render(
      <BulkDeleteConfirmDialog
        isOpen={false}
        count={3}
        onConfirm={onConfirm}
        onCancel={onCancel}
      />,
    );

    expect(screen.queryByTestId("alert-dialog")).not.toBeInTheDocument();
  });
});
