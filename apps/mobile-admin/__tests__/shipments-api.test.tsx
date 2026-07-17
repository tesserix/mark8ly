import { createShipmentsApi } from "@repo/mobile-shared/api/shipments";
import { createOrdersApi } from "@repo/mobile-shared/api/orders";

// A fake api client that records the (method, path, body) of every call so we
// can pin the exact routes/verbs — a path typo here would silently 404 the
// feature (and trip the store-invalid reset), so it's worth asserting.
type Call = { method: string; path: string; body?: unknown; params?: unknown };

function fakeClient() {
  const calls: Call[] = [];
  const client = {
    get: (path: string, params?: unknown) => {
      calls.push({ method: "GET", path, params });
      return Promise.resolve(null);
    },
    post: (path: string, body?: unknown) => {
      calls.push({ method: "POST", path, body });
      return Promise.resolve({});
    },
    patch: (path: string, body?: unknown) => {
      calls.push({ method: "PATCH", path, body });
      return Promise.resolve({});
    },
    delete: (path: string) => {
      calls.push({ method: "DELETE", path });
      return Promise.resolve({});
    },
  } as unknown as Parameters<typeof createShipmentsApi>[0];
  return { client, calls };
}

describe("createShipmentsApi routes", () => {
  it("targets the correct order-scoped shipment paths and verbs", async () => {
    const { client, calls } = fakeClient();
    const api = createShipmentsApi(client);

    await api.get("o1");
    await api.create("o1", { provider: "delhivery", service: "standard" });
    await api.updateStatus("o1", "s1", { status: "in_transit" });
    await api.refreshTracking("o1", "s1");
    await api.schedulePickup("o1", "s1");
    await api.emailLabel("o1", "s1", "warehouse@example.com");
    await api.remove("o1", "s1");

    expect(calls).toEqual([
      { method: "GET", path: "/orders/o1/shipments", params: undefined },
      { method: "POST", path: "/orders/o1/shipments", body: { provider: "delhivery", service: "standard" } },
      { method: "PATCH", path: "/orders/o1/shipments/s1/status", body: { status: "in_transit" } },
      { method: "POST", path: "/orders/o1/shipments/s1/tracking/refresh", body: {} },
      { method: "POST", path: "/orders/o1/shipments/s1/pickup/schedule", body: {} },
      { method: "POST", path: "/orders/o1/shipments/s1/label/email", body: { recipient: "warehouse@example.com" } },
      { method: "DELETE", path: "/orders/o1/shipments/s1" },
    ]);
  });
});

describe("createOrdersApi document-email routes", () => {
  it("posts to invoice/email and receipt/email; note only sent when present", async () => {
    const { client, calls } = fakeClient();
    const api = createOrdersApi(client);

    await api.emailInvoice("o1");
    await api.emailReceipt("o1", "Thanks again!");

    expect(calls).toEqual([
      { method: "POST", path: "/orders/o1/invoice/email", body: {} },
      { method: "POST", path: "/orders/o1/receipt/email", body: { note: "Thanks again!" } },
    ]);
  });
});
