import { describe, it, expect } from "vitest";
import {
  mintAdminHandoffCode,
  verifyAdminHandoffCode,
  AdminHandoffError,
  ADMIN_HANDOFF_KIND,
} from "./admin-handoff-code";

const KEY = "test-key-32-bytes-padded-padded!!";

const baseInput = {
  uid: "uid-1",
  email: "merchant@primasyss.com",
  tenant_id: "tenant-1",
  store_id: "store-1",
  target_host: "admin.primasyss.com",
};

describe("admin-handoff-code", () => {
  it("round-trips a payload", () => {
    const code = mintAdminHandoffCode(baseInput, KEY, 30);
    const claims = verifyAdminHandoffCode(code, KEY);
    expect(claims.kind).toBe(ADMIN_HANDOFF_KIND);
    expect(claims.uid).toBe(baseInput.uid);
    expect(claims.target_host).toBe(baseInput.target_host);
    expect(claims.tenant_id).toBe(baseInput.tenant_id);
  });

  it("rejects a tampered payload", () => {
    const code = mintAdminHandoffCode(baseInput, KEY, 30);
    const tampered = code.replace(/\.[^.]+$/, ".bad-signature");
    expect(() => verifyAdminHandoffCode(tampered, KEY)).toThrow(
      AdminHandoffError,
    );
  });

  it("rejects an expired code", () => {
    const code = mintAdminHandoffCode(baseInput, KEY, 0);
    expect(() => verifyAdminHandoffCode(code, KEY)).toThrow(/expired/i);
  });

  it("rejects a code signed with a different key", () => {
    const code = mintAdminHandoffCode(baseInput, KEY, 30);
    expect(() =>
      verifyAdminHandoffCode(code, "different-key-32-bytes-padded!!!"),
    ).toThrow(AdminHandoffError);
  });

  it("rejects malformed input", () => {
    expect(() => verifyAdminHandoffCode("nodothere", KEY)).toThrow(
      AdminHandoffError,
    );
  });

  it("rejects empty key", () => {
    expect(() => mintAdminHandoffCode(baseInput, "", 30)).toThrow(
      AdminHandoffError,
    );
    expect(() => verifyAdminHandoffCode("a.b", "")).toThrow(AdminHandoffError);
  });

  it("rejects mint without uid / tenant_id / target_host", () => {
    expect(() =>
      mintAdminHandoffCode({ ...baseInput, uid: "" }, KEY, 30),
    ).toThrow(AdminHandoffError);
    expect(() =>
      mintAdminHandoffCode({ ...baseInput, tenant_id: "" }, KEY, 30),
    ).toThrow(AdminHandoffError);
    expect(() =>
      mintAdminHandoffCode({ ...baseInput, target_host: "" }, KEY, 30),
    ).toThrow(AdminHandoffError);
  });
});
