import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { InStoreProduct } from "@/lib/api/instore-api";

const quoteSaleAction = vi.fn();
const recordSaleAction = vi.fn();

vi.mock("@/app/(admin)/sell/actions", () => ({
  quoteSaleAction: (...args: unknown[]) => quoteSaleAction(...args),
  recordSaleAction: (...args: unknown[]) => recordSaleAction(...args),
}));

import { SaleScreen } from "./SaleScreen";

const catalog: InStoreProduct[] = [
  {
    id: "p1",
    title: "BMW M4 Laptop Skin",
    variants: [{ id: "v1", price: "40", currency_code: "AUD", available: 5, inventory_policy: "deny" }],
  },
  {
    id: "p2",
    title: "Linen Shirt",
    variants: [
      { id: "v2", option_summary: "S", price: "149", currency_code: "AUD", available: 3, inventory_policy: "deny" },
      { id: "v3", option_summary: "M", price: "149", currency_code: "AUD", available: 0, inventory_policy: "deny" },
    ],
  },
];

function quote(grand: string) {
  return { ok: true, data: { subtotal: grand, tax_total: "0.00", discount_total: "0.00", grand_total: grand, currency_code: "AUD" } };
}

function renderScreen(props: Partial<Parameters<typeof SaleScreen>[0]> = {}) {
  return render(<SaleScreen storeId="s1" currencyCode="AUD" catalog={catalog} canDiscount {...props} />);
}

async function ringUpSkinAndCharge(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /BMW M4 Laptop Skin/ }));
  await user.click(screen.getByRole("button", { name: /^Charge/ }));
  await screen.findByText(/Charge A\$44\.00/);
}

beforeEach(() => {
  localStorage.clear();
  quoteSaleAction.mockReset().mockResolvedValue(quote("44.00"));
  recordSaleAction.mockReset();
});

