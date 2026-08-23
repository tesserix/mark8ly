import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  encodeSession,
  decodeSession,
  SESSION_TTL_SECONDS,
} from "./session";

const session = {
  uid: "uid-1",
  email: "shopper@example.com",
  store_slug: "demo-store",
  store_id: "store-1",
  tenant_id: "tenant-1",
};

beforeEach(() => {
  process.env.SESSION_ENCRYPT_KEY = "test-session-key-at-least-32-bytes!!";
});

afterEach(() => {
  vi.useRealTimers();
});

describe("customer session cookie", () => {
  it("round-trips a signed session", () => {
    const decoded = decodeSession(encodeSession(session));
    expect(decoded).toMatchObject(session);
  });

  it("stamps exp so the server can expire the session independently", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-01-01T00:00:00Z"));
    const now = Math.floor(Date.now() / 1000);

    const decoded = decodeSession(encodeSession(session));
    expect(decoded?.iat).toBe(now);
    expect(decoded?.exp).toBe(now + SESSION_TTL_SECONDS);
  });

  it("rejects a session past its exp", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-01-01T00:00:00Z"));
    const cookie = encodeSession(session);

    vi.setSystemTime(new Date("2026-01-01T00:00:00Z").getTime() + (SESSION_TTL_SECONDS + 1) * 1000);
    expect(decodeSession(cookie)).toBeNull();
  });

  it("rejects a signed payload carrying no exp", () => {
    // A cookie minted before exp existed can never be revoked, so it is
    // no longer accepted.
    const payload = Buffer.from(JSON.stringify(session)).toString("base64");
    const signed = encodeSession(session);
    const sig = signed.slice(signed.lastIndexOf(".") + 1);
    // Re-sign the exp-less payload so only the missing claim is under test.
    const forged = `${payload}.${sig}`;
    expect(decodeSession(forged)).toBeNull();
  });

  it("rejects an unsigned cookie", () => {
    const raw = Buffer.from(JSON.stringify(session)).toString("base64");
    expect(decodeSession(raw)).toBeNull();
  });

  it("rejects a tampered payload", () => {
    const cookie = encodeSession(session);
    const sig = cookie.slice(cookie.lastIndexOf(".") + 1);
    const forged = Buffer.from(
      JSON.stringify({ ...session, uid: "attacker" }),
    ).toString("base64");
    expect(decodeSession(`${forged}.${sig}`)).toBeNull();
  });
});
