import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { AdminCategory } from "@/lib/api/marketplace-api";

const createProductAction = vi.fn(async () => ({ ok: true, data: { id: "p1" } }));
const push = vi.fn();

vi.mock("@/app/(admin)/products/actions", () => ({
  createProductAction: (...args: unknown[]) => createProductAction(...(args as [])),
  updateProductAction: vi.fn(),
  deleteProductAction: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, refresh: vi.fn() }),
}));
vi.mock("./form/MediaTab", () => ({
  MediaTab: () => <div data-testid="media-tab-stub" />,
}));

import { ProductForm } from "./ProductForm";

const categories: AdminCategory[] = [];

const baseProps = {
  storeId: "s1",
  categories,
  currencyCode: "AUD",
  storeCountryCode: "AU",
  canDelete: false,
  canArchive: false,
  session: { userId: "u1", tenantId: "t1" },
};

beforeEach(() => vi.clearAllMocks());

// Create used to render the identical edit page, including a Media section
// that cannot work before the product exists. It is now a short form: the
// few things needed to make the product real, then a redirect into edit
// where the rest has room.
describe("ProductForm — the create form", () => {
  it("asks only what is needed to make the product exist", () => {
    render(<ProductForm {...baseProps} mode="create" />);

    expect(screen.getByText(/^Title$/i)).toBeInTheDocument();
    expect(screen.getByText(/^Categories$/i)).toBeInTheDocument();
    expect(screen.getByText(/^Price/i)).toBeInTheDocument();
    expect(screen.getByText(/^Stock$/i)).toBeInTheDocument();

    // Handle auto-generates from the title, and description has room on the
    // edit page. Asking for them here makes the form longer for nothing.
    expect(screen.queryByText(/^Handle$/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/^Description$/i)).not.toBeInTheDocument();
  });

  it("omits the sections that need a saved product", () => {
    render(<ProductForm {...baseProps} mode="create" />);
    for (const name of [/^Media$/, /^Options$/, /^Shipping$/, /^Tax$/]) {
      expect(screen.queryByRole("heading", { name })).not.toBeInTheDocument();
    }
    // Absent, not disabled: a dropzone that cannot accept a file is a
    // promise the page cannot keep.
    expect(screen.queryByTestId("media-tab-stub")).toBeNull();
  });

  it("has no rail, because there is no scroll to glance across", () => {
    render(<ProductForm {...baseProps} mode="create" />);
    expect(screen.queryByRole("complementary")).not.toBeInTheDocument();
  });

  it("is left-aligned and constrained, never centred", () => {
    const { container } = render(<ProductForm {...baseProps} mode="create" />);
    const column = container.querySelector(".max-w-2xl");
    expect(column).not.toBeNull();
    // mx-auto would centre it into the generic signup-card look the design
    // direction rules out; the empty right-hand width is the asymmetric
    // margin the system asks for.
    expect(column!.className).not.toMatch(/mx-auto/);
  });

  // Status is decided by WHICH action the merchant takes, not by a fifth
  // field asking the same question the submit button is about to answer.
  it("offers draft and publish as the two actions, with no status field", () => {
    render(<ProductForm {...baseProps} mode="create" />);
    expect(screen.getByRole("button", { name: /create as draft/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /create and publish/i })).toBeInTheDocument();
    expect(screen.queryByText(/^Status$/i)).not.toBeInTheDocument();
  });

  it("creates a draft when the draft action is used", async () => {
    const user = userEvent.setup();
    render(<ProductForm {...baseProps} mode="create" />);

    await user.type(screen.getByLabelText(/^Title$/i), "Bondi Beach Cotton Towel");
    await user.type(screen.getByLabelText(/^Price/i), "75");
    await user.click(screen.getByRole("button", { name: /create as draft/i }));

    await waitFor(() => expect(createProductAction).toHaveBeenCalled());
    const values = createProductAction.mock.calls[0]![2] as { status: string };
    expect(values.status).toBe("draft");
  });

  it("creates an active product when the publish action is used", async () => {
    const user = userEvent.setup();
    render(<ProductForm {...baseProps} mode="create" />);

    await user.type(screen.getByLabelText(/^Title$/i), "Bondi Beach Cotton Towel");
    await user.type(screen.getByLabelText(/^Price/i), "75");
    await user.click(screen.getByRole("button", { name: /create and publish/i }));

    await waitFor(() => expect(createProductAction).toHaveBeenCalled());
    const values = createProductAction.mock.calls[0]![2] as { status: string };
    expect(values.status).toBe("active");
  });
});
