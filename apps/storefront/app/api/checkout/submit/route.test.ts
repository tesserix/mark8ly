import { afterEach, describe, expect, it, vi } from "vitest";

// Regression for the bug that made personalised orders unplaceable even
// after the answers were being forwarded (#966).
//
// marketplace-api's cartTokenForCheckout reads mk_cart_token off the
// request it receives and, failing that, MINTS A FRESH UUID. This proxy
// forwarded only mp_customer_session, so every checkout arrived with a
// brand new cart token that matched no upload — and the server
// truthfully reported "the image for X is no longer available" while the
// object sat in the bucket and its own preview endpoint served it fine.
//
// The cart token is the only thing proving a buyer's artwork is theirs,
// so it is also what scopes the lookup. It additionally keys stock-hold
// consumption (checkout_ext.go), so a missing cookie silently detached
// the order from the holds it was placed against.

const proxyJson = vi.fn(
  async (_url: string, _init: { headers: Record<string, string> }) =>
    new Response("{}", { status: 200 }),
);
let cookieJar: Record<string, string> = {};

vi.mock("next/headers", () => ({
  cookies: async () => ({
    get: (k: string) => (cookieJar[k] ? { value: cookieJar[k] } : undefined),
  }),
}));

vi.mock("../_proxy", () => ({
  proxyJson: (url: string, init: { headers: Record<string, string> }) =>
    proxyJson(url, init),
  marketplaceStoreUrl: (slug: string) => `https://api.test/stores/${slug}`,
  requireStoreSlug: () => ({ slug: "acme" }),
}));

function post(): Request {
  return new Request("https://shop.test/api/checkout/submit?store=acme", {
    method: "POST",
    body: JSON.stringify({ idempotency_key: "k" }),
    headers: { origin: "https://shop.test" },
  });
}

afterEach(() => {
  vi.clearAllMocks();
  cookieJar = {};
});

describe("checkout submit proxy", () => {
  it("forwards the cart token so the server can find the buyer's uploads", async () => {
    cookieJar = { mk_cart_token: "cart-123" };
    const { POST } = await import("./route");
    await POST(post());

    const init = proxyJson.mock.calls[0]![1];
    expect(init.headers.Cookie).toContain("mk_cart_token=cart-123");
  });

  it("forwards both cookies together when the shopper is signed in", async () => {
    cookieJar = { mk_cart_token: "cart-123", mp_customer_session: "sess-9" };
    const { POST } = await import("./route");
    await POST(post());

    const init = proxyJson.mock.calls[0]![1];
    // Both matter: the session stamps orders.customer_id, the cart token
    // scopes the artwork lookup. Dropping either loses something.
    expect(init.headers.Cookie).toContain("mp_customer_session=sess-9");
    expect(init.headers.Cookie).toContain("mk_cart_token=cart-123");
  });

  it("sends no Cookie header at all when there is nothing to send", async () => {
    const { POST } = await import("./route");
    await POST(post());

    const init = proxyJson.mock.calls[0]![1];
    expect(init.headers.Cookie).toBeUndefined();
  });
});
