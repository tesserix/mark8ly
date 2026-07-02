import { describe, it, expect } from "vitest";
import {
  addItem,
  removeItem,
  markItem,
  loadOutbox,
  saveOutbox,
  outboxKey,
  backoffMs,
  isRetryable,
  outboxItemToMessage,
  type KVStorage,
  type OutboxItem,
} from "../outbox";

function memStorage(): KVStorage & { dump: Record<string, string> } {
  const dump: Record<string, string> = {};
  return {
    dump,
    getItem: async (k) => dump[k] ?? null,
    setItem: async (k, v) => {
      dump[k] = v;
    },
    removeItem: async (k) => {
      delete dump[k];
    },
  };
}

const item = (id: string, status: OutboxItem["status"] = "queued"): OutboxItem => ({
  clientMsgId: id,
  conversationId: "c1",
  body: "hi " + id,
  createdAt: "2026-06-22T10:00:00Z",
  attempts: 0,
  status,
});

describe("outbox list ops", () => {
  it("adds and dedupes by clientMsgId", () => {
    let q = addItem([], item("a"));
    q = addItem(q, item("a")); // dup ignored
    q = addItem(q, item("b"));
    expect(q.map((i) => i.clientMsgId)).toEqual(["a", "b"]);
  });

  it("removes by clientMsgId", () => {
    expect(removeItem([item("a"), item("b")], "a").map((i) => i.clientMsgId)).toEqual(["b"]);
  });

  it("marks fields on one item", () => {
    const q = markItem([item("a"), item("b")], "a", { status: "failed", attempts: 3 });
    expect(q[0]).toMatchObject({ status: "failed", attempts: 3 });
    expect(q[1]?.status).toBe("queued");
  });
});

describe("outbox persistence", () => {
  it("round-trips through storage and survives a reload", async () => {
    const s = memStorage();
    await saveOutbox(s, "c1", [item("a"), item("b")]);
    expect(s.dump[outboxKey("c1")]).toBeTruthy();
    const reloaded = await loadOutbox(s, "c1");
    expect(reloaded.map((i) => i.clientMsgId)).toEqual(["a", "b"]);
  });

  it("clears the key when empty", async () => {
    const s = memStorage();
    await saveOutbox(s, "c1", [item("a")]);
    await saveOutbox(s, "c1", []);
    expect(s.dump[outboxKey("c1")]).toBeUndefined();
    expect(await loadOutbox(s, "c1")).toEqual([]);
  });

  it("returns [] on corrupt storage", async () => {
    const s = memStorage();
    s.dump[outboxKey("c1")] = "{not json";
    expect(await loadOutbox(s, "c1")).toEqual([]);
  });
});

describe("retry policy", () => {
  it("retries transport errors and 5xx/429, not 4xx", () => {
    expect(isRetryable(null)).toBe(true);
    expect(isRetryable(500)).toBe(true);
    expect(isRetryable(502)).toBe(true);
    expect(isRetryable(429)).toBe(true);
    expect(isRetryable(400)).toBe(false);
    expect(isRetryable(409)).toBe(false);
    expect(isRetryable(401)).toBe(false);
  });

  it("backs off exponentially with a cap", () => {
    expect(backoffMs(1)).toBe(1000);
    expect(backoffMs(2)).toBe(2000);
    expect(backoffMs(3)).toBe(4000);
    expect(backoffMs(99)).toBe(30000);
  });
});

describe("optimistic rendering", () => {
  it("renders an outbox item as a pending customer message", () => {
    const m = outboxItemToMessage(item("a", "failed"));
    expect(m).toMatchObject({ id: "a", sender_type: "customer", pending: true, failed: true });
  });
});
