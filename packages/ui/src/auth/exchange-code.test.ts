import { describe, it, expect } from "vitest";
import {
  mintExchangeCode,
  verifyExchangeCode,
  ExchangeCodeError,
} from "./exchange-code";

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
});
