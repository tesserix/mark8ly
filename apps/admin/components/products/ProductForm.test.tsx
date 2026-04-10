import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ProductForm } from "./ProductForm";
import type { AdminCategory, AdminProduct } from "@/lib/api/marketplace-api";

vi.mock("@/app/products/actions", () => ({
  createProductAction: vi.fn(async () => ({ ok: true, data: { id: "p1" } })),
  updateProductAction: vi.fn(async () => ({ ok: true, data: { id: "p1" } })),
  deleteProductAction: vi.fn(async () => ({ ok: true })),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }),
}));

// MediaTab pulls in upload client / fetch; stub it so create-mode rendering is deterministic.
vi.mock("./form/MediaTab", () => ({
  MediaTab: () => <div data-testid="media-tab-stub">drop images here</div>,
}));

const categories: AdminCategory[] = [
  { id: "00000000-0000-0000-0000-000000000001", name: "Apparel", slug: "apparel" } as unknown as AdminCategory,
];

const baseProps = {
  mode: "create" as const,
  storeId: "s1",
  categories,
  currencyCode: "USD",
  canDelete: false,
  canArchive: false,
  session: { userId: "u1", tenantId: "t1" },
};

describe("ProductForm (Task 10 tab shell)", () => {
  describe("M7b General tab contract", () => {
    it("renders core M7b fields on the General tab by default", () => {
      render(<ProductForm {...baseProps} />);
      expect(screen.getByText(/^Title$/i)).toBeInTheDocument();
      expect(screen.getByText(/^Handle$/i)).toBeInTheDocument();
      expect(screen.getByText(/^Description$/i)).toBeInTheDocument();
      expect(screen.getByText(/^Status$/i)).toBeInTheDocument();
      expect(screen.getByText(/^Price/i)).toBeInTheDocument();
      expect(screen.getByText(/^Stock$/i)).toBeInTheDocument();
    });

    it("defaults to the General tab on mount", () => {
      render(<ProductForm {...baseProps} />);
      const generalTab = screen.getByRole("tab", { name: /general/i });
      expect(generalTab).toHaveAttribute("aria-selected", "true");
    });
  });

  describe("tab switching", () => {
    it("switches to Options tab and renders OptionsEditor add button", () => {
      render(<ProductForm {...baseProps} />);
      fireEvent.click(screen.getByRole("tab", { name: /options/i }));
      expect(screen.getByRole("button", { name: /add option/i })).toBeInTheDocument();
    });

    it("switches to Variants tab and renders empty state when no options set", () => {
      render(<ProductForm {...baseProps} />);
      fireEvent.click(screen.getByRole("tab", { name: /variants/i }));
      expect(
        screen.getByText(/add options on the options tab/i),
      ).toBeInTheDocument();
    });

    it("switches to Media tab in edit mode", () => {
      const product: Partial<AdminProduct> = {
        id: "p1",
        title: "T",
        handle: "t",
        description: "",
        status: "draft",
        variants: [],
        categories: [],
      };
      render(
        <ProductForm
          {...baseProps}
          mode="edit"
          initialProduct={product as AdminProduct}
        />,
      );
      fireEvent.click(screen.getByRole("tab", { name: /media/i }));
      expect(screen.getByTestId("media-tab-stub")).toBeInTheDocument();
    });
  });

  describe("infinite loop safety", () => {
    it("does not infinite-loop when options are preset on mount", () => {
      const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
      const product: Partial<AdminProduct> = {
        id: "p1",
        title: "T",
        handle: "t",
        description: "",
        status: "draft",
        variants: [],
        categories: [],
      };
      render(
        <ProductForm
          {...baseProps}
          mode="edit"
          initialProduct={product as AdminProduct}
        />,
      );
      const looped = consoleError.mock.calls.some((call) =>
        String(call[0] ?? "").match(/Maximum update depth|Too many re-renders/),
      );
      expect(looped).toBe(false);
      consoleError.mockRestore();
    });
  });
});
