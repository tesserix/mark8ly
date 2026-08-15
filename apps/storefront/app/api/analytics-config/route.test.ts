import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => {
  vi.resetModules();
  vi.unstubAllEnvs();
});

describe("analytics config route", () => {
  it("is never prerendered, so it can read the pod env", async () => {
    const route = await import("./route");
    expect(route.dynamic).toBe("force-dynamic");
  });

  it("serves the OpenPanel config supplied at runtime", async () => {
    vi.stubEnv("OPENPANEL_CLIENT_ID", "a-client-id");
    vi.stubEnv("OPENPANEL_API_URL", "https://analytics.tesserix.app/api");
    vi.stubEnv("OPENPANEL_SCRIPT_URL", "https://analytics.tesserix.app/op1.js");

    const { GET } = await import("./route");
    await expect(GET().json()).resolves.toEqual({
      clientId: "a-client-id",
      apiUrl: "https://analytics.tesserix.app/api",
      scriptUrl: "https://analytics.tesserix.app/op1.js",
    });
  });

  it("reports a null client id when analytics is not configured", async () => {
    vi.stubEnv("OPENPANEL_CLIENT_ID", "");

    const { GET } = await import("./route");
    await expect(GET().json()).resolves.toMatchObject({ clientId: null });
  });

  it("is never cached, so a redeploy takes effect immediately", async () => {
    const { GET } = await import("./route");
    expect(GET().headers.get("cache-control")).toContain("no-store");
  });
});
