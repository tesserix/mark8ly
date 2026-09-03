import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Important 2 (whole-branch review, phase 3a): before this fix, this
// route was reachable regardless of the provider flag — it set three
// flow cookies on any anonymous GET and 500'd when the issuer was
// unset. It should not exist under GIP at all.
const configMock = vi.hoisted(() => ({ authProvider: "zitadel" as "gip" | "zitadel" }));
vi.mock("@/lib/config", () => ({ publicConfig: configMock }));

import { GET } from "./route";

function makeRequest(search = ""): NextRequest {
  return new NextRequest(`https://admin.mark8ly.com/login/authorize${search}`, {
    headers: { host: "admin.mark8ly.com" },
  });
}

beforeEach(() => {
  configMock.authProvider = "zitadel";
  vi.stubEnv("NEXT_PUBLIC_ZITADEL_ISSUER", "https://auth.tesserix.app");
  vi.stubEnv("NEXT_PUBLIC_ZITADEL_ADMIN_CLIENT_ID", "admin-client-id");
});

afterEach(() => {
  vi.unstubAllEnvs();
  configMock.authProvider = "zitadel";
});

describe("GET /login/authorize — provider gate", () => {
  it("404s when the provider is not zitadel, never minting flow cookies", async () => {
    configMock.authProvider = "gip";

    const res = await GET(makeRequest());

    expect(res.status).toBe(404);
    expect((res as Response).headers.get("set-cookie")).toBeNull();
  });

  it("redirects to Zitadel's /authorize when the provider is zitadel and the issuer is configured", async () => {
    const res = await GET(makeRequest());

    expect(res.status).toBe(307);
    const location = res.headers.get("location") ?? "";
    expect(location).toContain("https://auth.tesserix.app/oauth/v2/authorize");
  });
});
