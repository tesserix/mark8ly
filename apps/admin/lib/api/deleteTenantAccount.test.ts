import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

import { deleteTenantAccount, PlatformApiError } from "./platform-api";

function makeFetchResponse(body: unknown, status: number): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as unknown as Response;
}

describe("deleteTenantAccount", () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  const mockedFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>;

  it("resolves on 204 and sends DELETE with uid body", async () => {
    mockedFetch().mockResolvedValue(makeFetchResponse(undefined, 204));

    await expect(
      deleteTenantAccount("t1", "u1"),
    ).resolves.toBeUndefined();

    expect(mockedFetch()).toHaveBeenCalledTimes(1);
    const [url, opts] = mockedFetch().mock.calls[0]!;
    expect(url).toContain("/internal/tenants/t1/account");
    expect(opts.method).toBe("DELETE");
    expect(opts.body).toBe(JSON.stringify({ uid: "u1" }));
  });

  it("rejects with PlatformApiError on non-2xx", async () => {
    mockedFetch().mockResolvedValue(
      makeFetchResponse(
        { error: "forbidden", message: "not allowed" },
        403,
      ),
    );

    let thrown: unknown;
    try {
      await deleteTenantAccount("t1", "u1");
    } catch (err) {
      thrown = err;
    }

    expect(thrown).toBeInstanceOf(PlatformApiError);
    expect(thrown).toMatchObject({ status: 403, code: "forbidden" });
  });
});
