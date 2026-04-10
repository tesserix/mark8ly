import { describe, it, expect } from "vitest";
import { validateDeepLink, extractDeepLinkRoute } from "../validator";

describe("validateDeepLink", () => {
  // Valid routes
  it("accepts account/orders with a valid UUID", () => {
    expect(
      validateDeepLink(
        "account/orders/550e8400-e29b-41d4-a716-446655440000"
      )
    ).toBe(true);
  });

  it("accepts account/loyalty", () => {
    expect(validateDeepLink("account/loyalty")).toBe(true);
  });

  it("accepts account/wishlist", () => {
    expect(validateDeepLink("account/wishlist")).toBe(true);
  });

  it("accepts account/reviews", () => {
    expect(validateDeepLink("account/reviews")).toBe(true);
  });

  it("accepts browse/product/[handle]", () => {
    expect(validateDeepLink("browse/product/my-cool-product")).toBe(true);
    expect(validateDeepLink("browse/product/product-123")).toBe(true);
  });

  it("accepts browse/category/[slug]", () => {
    expect(validateDeepLink("browse/category/electronics")).toBe(true);
    expect(validateDeepLink("browse/category/summer-sale-2024")).toBe(true);
  });

  // Invalid routes
  it("rejects empty path", () => {
    expect(validateDeepLink("")).toBe(false);
  });

  it("rejects path traversal (..)", () => {
    expect(validateDeepLink("account/../etc/passwd")).toBe(false);
    expect(validateDeepLink("../secret")).toBe(false);
    expect(validateDeepLink("browse/product/../../admin")).toBe(false);
  });

  it("rejects unknown routes", () => {
    expect(validateDeepLink("admin/dashboard")).toBe(false);
    expect(validateDeepLink("settings/danger")).toBe(false);
    expect(validateDeepLink("browse/orders/123")).toBe(false);
  });

  it("rejects malformed UUID in account/orders", () => {
    expect(validateDeepLink("account/orders/not-a-uuid")).toBe(false);
    expect(validateDeepLink("account/orders/12345")).toBe(false);
    expect(validateDeepLink("account/orders/")).toBe(false);
  });

  it("rejects account/orders without an ID", () => {
    expect(validateDeepLink("account/orders")).toBe(false);
  });

  it("rejects handle/slug with path separators", () => {
    expect(validateDeepLink("browse/product/foo/bar")).toBe(false);
    expect(validateDeepLink("browse/category/foo/bar")).toBe(false);
  });
});

describe("extractDeepLinkRoute", () => {
  it("returns route name for account/loyalty", () => {
    const result = extractDeepLinkRoute("account/loyalty");
    expect(result).toEqual({ route: "account/loyalty", params: {} });
  });

  it("returns route name and orderId for account/orders/[uuid]", () => {
    const uuid = "550e8400-e29b-41d4-a716-446655440000";
    const result = extractDeepLinkRoute(`account/orders/${uuid}`);
    expect(result).toEqual({
      route: "account/orders/[id]",
      params: { id: uuid },
    });
  });

  it("returns route name and handle for browse/product/[handle]", () => {
    const result = extractDeepLinkRoute("browse/product/cool-shirt");
    expect(result).toEqual({
      route: "browse/product/[handle]",
      params: { handle: "cool-shirt" },
    });
  });

  it("returns route name and slug for browse/category/[slug]", () => {
    const result = extractDeepLinkRoute("browse/category/clothing");
    expect(result).toEqual({
      route: "browse/category/[slug]",
      params: { slug: "clothing" },
    });
  });

  it("returns null for invalid paths", () => {
    expect(extractDeepLinkRoute("")).toBeNull();
    expect(extractDeepLinkRoute("../evil")).toBeNull();
    expect(extractDeepLinkRoute("unknown/route")).toBeNull();
  });
});
