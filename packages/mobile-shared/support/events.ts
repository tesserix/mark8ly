// WebSocket event parsing + the pure message-merge reducer. Kept free of
// React Native imports so it is unit-testable under the node vitest env;
// the RN socket lifecycle lives in useSupportChat.
import { SupportMessageSchema, type SupportMessage } from "./types";

// OttoEvent is the normalised form of an otto WebSocket envelope
// ({type, room, payload}). Unknown/garbage frames collapse to "unknown"
// so the UI can ignore them safely.
export type OttoEvent =
  | { kind: "message"; message: SupportMessage }
  | { kind: "conversation_updated"; conversationId?: string }
  | { kind: "conversation_closed" }
  | { kind: "unknown" };

const UNKNOWN: OttoEvent = { kind: "unknown" };

/** Parses a raw WebSocket frame into a typed OttoEvent. */
export function parseOttoEvent(raw: string): OttoEvent {
  let env: unknown;
  try {
    env = JSON.parse(raw);
  } catch {
    return UNKNOWN;
  }
  if (!env || typeof env !== "object") return UNKNOWN;
  const e = env as { type?: unknown; payload?: unknown };
  const payload = (e.payload ?? {}) as Record<string, unknown>;

  switch (e.type) {
    case "otto.message.created": {
      const parsed = SupportMessageSchema.safeParse(payload.message);
      return parsed.success ? { kind: "message", message: parsed.data } : UNKNOWN;
    }
    case "otto.conversation.closed":
      return { kind: "conversation_closed" };
    case "otto.conversation.updated": {
      const conv = payload.conversation as { id?: string } | undefined;
      const conversationId =
        (typeof payload.conversation_id === "string" ? payload.conversation_id : undefined) ??
        conv?.id;
      return { kind: "conversation_updated", conversationId };
    }
    default:
      return UNKNOWN;
  }
}

/**
 * Appends a message, de-duplicating by id (the sender sees their own
 * message optimistically AND echoed back over the socket) and keeping the
 * list ordered by created_at.
 */
export function mergeMessage(
  messages: SupportMessage[],
  incoming: SupportMessage,
): SupportMessage[] {
  if (messages.some((m) => m.id === incoming.id)) return messages;
  const next = [...messages, incoming];
  next.sort((a, b) => (a.created_at < b.created_at ? -1 : a.created_at > b.created_at ? 1 : 0));
  return next;
}

/** Merges a batch (initial history / resume) into existing messages. */
export function mergeMessages(
  messages: SupportMessage[],
  batch: SupportMessage[],
): SupportMessage[] {
  return batch.reduce(mergeMessage, messages);
}
