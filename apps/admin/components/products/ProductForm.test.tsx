import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ProductForm } from "./ProductForm";
import type { AdminCategory, AdminProduct } from "@/lib/api/marketplace-api";

vi.mock("@/app/(admin)/products/actions", () => ({
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

// The form was five tabs. It is one scrolling page of hairline-separated
// sections now: tabs hid whether a section held anything until you clicked
// it, and the tab bar changed shape between create and edit because Media
// cannot work before a product exists — a sign the information
// architecture was wrong rather than that Media needed a placeholder.
describe("ProductForm — one page, no tabs", () => {
  describe("everything is present without navigating", () => {
    it("renders the core fields on mount", () => {
      render(<ProductForm {...baseProps} />);
      expect(screen.getByText(/^Title$/i)).toBeInTheDocument();
      expect(screen.getByText(/^Handle$/i)).toBeInTheDocument();
      expect(screen.getByText(/^Description$/i)).toBeInTheDocument();
      expect(screen.getByText(/^Status$/i)).toBeInTheDocument();
      expect(screen.getByText(/^Price/i)).toBeInTheDocument();
      expect(screen.getByText(/^Stock$/i)).toBeInTheDocument();
    });

    it("has no tabs at all", () => {
      render(<ProductForm {...baseProps} />);
      expect(screen.queryAllByRole("tab")).toHaveLength(0);
      expect(screen.queryByRole("tablist")).toBeNull();
    });

    it("shows options and shipping without a click", () => {
      render(<ProductForm {...baseProps} />);
      expect(
        screen.getByRole("button", { name: /add option/i }),
      ).toBeInTheDocument();
      expect(screen.getByText(/^Weight \(kg\)$/i)).toBeInTheDocument();
    });

    it("names its sections as headings, so the page is navigable by structure", () => {
      render(<ProductForm {...baseProps} />);
      for (const name of [/^Details$/, /^Options$/, /^Shipping$/]) {
        expect(screen.getByRole("heading", { name })).toBeInTheDocument();
      }
    });
  });

  // Media needs a product to attach to. It is ABSENT on create rather than
  // present-and-disabled: a section that cannot work yet is worse than one
  // that is not there, and a disabled tab was what made the chrome change
  // shape between modes.
  describe("Media", () => {
    it("is absent in create mode", () => {
      render(<ProductForm {...baseProps} />);
      expect(screen.queryByTestId("media-tab-stub")).toBeNull();
      expect(screen.queryByRole("heading", { name: /^Media$/ })).toBeNull();
    });

    it("is present, unprompted, in edit mode", () => {
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
      expect(screen.getByTestId("media-tab-stub")).toBeInTheDocument();
    });
  });

  // The sentence "This product has variants. Price and stock live in the
  // Variants tab." existed only to paper over a seam: price and stock had
  // two homes and one of them got hidden. One adaptive section replaced
  // both, so nothing needs explaining.
  describe("pricing has one home", () => {
    it("never tells the merchant where their price went", () => {
      const product: Partial<AdminProduct> = {
        id: "p1",
        title: "T",
        handle: "t",
        description: "",
        status: "draft",
        categories: [],
        variants: [
          { id: "v1", sku: "A", price: "1", option_values: [] },
          { id: "v2", sku: "B", price: "2", option_values: [] },
        ] as unknown as AdminProduct["variants"],
      };
      render(
        <ProductForm
          {...baseProps}
          mode="edit"
          initialProduct={product as AdminProduct}
        />,
      );
      expect(screen.queryByText(/live in the Variants tab/i)).toBeNull();
    });
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