describe("SaleScreen", () => {
  it("filters the grid by search", async () => {
    const user = userEvent.setup();
    renderScreen();
    await user.type(screen.getByRole("searchbox", { name: /search products/i }), "bmw");
    expect(screen.getByRole("button", { name: /BMW M4 Laptop Skin/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Linen Shirt/ })).not.toBeInTheDocument();
  });

  it("picks a variant when a product has several", async () => {
    const user = userEvent.setup();
    renderScreen();
    await user.click(screen.getByRole("button", { name: /Linen Shirt/ }));
    expect(screen.getByRole("button", { name: /^M.*Sold out/ })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: /^S\b/ }));
    const cart = screen.getByRole("region", { name: /cart/i });
    expect(within(cart).getByText("Linen Shirt")).toBeInTheDocument();
    expect(within(cart).getByText("S")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /BMW M4 Laptop Skin/ }));
    expect(within(cart).getByText("BMW M4 Laptop Skin")).toBeInTheDocument();
  });

  it("hides discount and custom item without permission", () => {
    renderScreen({ canDiscount: false });
    expect(screen.queryByLabelText(/discount/i)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /custom item/i })).not.toBeInTheDocument();
  });

  it("adds a custom item", async () => {
    const user = userEvent.setup();
    renderScreen();
    await user.click(screen.getByRole("button", { name: /custom item/i }));
    await user.type(screen.getByLabelText(/item name/i), "Gift wrap");
    await user.type(screen.getByLabelText(/^price/i), "5");
    await user.click(screen.getByRole("button", { name: /add item/i }));
    const cart = screen.getByRole("region", { name: /cart/i });
    expect(within(cart).getByText("Gift wrap")).toBeInTheDocument();
    expect(within(cart).queryByLabelText(/price/i)).not.toBeInTheDocument();
  });

  it("charge shows the server total", async () => {
    const user = userEvent.setup();
    renderScreen();
    await ringUpSkinAndCharge(user);
    expect(quoteSaleAction).toHaveBeenCalledWith("s1", { lines: [{ variant_id: "v1", quantity: 1 }], discount: undefined });
  });

  it("cash shows change and needs enough tendered", async () => {
    const user = userEvent.setup();
    renderScreen();
    await ringUpSkinAndCharge(user);
    await user.click(screen.getByRole("radio", { name: /cash/i }));
    await user.type(screen.getByLabelText(/cash tendered/i), "50");
    expect(screen.getByText(/Change A\$6\.00/)).toBeInTheDocument();
    await user.clear(screen.getByLabelText(/cash tendered/i));
    await user.type(screen.getByLabelText(/cash tendered/i), "40");
    expect(screen.getByRole("button", { name: /^paid/i })).toBeDisabled();
  });

  it("shows paid only after the server confirms", async () => {
    const user = userEvent.setup();
    let resolve!: (v: unknown) => void;
    recordSaleAction.mockReturnValue(new Promise((r) => (resolve = r)));
    renderScreen();
    await ringUpSkinAndCharge(user);
    await user.click(screen.getByRole("button", { name: /^paid/i }));
    expect(screen.queryByText(/BON-1001/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /new sale/i })).not.toBeInTheDocument();

    resolve({ ok: true, orderId: "o1", orderNumber: "BON-1001", grandTotal: "44.00", reused: false });
    expect(await screen.findByText(/BON-1001/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /new sale/i })).toBeInTheDocument();
    expect(recordSaleAction.mock.calls[0]![1]).toMatchObject({ expected_total: "44.00", payment_method: "card_external" });
  });

  it("keeps the cart and says not recorded when unreachable", async () => {
    const user = userEvent.setup();
    recordSaleAction.mockResolvedValue({ ok: false, kind: "unreachable" });
    renderScreen();
    await ringUpSkinAndCharge(user);
    await user.click(screen.getByRole("button", { name: /^paid/i }));
    expect(await screen.findByText(/No connection, sale not recorded yet/)).toBeInTheDocument();
    const cart = screen.getByRole("region", { name: /cart/i });
    expect(within(cart).getByText("BMW M4 Laptop Skin")).toBeInTheDocument();
    expect(within(cart).queryByRole("button", { name: /remove/i })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /try again/i }));
    await waitFor(() => expect(recordSaleAction).toHaveBeenCalledTimes(2));
    const [first, second] = recordSaleAction.mock.calls.map((c) => c[1].idempotency_key);
    expect(second).toBe(first);
  });

  it("lets the seller pick cash when retrying a sale restored after reload", async () => {
    const user = userEvent.setup();
    localStorage.setItem(
      "mark8ly:instore-cart:s1",
      JSON.stringify({
        lines: [{ key: "k", variantId: "v1", title: "BMW M4 Laptop Skin", unitPrice: "40", quantity: 1 }],
        discount: "", note: "", idempotencyKey: "key-1", locked: true,
      }),
    );
    recordSaleAction.mockResolvedValue({ ok: true, orderId: "o1", orderNumber: "BON-1001", grandTotal: "44.00", reused: true });
    renderScreen();
    await user.click(screen.getByRole("button", { name: /^Charge/ }));
    await screen.findByText(/Charge A\$44\.00/);
    await user.click(screen.getByRole("radio", { name: /cash/i }));
    await user.type(screen.getByLabelText(/cash tendered/i), "50");
    await user.click(screen.getByRole("button", { name: /try again/i }));
    await screen.findByText(/BON-1001/);
    expect(recordSaleAction.mock.calls[0]![1]).toMatchObject({
      idempotency_key: "key-1", payment_method: "cash", cash_tendered: "50",
    });
  });

  it("keeps a possibly-recorded sale locked when the retry is refused", async () => {
    const user = userEvent.setup();
    recordSaleAction
      .mockResolvedValueOnce({ ok: false, kind: "unreachable" })
      .mockResolvedValueOnce({ ok: false, kind: "rejected", message: "this product is not for sale in this store" });
    renderScreen();
    await ringUpSkinAndCharge(user);
    await user.click(screen.getByRole("button", { name: /^paid/i }));
    await user.click(await screen.findByRole("button", { name: /try again/i }));
    expect(await screen.findByText(/not for sale/)).toBeInTheDocument();
    const cart = screen.getByRole("region", { name: /cart/i });
    expect(within(cart).queryByRole("button", { name: /remove/i })).not.toBeInTheDocument();
    expect(within(cart).getByRole("button", { name: /discard this sale/i })).toBeInTheDocument();
    expect(JSON.parse(localStorage.getItem("mark8ly:instore-cart:s1") ?? "{}").idempotencyKey).toBeTruthy();
  });

  it("names the line when out of stock", async () => {
    const user = userEvent.setup();
    recordSaleAction.mockResolvedValue({ ok: false, kind: "out_of_stock", variantId: "v1" });
    renderScreen();
    await ringUpSkinAndCharge(user);
    await user.click(screen.getByRole("button", { name: /^paid/i }));
    const cart = screen.getByRole("region", { name: /cart/i });
    expect(await within(cart).findByText(/Only 5 left/)).toBeInTheDocument();
    expect(within(cart).getByRole("button", { name: /remove/i })).toBeInTheDocument();
  });

  it("shows the new total when the server refuses a changed total", async () => {
    const user = userEvent.setup();
    recordSaleAction.mockResolvedValue({ ok: false, kind: "total_changed", grandTotal: "46.00" });
    renderScreen();
    await ringUpSkinAndCharge(user);
    await user.click(screen.getByRole("button", { name: /^paid/i }));
    expect(await screen.findByText("The total changed to A$46.00. Charge that amount instead.")).toBeInTheDocument();
    expect(screen.getByText(/Charge A\$46\.00/)).toBeInTheDocument();
  });

  it("restores the cart after reload", () => {
    localStorage.setItem(
      "mark8ly:instore-cart:s1",
      JSON.stringify({
        lines: [{ key: "k", variantId: "v1", title: "BMW M4 Laptop Skin", unitPrice: "40", quantity: 2 }],
        discount: "", note: "", idempotencyKey: null, locked: false,
      }),
    );
    renderScreen();
    const cart = screen.getByRole("region", { name: /cart/i });
    expect(within(cart).getByText("BMW M4 Laptop Skin")).toBeInTheDocument();
  });

  it("new sale clears the cart", async () => {
    const user = userEvent.setup();
    recordSaleAction.mockResolvedValue({ ok: true, orderId: "o1", orderNumber: "BON-1001", grandTotal: "44.00", reused: false });
    renderScreen();
    await ringUpSkinAndCharge(user);
    await user.click(screen.getByRole("button", { name: /^paid/i }));
    await user.click(await screen.findByRole("button", { name: /new sale/i }));
    const cart = screen.getByRole("region", { name: /cart/i });
    expect(within(cart).queryByText("BMW M4 Laptop Skin")).not.toBeInTheDocument();
    expect(JSON.parse(localStorage.getItem("mark8ly:instore-cart:s1") ?? "{}").lines).toEqual([]);
  });

  it("switch person links to /logout", () => {
    renderScreen();
    expect(screen.getByRole("link", { name: /switch person/i })).toHaveAttribute("href", "/logout");
  });
});
