import { describe, it, expect } from "vitest";
import { parseOttoEvent, mergeMessage, mergeMessages } from "../events";
import type { SupportMessage } from "../types";

const msg = (id: string, at: string): SupportMessage => ({
  id,
  sender_type: "customer",
  sender_name: "Sam",
  body: "hi",
  created_at: at,
});

describe("parseOttoEvent", () => {
  it("parses a message.created frame", () => {
    const raw = JSON.stringify({
      type: "otto.message.created",
      room: "conv:1",
      payload: { message: { id: "m1", sender_type: "staff", body: "hello", created_at: "2026-06-21T10:00:00Z" } },
    });
    const ev = parseOttoEvent(raw);
    expect(ev.kind).toBe("message");
    if (ev.kind === "message") {
      expect(ev.message.id).toBe("m1");
      expect(ev.message.sender_type).toBe("staff");
    }
  });

  it("parses a conversation.closed frame", () => {
    const ev = parseOttoEvent(JSON.stringify({ type: "otto.conversation.closed", payload: {} }));
    expect(ev.kind).toBe("conversation_closed");
  });

  it("parses conversation.updated with conversation_id", () => {
    const ev = parseOttoEvent(JSON.stringify({ type: "otto.conversation.updated", payload: { conversation_id: "c9" } }));
    expect(ev).toEqual({ kind: "conversation_updated", conversationId: "c9" });
  });

  it("parses conversation.updated with nested conversation object", () => {
    const ev = parseOttoEvent(JSON.stringify({ type: "otto.conversation.updated", payload: { conversation: { id: "c5" } } }));
    expect(ev).toEqual({ kind: "conversation_updated", conversationId: "c5" });
  });

  it("returns unknown for unrecognised type", () => {
    expect(parseOttoEvent(JSON.stringify({ type: "otto.something.else" })).kind).toBe("unknown");
  });

  it("returns unknown for malformed JSON", () => {
    expect(parseOttoEvent("not json").kind).toBe("unknown");
  });

  it("returns unknown when the message payload is invalid", () => {
    const ev = parseOttoEvent(JSON.stringify({ type: "otto.message.created", payload: { message: { id: 123 } } }));
    expect(ev.kind).toBe("unknown");
  });
});

describe("mergeMessage", () => {
  it("appends a new message", () => {
    const out = mergeMessage([], msg("a", "2026-06-21T10:00:00Z"));
    expect(out).toHaveLength(1);
  });

  it("de-duplicates by id", () => {
    const start = [msg("a", "2026-06-21T10:00:00Z")];
    const out = mergeMessage(start, msg("a", "2026-06-21T10:00:00Z"));
    expect(out).toHaveLength(1);
    expect(out).toBe(start); // unchanged reference when no-op
  });

  it("keeps messages ordered by created_at", () => {
    const out = mergeMessages([], [msg("b", "2026-06-21T10:05:00Z"), msg("a", "2026-06-21T10:00:00Z")]);
    expect(out.map((m) => m.id)).toEqual(["a", "b"]);
  });
});
