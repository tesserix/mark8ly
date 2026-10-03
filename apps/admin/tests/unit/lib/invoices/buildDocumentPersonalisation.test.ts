/**
 * buildDocument personalisation mapping (#968).
 *
 * The invoice is the document that travels with the goods, so it doubles
 * as the production brief for whoever packs the box. Two things have to
 * hold or that brief misleads them:
 *
 *   1. an upload prints its reference code, never the buyer's filename.
 *      "IMG_4821.jpg" identifies nothing on a printed sheet; the code is
 *      what matches the downloaded file.
 *   2. a text answer prints its value verbatim, and gets no reference —
 *      a code beside an engraving implies a file that does not exist.
 */

import { describe, it, expect } from "vitest";

import { buildDocument } from "@/lib/invoices/build";
import type {
  AdminOrder,
  AdminOrderItem,
  AdminOrderPersonalisation,
} from "@/lib/api/marketplace-api";
import type { Store } from "@/lib/api/marketplace-api";

const IMAGE_ID = "33333333-3333-3333-3333-333333333333";

function answer(
  over: Partial<AdminOrderPersonalisation>,
): AdminOrderPersonalisation {
  return {
    id: "x",
    field_key: "k",
    field_label: "Label",
    kind: "text",
    price_delta: "0.00",
    position: 0,
    has_artwork: false,
    reference: "refcode1",
    ...over,
  };
}

function line(personalisation?: AdminOrderPersonalisation[]): AdminOrderItem {
  return {
    id: "item-1",
    title_snapshot: "Custom figurine",
    sku_snapshot: "FIG-1",
    unit_price: "80.00",
    quantity: 1,
    line_total: "80.00",
    currency_code: "USD",
    personalisation,
  };
}

function order(items: AdminOrderItem[]): AdminOrder {
  return {
    id: "22222222-2222-2222-2222-222222222222",
    order_number: "M-TST-1",
    status: "confirmed",
    payment_status: "paid",
    fulfillment_status: "unfulfilled",
    customer_email: "buyer@example.com",
    customer_name: "A Buyer",
    placed_at: "2026-10-01T00:00:00Z",
    subtotal: "80.00",
    shipping_total: "0.00",
    tax_total: "0.00",
    discount_total: "0.00",
    grand_total: "80.00",
    refunded_amount: "0.00",
    currency_code: "USD",
    items,
    addresses: [
      {
        kind: "shipping",
        name: "A Buyer",
        line1: "1 Main St",
        city: "Dublin",
        country_code: "IE",
      },
    ],
  } as unknown as AdminOrder;
}

const store = {
  id: "11111111-1111-1111-1111-111111111111",
  name: "Test Store",
  slug: "test-store",
  country_code: "IE",
} as unknown as Store;

function build(items: AdminOrderItem[]) {
  return buildDocument({
    kind: "invoice",
    order: order(items),
    branding: null,
    store,
    documentNumber: "INV-TST-260101-00001",
  });
}

describe("buildDocument personalisation", () => {
  it("prints the reference code for an upload, not the filename", () => {
    const doc = build([
      line([
        answer({
          id: IMAGE_ID,
          kind: "image",
          field_label: "Your photo",
          has_artwork: true,
          original_filename: "IMG_4821.jpg",
          reference: IMAGE_ID.slice(0, 8),
        }),
      ]),
    ]);

    const pz = doc.lines[0]?.personalisation;
    expect(pz).toHaveLength(1);
    expect(pz?.[0]?.label).toBe("Your photo");
    expect(pz?.[0]?.reference).toBe(IMAGE_ID.slice(0, 8));
    // No value: a filename on a printed sheet identifies nothing.
    expect(pz?.[0]?.value).toBeUndefined();
    expect(JSON.stringify(doc)).not.toContain("IMG_4821.jpg");
  });

  it("prints a text answer verbatim with no reference", () => {
    const doc = build([
      line([
        answer({
          field_label: "Name to engrave",
          text_value: "Asha",
        }),
      ]),
    ]);

    const pz = doc.lines[0]?.personalisation;
    expect(pz?.[0]).toEqual({
      label: "Name to engrave",
      value: "Asha",
      reference: undefined,
    });
  });

  it("orders answers by position", () => {
    const doc = build([
      line([
        answer({ id: "b", position: 9, field_label: "Second", text_value: "b" }),
        answer({ id: "a", position: 2, field_label: "First", text_value: "a" }),
      ]),
    ]);
    expect(doc.lines[0]?.personalisation?.map((p) => p.label)).toEqual([
      "First",
      "Second",
    ]);
  });

  it("leaves ordinary lines with an empty list", () => {
    const doc = build([line()]);
    expect(doc.lines[0]?.personalisation).toEqual([]);
  });
});
