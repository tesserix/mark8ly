import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";

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
];

describe("CopyToStoreDialog — bulk mode", () => {
  const onCopy = vi.fn();
  const onClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("bulk copy passes all selectedIds to onCopy", async () => {
    const user = userEvent.setup();
    onCopy.mockResolvedValue(undefined);

    const selectedIds = ["p1", "p2", "p3"];

    render(
      <CopyToStoreDialog
        isOpen
        onClose={onClose}
        stores={stores}
        currentStoreId="s1"
        productIds={selectedIds}
        onCopy={onCopy}
      />,
    );

    // Verify bulk title
    expect(screen.getByText(/copy 3 products/i)).toBeInTheDocument();

    // Select target store
    await user.click(screen.getByLabelText("UK Store"));

    // Submit
    await user.click(screen.getByRole("button", { name: /copy/i }));

    expect(onCopy).toHaveBeenCalledTimes(1);
    expect(onCopy).toHaveBeenCalledWith({
      targetStoreId: "s2",
      copyMedia: true,
      productIds: ["p1", "p2", "p3"],
    });
  });

  it("the caller can iterate over productIds to call copyProduct per id", async () => {
    // This test proves the contract: the onCopy handler receives all ids
    // and the caller is responsible for iterating and calling copyProduct
    // per id (shown in the wiring in ProductsList).
    const copyPerProduct = vi.fn();

    const handler = async (req: { targetStoreId: string; copyMedia: boolean; productIds: string[] }) => {
      for (const id of req.productIds) {
        await copyPerProduct(id, req.targetStoreId, req.copyMedia);
      }
    };

    const user = userEvent.setup();

    render(
      <CopyToStoreDialog
        isOpen
        onClose={onClose}
        stores={stores}
        currentStoreId="s1"
        productIds={["p1", "p2"]}
        onCopy={handler}
      />,
    );

    await user.click(screen.getByLabelText("UK Store"));
    await user.click(screen.getByRole("button", { name: /copy/i }));

    expect(copyPerProduct).toHaveBeenCalledTimes(2);
    expect(copyPerProduct).toHaveBeenCalledWith("p1", "s2", true);
    expect(copyPerProduct).toHaveBeenCalledWith("p2", "s2", true);
  });
});
