import { describe, expect, it, vi, beforeEach } from "vitest";

// #695: the reset action claimed an 8-character minimum on BOTH providers.
// Zitadel's real policy is 12 + upper/lower/number/symbol, so an
// 8-character password was rejected upstream and this action answered
// "Password must be at least 8 characters." — telling the user to do
// exactly what they had just done, with no way to discover the real rule.
const configMock = vi.hoisted(() => ({
  authProvider: "zitadel" as "gip" | "zitadel",
}));
vi.mock("@/lib/config", () => ({ publicConfig: configMock }));

const confirmPasswordReset = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/platform-api", () => ({ confirmPasswordReset }));

import { confirmPasswordResetAction } from "./actions";

beforeEach(() => {
  configMock.authProvider = "zitadel";
  confirmPasswordReset.mockReset();
  confirmPasswordReset.mockResolvedValue({ ok: true });
});

describe("confirmPasswordResetAction — password policy", () => {
  it("rejects an 8-character password WITHOUT repeating '8 characters' back", async () => {
    const r = await confirmPasswordResetAction("code", "Test123!");
    expect(r.ok).toBe(false);
    if (r.ok) return;
    // The exact failure from production: the message must not restate the
    // bound the input already satisfies.
    expect(r.message).not.toMatch(/at least 8 characters/i);
    expect(r.message).toMatch(/12/);
    // and it must never reach the upstream call
    expect(confirmPasswordReset).not.toHaveBeenCalled();
  });

  it("names every rule so the user can act on it", async () => {
    const r = await confirmPasswordResetAction("code", "alllowercaseonly");
    expect(r.ok).toBe(false);
    if (r.ok) return;
    expect(r.message).toMatch(/uppercase/i);
  });

  it("accepts a policy-compliant password and calls upstream", async () => {
    const r = await confirmPasswordResetAction("code", "Not-A-Real-Password-1!");
    expect(r.ok).toBe(true);
    expect(confirmPasswordReset).toHaveBeenCalledWith("code", "Not-A-Real-Password-1!");
  });

  it("keeps GIP's 8-character rule when the flag is off", async () => {
    configMock.authProvider = "gip";
    const ok = await confirmPasswordResetAction("code", "Test123!");
    expect(ok.ok).toBe(true);
    const short = await confirmPasswordResetAction("code", "short");
    expect(short.ok).toBe(false);
    if (short.ok) return;
    expect(short.message).toMatch(/at least 8 characters/i);
  });
});
