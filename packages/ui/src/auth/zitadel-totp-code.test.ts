import { describe, it, expect } from "vitest";
import { createHmac } from "node:crypto";
import {
  mintZitadelTotpCode,
  verifyZitadelTotpCode,
  ZitadelTotpCodeError,
  ZITADEL_TOTP_CODE_KIND,
} from "./zitadel-totp-code";

const KEY = "test-key-32-bytes-padded-padded!!";

const baseInput = {
  tenant_id: "tenant-1",
  multiple_tenants: true,
};

describe("zitadel-totp-code", () => {
  it("round-trips a payload", () => {
    const code = mintZitadelTotpCode(baseInput, KEY, 30);
    const claims = verifyZitadelTotpCode(code, KEY);
    expect(claims.kind).toBe(ZITADEL_TOTP_CODE_KIND);
    expect(claims.tenant_id).toBe(baseInput.tenant_id);
    expect(claims.multiple_tenants).toBe(baseInput.multiple_tenants);
  });

  it("rejects a tampered payload", () => {
    const code = mintZitadelTotpCode(baseInput, KEY, 30);
    const [, sig] = code.split(".");
    const tamperedPayload = Buffer.from(
      JSON.stringify({ ...baseInput, kind: ZITADEL_TOTP_CODE_KIND, tenant_id: "tenant-evil", exp: 9999999999 }),
    ).toString("base64url");
    const tampered = `${tamperedPayload}.${sig}`;
    expect(() => verifyZitadelTotpCode(tampered, KEY)).toThrow(ZitadelTotpCodeError);
  });

  it("rejects a tampered signature", () => {
    const code = mintZitadelTotpCode(baseInput, KEY, 30);
    const tampered = code.replace(/\.[^.]+$/, ".bad-signature");
    expect(() => verifyZitadelTotpCode(tampered, KEY)).toThrow(ZitadelTotpCodeError);
  });

  it("rejects an expired code", () => {
    const code = mintZitadelTotpCode(baseInput, KEY, 0);
    expect(() => verifyZitadelTotpCode(code, KEY)).toThrow(/expired/i);
    try {
      verifyZitadelTotpCode(code, KEY);
      throw new Error("expected verifyZitadelTotpCode to throw");
    } catch (err) {
      expect(err).toBeInstanceOf(ZitadelTotpCodeError);
      expect((err as ZitadelTotpCodeError).code).toBe("expired");
    }
  });

  it("rejects a code signed with a different key", () => {
    const code = mintZitadelTotpCode(baseInput, KEY, 30);
    expect(() =>
      verifyZitadelTotpCode(code, "different-key-32-bytes-padded!!!"),
    ).toThrow(ZitadelTotpCodeError);
  });

  it("rejects malformed input with no separator", () => {
    expect(() => verifyZitadelTotpCode("nodothere", KEY)).toThrow(ZitadelTotpCodeError);
  });

  it("rejects an empty string", () => {
    expect(() => verifyZitadelTotpCode("", KEY)).toThrow(ZitadelTotpCodeError);
  });

  it("rejects a payload that is not valid base64url", () => {
    // "!!!" is not in the base64url alphabet, so decoding it does not
    // yield valid JSON — this exercises the malformed_payload branch
    // without throwing anything but ZitadelTotpCodeError.
    expect(() => verifyZitadelTotpCode("!!!.somesig", KEY)).toThrow(ZitadelTotpCodeError);
  });

  it("rejects a payload that is valid base64url but not JSON", () => {
    const notJsonPayload = Buffer.from("not json at all").toString("base64url");
    const sig = createHmac("sha256", KEY).update(notJsonPayload).digest("hex");
    const code = `${notJsonPayload}.${sig}`;
    expect(() => verifyZitadelTotpCode(code, KEY)).toThrow(ZitadelTotpCodeError);
    try {
      verifyZitadelTotpCode(code, KEY);
      throw new Error("expected verifyZitadelTotpCode to throw");
    } catch (err) {
      expect(err).toBeInstanceOf(ZitadelTotpCodeError);
      expect((err as ZitadelTotpCodeError).code).toBe("malformed_payload");
    }
  });

  it("rejects a code minted under a different kind, signed with the same key", () => {
    // Build a code by hand carrying a different `kind` — this is the
    // property that distinguishes this module from the structurally
    // identical exchange-code.ts / admin-handoff-code.ts: codes are not
    // interchangeable across purposes even when the wire format and key
    // match.
    const claims = {
      kind: "some_other_kind_v1",
      tenant_id: baseInput.tenant_id,
      multiple_tenants: baseInput.multiple_tenants,
      exp: Math.floor(Date.now() / 1000) + 300,
    };
    const payload = Buffer.from(JSON.stringify(claims)).toString("base64url");
    const sig = createHmac("sha256", KEY).update(payload).digest("hex");
    const code = `${payload}.${sig}`;

    try {
      verifyZitadelTotpCode(code, KEY);
      throw new Error("expected verifyZitadelTotpCode to throw");
    } catch (err) {
      expect(err).toBeInstanceOf(ZitadelTotpCodeError);
      expect((err as ZitadelTotpCodeError).code).toBe("wrong_kind");
    }
  });

  it("rejects an empty key on mint and verify", () => {
    expect(() => mintZitadelTotpCode(baseInput, "", 30)).toThrow(ZitadelTotpCodeError);
    expect(() => verifyZitadelTotpCode("a.b", "")).toThrow(ZitadelTotpCodeError);
  });

  it("rejects mint without tenant_id", () => {
    expect(() => mintZitadelTotpCode({ ...baseInput, tenant_id: "" }, KEY, 30)).toThrow(
      ZitadelTotpCodeError,
    );
  });
});
