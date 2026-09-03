import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => {
  vi.resetModules();
  vi.unstubAllEnvs();
});

describe("publicConfig", () => {
  it("defaults authProvider to 'gip' when NEXT_PUBLIC_AUTH_PROVIDER is unset", async () => {
    vi.stubEnv("NEXT_PUBLIC_AUTH_PROVIDER", "");
    const { publicConfig } = await import("./config");
    expect(publicConfig.authProvider).toBe("gip");
  });

  it("sets authProvider to 'zitadel' when NEXT_PUBLIC_AUTH_PROVIDER is exactly 'zitadel'", async () => {
    vi.stubEnv("NEXT_PUBLIC_AUTH_PROVIDER", "zitadel");
    const { publicConfig } = await import("./config");
    expect(publicConfig.authProvider).toBe("zitadel");
  });

  it("defaults authProvider to 'gip' when NEXT_PUBLIC_AUTH_PROVIDER is 'Zitadel' (capitalized)", async () => {
    vi.stubEnv("NEXT_PUBLIC_AUTH_PROVIDER", "Zitadel");
    const { publicConfig } = await import("./config");
    expect(publicConfig.authProvider).toBe("gip");
  });

  it("defaults authProvider to 'gip' when NEXT_PUBLIC_AUTH_PROVIDER is 'ZITADEL' (uppercase)", async () => {
    vi.stubEnv("NEXT_PUBLIC_AUTH_PROVIDER", "ZITADEL");
    const { publicConfig } = await import("./config");
    expect(publicConfig.authProvider).toBe("gip");
  });

  it("defaults authProvider to 'gip' when NEXT_PUBLIC_AUTH_PROVIDER is ' zitadel' (with leading whitespace)", async () => {
    vi.stubEnv("NEXT_PUBLIC_AUTH_PROVIDER", " zitadel");
    const { publicConfig } = await import("./config");
    expect(publicConfig.authProvider).toBe("gip");
  });

  it("defaults authProvider to 'gip' when NEXT_PUBLIC_AUTH_PROVIDER is 'zitadel ' (with trailing whitespace)", async () => {
    vi.stubEnv("NEXT_PUBLIC_AUTH_PROVIDER", "zitadel ");
    const { publicConfig } = await import("./config");
    expect(publicConfig.authProvider).toBe("gip");
  });

  it("defaults authProvider to 'gip' when NEXT_PUBLIC_AUTH_PROVIDER is 'true'", async () => {
    vi.stubEnv("NEXT_PUBLIC_AUTH_PROVIDER", "true");
    const { publicConfig } = await import("./config");
    expect(publicConfig.authProvider).toBe("gip");
  });

  it("defaults authProvider to 'gip' when NEXT_PUBLIC_AUTH_PROVIDER is an arbitrary unrecognised value", async () => {
    vi.stubEnv("NEXT_PUBLIC_AUTH_PROVIDER", "unsupported-provider");
    const { publicConfig } = await import("./config");
    expect(publicConfig.authProvider).toBe("gip");
  });

  it("includes zitadelIssuer from NEXT_PUBLIC_ZITADEL_ISSUER env var", async () => {
    vi.stubEnv("NEXT_PUBLIC_ZITADEL_ISSUER", "https://zitadel.example.com");
    const { publicConfig } = await import("./config");
    expect(publicConfig.zitadelIssuer).toBe("https://zitadel.example.com");
  });

  it("defaults zitadelIssuer to empty string when unset", async () => {
    vi.stubEnv("NEXT_PUBLIC_ZITADEL_ISSUER", "");
    const { publicConfig } = await import("./config");
    expect(publicConfig.zitadelIssuer).toBe("");
  });

  it("includes zitadelAdminClientId from NEXT_PUBLIC_ZITADEL_ADMIN_CLIENT_ID env var", async () => {
    vi.stubEnv("NEXT_PUBLIC_ZITADEL_ADMIN_CLIENT_ID", "admin-client-id-123");
    const { publicConfig } = await import("./config");
    expect(publicConfig.zitadelAdminClientId).toBe("admin-client-id-123");
  });

  it("defaults zitadelAdminClientId to empty string when unset", async () => {
    vi.stubEnv("NEXT_PUBLIC_ZITADEL_ADMIN_CLIENT_ID", "");
    const { publicConfig } = await import("./config");
    expect(publicConfig.zitadelAdminClientId).toBe("");
  });
});
