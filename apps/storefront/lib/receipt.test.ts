import { describe, expect, it } from "vitest";

import { receiptView, type Receipt } from "./receipt";

const base: Receipt = {
  order_number: "M-THE-261115-00451",
  placed_at: "2026-11-15T03:20:00Z",
  currency_code: "AUD",
  lines: [
    { title: "Sandstone Ceramic Mug", image_url: "https://img/mug.jpg", quantity: 1, unit_price: "45", line_total: "45" },
    { title: "Hand-painted tile", quantity: 2, unit_price: "15", line_total: "30", option_summary: "Blue" },
  ],
  subtotal: "75",
  discount_total: "5",
  tax_total: "7.5",
  grand_total: "77.5",
  tax_label: "GST included",
  fresh: true,
  payment_method: "cash",
  receipt_email_masked: "j***@example.com",
};

describe("receiptView", () => {
  it("lists each line with its summary, quantity and price", () => {
    const v = receiptView(base);
    expect(v.lines[0]).toMatchObject({ title: "Sandstone Ceramic Mug", imageUrl: "https://img/mug.jpg", detail: "Qty 1", price: "$45.00" });
    expect(v.lines[1]).toMatchObject({ title: "Hand-painted tile", imageUrl: null, detail: "Blue · Qty 2", price: "$30.00" });
  });

  it("totals: subtotal, discount when there is one, total, and GST included", () => {
    expect(receiptView(base).totals).toEqual([
      ["Subtotal", "$75.00"],
      ["Discount", "−$5.00"],
      ["Total", "$77.50"],
      ["GST included", "$7.50"],
    ]);
    expect(receiptView({ ...base, discount_total: "0" }).totals.map(([k]) => k)).not.toContain("Discount");
  });

  it("a fresh link shows how it was paid and where it was sent", () => {
    expect(receiptView(base).contact).toEqual([
      ["Paid by", "Cash"],
      ["Sent to", "j***@example.com"],
    ]);
    expect(receiptView({ ...base, payment_method: "other", payment_method_label: "UPI" }).contact[0]).toEqual(["Paid by", "UPI"]);
  });

  it("an old link hides them and offers a new one", () => {
    const v = receiptView({ ...base, fresh: false, payment_method: undefined, receipt_email_masked: undefined });
    expect(v.contact).toEqual([]);
    expect(v.offerNewLink).toBe(true);
  });

  it("says In-store purchase with the date and time where the shop is", () => {
    // 03:20 UTC is 2:20 pm in Sydney (AEDT).
    expect(receiptView({ ...base, time_zone: "Australia/Sydney" }).placed).toMatch(/^In-store purchase · 15 Nov 2026, 2:20\s?pm$/i);
  });
});
