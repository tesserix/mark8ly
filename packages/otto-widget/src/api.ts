import type { Conversation, Message } from "./types";

export interface OttoApi {
  /** Start a new thread. Returns conversation + opening message. The
   *  `otp_code` field is required for anonymous callers — logged-in users
   *  (whose identity the storefront proxy forwards server-side) can omit
   *  it and are accepted on their existing session. */
  startConversation(input: {
    subject?: string;
    name?: string;
    email?: string;
    message: string;
    otp_code?: string;
  }): Promise<{ conversation: Conversation; first_message: Message }>;
  /** Request a 6-digit verification code be emailed to the given address. */
  requestOtp(input: {
    email: string;
    name?: string;
    store_name?: string;
  }): Promise<{ sent: boolean; masked_to: string }>;
  /** Fetch the conversation the caller is currently bound to. */
  getConversation(id: string): Promise<{ conversation: Conversation }>;
  /** Fetch full message history for a thread. */
  listMessages(id: string): Promise<{ messages: Message[] }>;
  /** Send a new message as the customer. */
  sendMessage(id: string, body: string): Promise<{ message: Message }>;
  /** Close the conversation from the customer side. */
  closeConversation(id: string): Promise<{ conversation: Conversation }>;
}

/**
 * buildOttoApi returns a ready-to-use client scoped to the given base URL.
 * In the storefront this is `/api/otto` (the Next.js proxy prefix).
 * In the admin app — or any future host — swap the base and the calls work
 * identically.
 */
export function buildOttoApi(baseUrl: string): OttoApi {
  const base = baseUrl.replace(/\/+$/, "");
  const post = async <T,>(path: string, body?: unknown): Promise<T> => {
    const res = await fetch(`${base}${path}`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!res.ok) throw await toError(res);
    return (await res.json()) as T;
  };
  const get = async <T,>(path: string): Promise<T> => {
    const res = await fetch(`${base}${path}`, { credentials: "include" });
    if (!res.ok) throw await toError(res);
    return (await res.json()) as T;
  };

  return {
    startConversation: (input) => post("/conversations", input),
    requestOtp: (input) => post("/verify/start", input),
    getConversation: (id) => get(`/conversations/${encodeURIComponent(id)}`),
    listMessages: (id) =>
      get(`/conversations/${encodeURIComponent(id)}/messages`),
    sendMessage: (id, body) =>
      post(`/conversations/${encodeURIComponent(id)}/messages`, { body }),
    closeConversation: (id) =>
      post(`/conversations/${encodeURIComponent(id)}/close`, {}),
  };
}

async function toError(res: Response): Promise<Error> {
  let msg = `request failed (${res.status})`;
  try {
    const body = (await res.json()) as { error?: string; message?: string };
    if (body.message) msg = body.message;
    else if (body.error) msg = body.error;
  } catch {
    /* keep default */
  }
  const e = new Error(msg) as Error & { status?: number };
  e.status = res.status;
  return e;
}
