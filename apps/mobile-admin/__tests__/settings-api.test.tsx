import { createBrandingApi } from "@repo/mobile-shared/api/branding";
import { createAuditLogApi } from "@repo/mobile-shared/api/audit-log";
import { createTicketsApi } from "@repo/mobile-shared/api/tickets";

type Call = { method: string; path: string; body?: unknown };

function fakeClient() {
  const calls: Call[] = [];
  const rec = (method: string) => (path: string, body?: unknown) => {
    calls.push({ method, path, body });
    return Promise.resolve({});
  };
  const client = {
    get: (path: string) => {
      calls.push({ method: "GET", path });
      return Promise.resolve({});
    },
    post: rec("POST"),
    patch: rec("PATCH"),
    put: rec("PUT"),
    delete: (path: string) => {
      calls.push({ method: "DELETE", path });
      return Promise.resolve({});
    },
  } as unknown as Parameters<typeof createBrandingApi>[0];
  return { client, calls };
}

describe("createBrandingApi routes", () => {
  it("GET + PUT /branding", async () => {
    const { client, calls } = fakeClient();
    const api = createBrandingApi(client);
    await api.get();
    await api.update({ tagline: "Hi" });
    expect(calls).toEqual([
      { method: "GET", path: "/branding" },
      { method: "PUT", path: "/branding", body: { tagline: "Hi" } },
    ]);
  });
});

describe("createAuditLogApi routes", () => {
  it("GET /audit-logs (list only)", async () => {
    const { client, calls } = fakeClient();
    await createAuditLogApi(client).list();
    expect(calls).toEqual([{ method: "GET", path: "/audit-logs" }]);
  });
});

describe("createTicketsApi routes", () => {
  it("list/get/create/reply/status paths + verbs", async () => {
    const { client, calls } = fakeClient();
    const api = createTicketsApi(client);
    await api.list();
    await api.get("t1");
    await api.create({ subject: "S", description: "D" });
    await api.reply("t1", "hello");
    await api.updateStatus("t1", "resolved");
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "GET /tickets",
      "GET /tickets/t1",
      "POST /tickets",
      "POST /tickets/t1/reply",
      "PATCH /tickets/t1",
    ]);
    expect(calls[3]!.body).toEqual({ content: "hello" });
    expect(calls[4]!.body).toEqual({ status: "resolved" });
  });
});
