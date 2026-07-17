import { createCouponsApi } from "@repo/mobile-shared/api/coupons";
import { createCampaignsApi } from "@repo/mobile-shared/api/campaigns";
import { createSegmentsApi } from "@repo/mobile-shared/api/segments";
import { createGiftCardsApi } from "@repo/mobile-shared/api/gift-cards";
import { createLoyaltyApi } from "@repo/mobile-shared/api/loyalty";

type Call = { method: string; path: string; body?: unknown };

// Fake client that records calls and returns an enveloped stub so the api
// layer's `.then(r => r.data)` unwrap doesn't throw. Pins routes/verbs — a
// path typo would 404 on the store-scoped client and bounce the app Home.
function fakeClient() {
  const calls: Call[] = [];
  const stub = () => Promise.resolve({ data: {} });
  const client = {
    get: (path: string) => {
      calls.push({ method: "GET", path });
      return stub();
    },
    post: (path: string, body?: unknown) => {
      calls.push({ method: "POST", path, body });
      return stub();
    },
    patch: (path: string, body?: unknown) => {
      calls.push({ method: "PATCH", path, body });
      return stub();
    },
    put: (path: string, body?: unknown) => {
      calls.push({ method: "PUT", path, body });
      return stub();
    },
    delete: (path: string) => {
      calls.push({ method: "DELETE", path });
      return Promise.resolve({});
    },
  } as unknown as Parameters<typeof createCouponsApi>[0];
  return { client, calls };
}

describe("createCouponsApi routes", () => {
  it("targets /coupons paths and verbs", async () => {
    const { client, calls } = fakeClient();
    const api = createCouponsApi(client);
    await api.list();
    await api.get("c1");
    await api.create({ code: "X", title: "T", type: "percentage", value: 10 });
    await api.patch("c1", { status: "disabled" });
    await api.remove("c1");
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "GET /coupons",
      "GET /coupons/c1",
      "POST /coupons",
      "PATCH /coupons/c1",
      "DELETE /coupons/c1",
    ]);
  });
});

describe("createCampaignsApi routes", () => {
  it("targets /campaigns paths incl. lifecycle verbs", async () => {
    const { client, calls } = fakeClient();
    const api = createCampaignsApi(client);
    await api.list();
    await api.get("m1");
    await api.create({ name: "N" });
    await api.patch("m1", { name: "N2" });
    await api.send("m1");
    await api.schedule("m1", "2026-08-01T00:00:00Z");
    await api.pause("m1");
    await api.resume("m1");
    await api.remove("m1");
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "GET /campaigns",
      "GET /campaigns/m1",
      "POST /campaigns",
      "PATCH /campaigns/m1",
      "POST /campaigns/m1/send",
      "POST /campaigns/m1/schedule",
      "POST /campaigns/m1/pause",
      "POST /campaigns/m1/resume",
      "DELETE /campaigns/m1",
    ]);
  });

  it("schedule sends {scheduled_at}", async () => {
    const { client, calls } = fakeClient();
    await createCampaignsApi(client).schedule("m1", "2026-08-01T00:00:00Z");
    expect(calls[0]!.body).toEqual({ scheduled_at: "2026-08-01T00:00:00Z" });
  });
});

describe("createSegmentsApi routes", () => {
  it("targets /segments paths and verbs", async () => {
    const { client, calls } = fakeClient();
    const api = createSegmentsApi(client);
    await api.list();
    await api.get("s1");
    await api.create({ name: "N", rules: "[]" });
    await api.update("s1", { name: "N2", rules: "[]" });
    await api.remove("s1");
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "GET /segments",
      "GET /segments/s1",
      "POST /segments",
      "PATCH /segments/s1",
      "DELETE /segments/s1",
    ]);
  });
});

describe("createGiftCardsApi routes", () => {
  it("targets /gift-cards paths and verbs (issue + read only)", async () => {
    const { client, calls } = fakeClient();
    const api = createGiftCardsApi(client);
    await api.list();
    await api.get("g1");
    await api.issue({ initial_balance: 50, currency_code: "AUD" });
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "GET /gift-cards",
      "GET /gift-cards/g1",
      "POST /gift-cards",
    ]);
  });
});

describe("createLoyaltyApi routes", () => {
  it("targets /loyalty paths incl. PUT program + member adjust", async () => {
    const { client, calls } = fakeClient();
    const api = createLoyaltyApi(client);
    await api.getProgram();
    await api.updateProgram({
      is_active: true,
      points_per_unit: 1,
      points_currency: "AUD",
      min_redeem_points: 500,
      points_value: 0.01,
    });
    await api.listMembers();
    await api.getMember("m1");
    await api.adjustPoints("m1", { points: 100, description: "goodwill" });
    await api.listReferrals();
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "GET /loyalty/program",
      "PUT /loyalty/program",
      "GET /loyalty/members",
      "GET /loyalty/members/m1",
      "POST /loyalty/members/m1/adjust",
      "GET /loyalty/referrals",
    ]);
  });
});
