import { describe, it, expect } from "vitest";
import { validateDeepLink, extractDeepLinkRoute } from "../validator";

describe("validateDeepLink", () => {
  describe("allowed routes", () => {
    it("accepts account/orders/{uuid}", () => {
      expect(
        validateDeepLink("account/orders/550e8400-e29b-41d4-a716-446655440000"),
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

    it("accepts browse/product/{handle}", () => {
      expect(validateDeepLink("browse/product/my-cool-sneakers")).toBe(true);
    });

    it("accepts browse/category/{slug}", () => {
      expect(validateDeepLink("browse/category/mens-shoes")).toBe(true);
    });

    it("accepts paths with a leading slash", () => {
      expect(validateDeepLink("/account/loyalty")).toBe(true);
    });
  });

  describe("extractDeepLinkRoute — correct screen and params", () => {
    it("maps account/orders/{uuid} correctly", () => {
      const result = extractDeepLinkRoute(
        "account/orders/550e8400-e29b-41d4-a716-446655440000",
      );
      expect(result).toEqual({
        screen: "account/orders/[id]",
        params: { id: "550e8400-e29b-41d4-a716-446655440000" },
      });
    });

    it("maps account/loyalty correctly", () => {
      expect(extractDeepLinkRoute("account/loyalty")).toEqual({
        screen: "account/loyalty",
        params: {},
      });
    });

    it("maps account/wishlist correctly", () => {
      expect(extractDeepLinkRoute("account/wishlist")).toEqual({
        screen: "account/wishlist",
        params: {},
      });
    });

    it("maps account/reviews correctly", () => {
      expect(extractDeepLinkRoute("account/reviews")).toEqual({
        screen: "account/reviews",
        params: {},
      });
    });

    it("maps browse/product/{handle} correctly", () => {
      expect(extractDeepLinkRoute("browse/product/blue-canvas-tote")).toEqual({
        screen: "browse/product/[handle]",
        params: { handle: "blue-canvas-tote" },
      });
    });

    it("maps browse/category/{slug} correctly", () => {
      expect(extractDeepLinkRoute("browse/category/womens-bags")).toEqual({
        screen: "browse/category/[slug]",
        params: { slug: "womens-bags" },
      });
    });
  });

  describe("path traversal rejection", () => {
    it("rejects paths containing ..", () => {
      expect(validateDeepLink("account/../orders")).toBe(false);
    });

    it("rejects paths with .. at the start", () => {
      expect(validateDeepLink("../account/loyalty")).toBe(false);
    });

    it("rejects paths with .. in segment", () => {
      expect(
        validateDeepLink("browse/product/..550e8400-e29b-41d4-a716-446655440000"),
      ).toBe(false);
    });
  });

  describe("empty and malformed paths", () => {
    it("rejects empty string", () => {
      expect(validateDeepLink("")).toBe(false);
    });

    it("rejects whitespace-only string", () => {
      expect(validateDeepLink("   ")).toBe(false);
    });

    it("rejects bare slash", () => {
      expect(validateDeepLink("/")).toBe(false);
    });

    it("rejects double slashes", () => {
      expect(validateDeepLink("account//loyalty")).toBe(false);
    });
  });

  describe("unknown routes", () => {
    it("rejects unknown top-level segment", () => {
      expect(validateDeepLink("settings/profile")).toBe(false);
    });

    it("rejects partial known path without required segment", () => {
      expect(validateDeepLink("account")).toBe(false);
    });

    it("rejects browse without sub-path", () => {
      expect(validateDeepLink("browse")).toBe(false);
    });

    it("rejects too many segments", () => {
      expect(
        validateDeepLink(
          "account/orders/550e8400-e29b-41d4-a716-446655440000/extra",
        ),
      ).toBe(false);
    });
  });

  describe("malformed UUIDs in account/orders", () => {
    it("rejects non-UUID id", () => {
      expect(validateDeepLink("account/orders/not-a-uuid")).toBe(false);
    });

    it("rejects UUID with wrong format (missing hyphens)", () => {
      expect(
        validateDeepLink("account/orders/550e8400e29b41d4a716446655440000"),
      ).toBe(false);
    });

    it("rejects UUID with wrong segment count", () => {
      expect(validateDeepLink("account/orders/550e8400-e29b-41d4")).toBe(false);
    });

    it("rejects UUID with non-hex characters", () => {
      expect(
        validateDeepLink("account/orders/gggggggg-e29b-41d4-a716-446655440000"),
      ).toBe(false);
    });
  });

  describe("malformed handles and slugs", () => {
    it("rejects product handle with uppercase letters", () => {
      expect(validateDeepLink("browse/product/My-Product")).toBe(false);
    });

    it("rejects product handle starting with a hyphen", () => {
      expect(validateDeepLink("browse/product/-bad-handle")).toBe(false);
    });

    it("rejects category slug with special characters", () => {
      expect(validateDeepLink("browse/category/mens_shoes")).toBe(false);
    });
  });
});
