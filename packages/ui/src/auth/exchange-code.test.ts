import { createHmac } from "node:crypto";
import { describe, it, expect } from "vitest";
import {
  mintExchangeCode,
  verifyExchangeCode,
  ExchangeCodeError,
  EXCHANGE_CODE_KIND,
} from "./exchange-code";
import { ADMIN_HANDOFF_KIND, mintAdminHandoffCode } from "./admin-handoff-code";

const KEY = "test-key-32-bytes-padded-padded!!";

describe("exchange-code", () => {
  it("round-trips a payload", () => {
    const code = mintExchangeCode(
      {
        idToken: "id-token",
        storeSlug: "store-a",
        returnTo: "https://store-a.mark8ly.com/account",
        intent: "signin",
      },
      KEY,
      30,
    );
    const claims = verifyExchangeCode(code, KEY);
    expect(claims.storeSlug).toBe("store-a");
    expect(claims.intent).toBe("signin");
    expect(claims.idToken).toBe("id-token");
  });
  it("rejects a tampered payload", () => {
    const code = mintExchangeCode(
      {
        idToken: "id-token",
        storeSlug: "store-a",
        returnTo: "https://store-a.mark8ly.com/account",
        intent: "signin",
      },
      KEY,
      30,
    );
    const tampered = code.replace(/\.[^.]+$/, ".bad-signature");
    expect(() => verifyExchangeCode(tampered, KEY)).toThrow(ExchangeCodeError);
  });
  it("rejects an expired code", () => {
    const code = mintExchangeCode(
      {
        idToken: "x",
        storeSlug: "s",
        returnTo: "https://s.mark8ly.com/",
        intent: "signin",
      },
      KEY,
      0, // already expired
    );
    expect(() => verifyExchangeCode(code, KEY)).toThrow(/expired/i);
  });
  it("rejects a code signed with a different key", () => {
    const code = mintExchangeCode(
      {
        idToken: "x",
        storeSlug: "s",
        returnTo: "https://s.mark8ly.com/",
        intent: "signin",
      },
      KEY,
      30,
    );
    expect(() => verifyExchangeCode(code, "different-key-32-bytes-padded!!!")).toThrow(
      ExchangeCodeError,
    );
  });
  it("rejects malformed input (no dot)", () => {
    expect(() => verifyExchangeCode("nodothere", KEY)).toThrow(ExchangeCodeError);
  });
  it("rejects empty key on mint and verify", () => {
    expect(() =>
      mintExchangeCode(
        { idToken: "x", storeSlug: "s", returnTo: "https://s.mark8ly.com/", intent: "signin" },
        "",
        30,
      ),
    ).toThrow(ExchangeCodeError);
    expect(() => verifyExchangeCode("a.b", "")).toThrow(ExchangeCodeError);
  });

  it("round-trips a minted code and exposes its kind", () => {
    const code = mintExchangeCode(
      { idToken: "t", storeSlug: "shop", returnTo: "/", intent: "signin" },
      KEY,
      30,
    );
    const claims = verifyExchangeCode(code, KEY);
    expect(claims.kind).toBe(EXCHANGE_CODE_KIND);
    expect(claims.storeSlug).toBe("shop");
  });

  it("rejects a code minted by a sibling module with the same key", () => {
    const foreign = mintAdminHandoffCode(
      {
        uid: "uid-1",
        email: "merchant@example.com",
        tenant_id: "t1",
        target_host: "admin.example.com",
      },
      KEY,
      30,
    );
    expect(() => verifyExchangeCode(foreign, KEY)).toThrow(
      expect.objectContaining({ code: "wrong_kind" }),
    );
  });

  it("rejects a legacy code that carries no kind at all", () => {
    // Hand-built payload in the pre-kind format, signed with the real key.
    const legacy = {
      idToken: "t",
      storeSlug: "shop",
      returnTo: "/",
      intent: "signin",
      exp: Math.floor(Date.now() / 1000) + 30,
    };
    const payload = Buffer.from(JSON.stringify(legacy)).toString("base64url");
    const sig = createHmac("sha256", KEY).update(payload).digest("hex");
    expect(() => verifyExchangeCode(`${payload}.${sig}`, KEY)).toThrow(
      expect.objectContaining({ code: "wrong_kind" }),
    );
  });
});
