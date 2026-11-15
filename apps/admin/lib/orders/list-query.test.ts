import { describe, expect, it } from "vitest";

import { ordersHref, parseOrdersQuery } from "./list-query";

describe("ordersHref", () => {
  it("keeps the channel when another filter is picked", () => {
    expect(ordersHref({ channel: "in_store", status: "fulfilled" }, { paymentStatus: "paid" })).toBe(
      "/orders?status=fulfilled&payment_status=paid&channel=in_store",
    );
  });

  it("keeps every filter when paging, and drops page 1", () => {
    const q = { channel: "in_store", status: "fulfilled", paymentStatus: "paid", search: "M-1" } as const;
    expect(ordersHref(q, { page: 2 })).toBe(
      "/orders?status=fulfilled&payment_status=paid&channel=in_store&search=M-1&page=2",
    );
    expect(ordersHref(q, { page: 1 })).toBe("/orders?status=fulfilled&payment_status=paid&channel=in_store&search=M-1");
  });

  it("clears a filter set to undefined, and resets the page", () => {
    expect(ordersHref({ channel: "online", page: 3 }, { channel: undefined })).toBe("/orders");
  });
});

describe("parseOrdersQuery", () => {
  it("reads a known channel", () => {
    expect(parseOrdersQuery({ channel: "in_store" }).channel).toBe("in_store");
  });

  it("ignores an unknown channel", () => {
    expect(parseOrdersQuery({ channel: "pos" }).channel).toBeUndefined();
  });
});
