import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";

// Mock @tesserix/web Dialog — vitest can't resolve the ESM barrel export.
// Render children directly so we can test the dialog content in isolation.
vi.mock("@tesserix/web", () => ({
  Dialog: ({ open, children }: { open?: boolean; children: ReactNode }) =>
    open ? <div data-testid="dialog">{children}</div> : null,
  DialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children, className }: { children: ReactNode; className?: string }) => (
    <h2 className={className}>{children}</h2>
  ),
  DialogDescription: ({ children, className }: { children: ReactNode; className?: string }) => (
    <p className={className}>{children}</p>
  ),
  DialogFooter: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogClose: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

import { CopyToStoreDialog } from "./CopyToStoreDialog";
import type { AdminStore } from "@/lib/api/marketplace-api";

const stores: AdminStore[] = [
  { id: "s1", name: "US Store", slug: "us-store", country_code: "US", currency_code: "USD", status: "active" },
  { id: "s2", name: "UK Store", slug: "uk-store", country_code: "GB", currency_code: "GBP", status: "active" },
  { id: "s3", name: "DE Store", slug: "de-store", country_code: "DE", currency_code: "EUR", status: "active" },
];

describe("CopyToStoreDialog", () => {
  const onCopy = vi.fn();
  const onClose = vi.fn();

  beforeEach(() => {
    onCopy.mockReset();
    onClose.mockReset();
  });

  it("renders available stores excluding the current store", () => {
    render(
      <CopyToStoreDialog
        isOpen
        onClose={onClose}
        stores={stores}
        currentStoreId="s1"
        productIds={["p1"]}
        onCopy={onCopy}
      />,
    );

    expect(screen.getByText("UK Store")).toBeInTheDocument();
    expect(screen.getByText("DE Store")).toBeInTheDocument();
    expect(screen.queryByText("US Store")).not.toBeInTheDocument();
  });

  it("calls onCopy with selected store and copy_media flag on submit", async () => {
    const user = userEvent.setup();
    onCopy.mockResolvedValue(undefined);

    render(
      <CopyToStoreDialog
        isOpen
        onClose={onClose}
        stores={stores}
        currentStoreId="s1"
        productIds={["p1"]}
        onCopy={onCopy}
      />,
    );

    await user.click(screen.getByLabelText("UK Store"));
    await user.click(screen.getByRole("button", { name: /copy/i }));

    expect(onCopy).toHaveBeenCalledWith({
      targetStoreId: "s2",
      copyMedia: true,
      productIds: ["p1"],
    });
  });

  it("shows bulk mode title when multiple product ids are passed", () => {
    render(
      <CopyToStoreDialog
        isOpen
        onClose={onClose}
        stores={stores}
        currentStoreId="s1"
        productIds={["p1", "p2", "p3"]}
        onCopy={onCopy}
      />,
    );

    expect(screen.getByText(/copy 3 products/i)).toBeInTheDocument();
  });

  it("disables submit when no store is selected", () => {
    render(
      <CopyToStoreDialog
        isOpen
        onClose={onClose}
        stores={stores}
        currentStoreId="s1"
        productIds={["p1"]}
        onCopy={onCopy}
      />,
    );

    const submitBtn = screen.getByRole("button", { name: /^copy$/i });
    expect(submitBtn).toBeDisabled();
  });

  it("allows toggling copy media off", async () => {
    const user = userEvent.setup();
    onCopy.mockResolvedValue(undefined);

    render(
      <CopyToStoreDialog
        isOpen
        onClose={onClose}
        stores={stores}
        currentStoreId="s1"
        productIds={["p1"]}
        onCopy={onCopy}
      />,
    );

    await user.click(screen.getByLabelText("UK Store"));
    const toggle = screen.getByRole("checkbox", { name: /copy media/i });
    await user.click(toggle);
    await user.click(screen.getByRole("button", { name: /copy/i }));

    expect(onCopy).toHaveBeenCalledWith({
      targetStoreId: "s2",
      copyMedia: false,
      productIds: ["p1"],
    });
  });
});
