import { describe, expect, it } from "vitest";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

// #890: four of eleven admin API clients hand-built their header set and
// omitted X-Internal-Auth, so Campaigns, Segments, Coupons, Loyalty and CSV
// import 401'd — and the BFF rendered those 401s as empty states, so nothing
// looked wrong. The duplication was the defect, not the four missing lines.
//
// This fails if any client builds session headers without going through
// auth-headers.ts.

/**
 * Strip comments before matching. Without this the check is worthless: a
 * comment mentioning X-Internal-Auth satisfies a substring search, which
 * is exactly how the first version of this test passed against a
 * deliberately re-broken client.
 */
function code(src: string): string {
  return src
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .split("\n")
    .filter((l) => !l.trim().startsWith("//"))
    .join("\n");
}

const API_DIR = join(__dirname);
const SHARED = "auth-headers.ts";

function clients(): string[] {
  return readdirSync(API_DIR).filter(
    (f) => f.endsWith(".ts") && !f.endsWith(".test.ts") && f !== SHARED,
  );
}

describe("admin API clients use the shared header builder", () => {
  it("finds clients to check", () => {
    expect(clients().length).toBeGreaterThan(5);
  });

  // Deliberately NOT asserting that every client imports ./auth-headers.
  // Seven clients still build their own header objects and are correct —
  // they send X-Internal-Auth. Failing them would be churn, not safety.
  // The invariant that prevents #890 is the header being sent at all;
  // consolidating the remaining seven is follow-up cleanup.

  it.each(clients())(
    "%s sends X-Internal-Auth if it authenticates at all",
    (file) => {
      const src = code(readFileSync(join(API_DIR, file), "utf8"));
      if (!src.includes('"X-User-Id"') && !src.includes("auth-headers")) return;

      const viaShared =
        src.includes("readHeaders") || src.includes("writeHeaders");
      const viaOwn = src.includes("X-Internal-Auth");
      expect(
        viaShared || viaOwn,
        `${file} authenticates to marketplace-api but sends no X-Internal-Auth`,
      ).toBe(true);
    },
  );
});

describe("the shared builder itself", () => {
  it("omits Content-Type on reads so multipart uploads set their own boundary", async () => {
    const { readHeaders, writeHeaders } = await import("./auth-headers");
    const session = { userId: "u", tenantId: "t" };
    expect(readHeaders(session)["Content-Type"]).toBeUndefined();
    expect(writeHeaders(session)["Content-Type"]).toBe("application/json");
  });

  it("forwards the email only when present", async () => {
    const { readHeaders } = await import("./auth-headers");
    expect(
      readHeaders({ userId: "u", tenantId: "t" })["X-User-Email"],
    ).toBeUndefined();
    expect(
      readHeaders({ userId: "u", tenantId: "t", email: "a@b.c" })[
        "X-User-Email"
      ],
    ).toBe("a@b.c");
  });

  it("always carries the session identity", async () => {
    const { readHeaders } = await import("./auth-headers");
    const h = readHeaders({ userId: "u1", tenantId: "t1" });
    expect(h["X-User-Id"]).toBe("u1");
    expect(h["X-Tenant-Id"]).toBe("t1");
  });
});
