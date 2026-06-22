// createSupportClient — the shared REST client for the mobile support BFF.
// Both apps construct it with their own base path + GIP token getters; the
// only support-specific behaviour lives here, once:
//   - sends the GIP bearer token (with a single-flight refresh on 401)
//   - carries the opaque otto session as X-Otto-Session on every call and
//     captures the fresh token the BFF returns in the create body (the app
//     can't read otto's HttpOnly cookie), optionally persisting it
//   - unwraps otto's { conversation } / { messages } / { message } envelopes
import { z } from "zod";

import {
  CreateConversationInput,
  FeedbackInput,
  QueueStateSchema,
  SupportConversationSchema,
  SupportMessageSchema,
  WsTicketSchema,
  toCreateBody,
  type QueueState,
  type SupportConversation,
  type SupportMessage,
  type WsTicket,
} from "./types";

export class SupportError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = "SupportError";
  }
}

export interface SupportClientConfig {
  /** API origin, e.g. "https://api.mark8ly.com". */
  baseUrl: string;
  /**
   * Path prefix for this surface, e.g.
   * "/api/v1/mobile/storefront/stores/acme/support" (customer->merchant) or
   * "/api/v1/mobile/admin/platform-support" (merchant->platform).
   */
  basePath: string;
  /** Returns the cached GIP id_token, or null when signed out. */
  getToken: () => Promise<string | null>;
  /** Mints a fresh GIP id_token; called once on a 401 before giving up. */
  refreshToken?: () => Promise<string | null>;
  /** Called when a (refreshed) token is still rejected with 401. */
  onUnauthorized?: () => void | Promise<void>;
  /** Loads a persisted otto session token on first use (cross-launch resume). */
  loadSessionToken?: () => Promise<string | null> | string | null;
  /** Persists the otto session token when otto mints a new one. */
  saveSessionToken?: (token: string) => Promise<void> | void;
}

export interface ResumeResult {
  conversation: SupportConversation;
  messages: SupportMessage[];
}

export function createSupportClient(config: SupportClientConfig) {
  let sessionToken: string | null = null;
  let sessionLoaded = false;

  async function loadSession(): Promise<string | null> {
    if (!sessionLoaded) {
      sessionLoaded = true;
      if (config.loadSessionToken) {
        sessionToken = (await config.loadSessionToken()) ?? null;
      }
    }
    return sessionToken;
  }

  async function rememberSession(token: string): Promise<void> {
    sessionToken = token;
    if (config.saveSessionToken) await config.saveSessionToken(token);
  }

  // Single-flight token refresh — parallel 401s share one refresh call.
  let inflightRefresh: Promise<string | null> | null = null;
  function refresh(): Promise<string | null> {
    if (!config.refreshToken) return Promise.resolve(null);
    if (!inflightRefresh) {
      inflightRefresh = config.refreshToken().finally(() => {
        inflightRefresh = null;
      });
    }
    return inflightRefresh;
  }

  async function send(token: string | null, method: string, path: string, body?: unknown): Promise<Response> {
    const session = await loadSession();
    const headers: Record<string, string> = { Accept: "application/json" };
    if (token) headers["Authorization"] = `Bearer ${token}`;
    if (session) headers["X-Otto-Session"] = session;
    let payload: string | undefined;
    if (body !== undefined) {
      headers["Content-Type"] = "application/json";
      payload = JSON.stringify(body);
    }
    return fetch(`${config.baseUrl}${config.basePath}${path}`, { method, headers, body: payload });
  }

  async function request(method: string, path: string, body?: unknown): Promise<any> {
    let token = await config.getToken();
    let res = await send(token, method, path, body);

    if (res.status === 401 && token) {
      const refreshed = await refresh();
      if (refreshed) {
        token = refreshed;
        res = await send(refreshed, method, path, body);
      }
      if (res.status === 401) {
        await config.onUnauthorized?.();
        throw new SupportError(401, "unauthorized", "Session expired");
      }
    }

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: "error", message: res.statusText }));
      throw new SupportError(res.status, err.error ?? "error", err.message ?? res.statusText);
    }

    if (res.status === 204) return undefined;
    const data = await res.json();
    // otto mints the session cookie on create; the BFF surfaces it in the
    // body as session_token. Capture + persist it for later calls.
    if (data && typeof data === "object" && typeof data.session_token === "string") {
      await rememberSession(data.session_token);
    }
    return data;
  }

  return {
    /** Opens a new conversation and returns it plus the first message. */
    createConversation: async (
      input: CreateConversationInput,
    ): Promise<{ conversation: SupportConversation; firstMessage: SupportMessage }> => {
      const data = await request("POST", "/conversations", toCreateBody(input));
      return {
        conversation: SupportConversationSchema.parse(data.conversation),
        firstMessage: SupportMessageSchema.parse(data.first_message),
      };
    },

    /** Returns the customer's most recent open thread, or null. */
    resume: async (): Promise<ResumeResult | null> => {
      const data = await request("GET", "/resume");
      if (!data || data.conversation == null) return null;
      return {
        conversation: SupportConversationSchema.parse(data.conversation),
        messages: z.array(SupportMessageSchema).parse(data.messages ?? []),
      };
    },

    getConversation: async (id: string): Promise<SupportConversation> => {
      const data = await request("GET", `/conversations/${encodeURIComponent(id)}`);
      return SupportConversationSchema.parse(data.conversation);
    },

    listMessages: async (id: string): Promise<SupportMessage[]> => {
      const data = await request("GET", `/conversations/${encodeURIComponent(id)}/messages`);
      return z.array(SupportMessageSchema).parse(data.messages ?? []);
    },

    postMessage: async (id: string, body: string): Promise<SupportMessage> => {
      const data = await request("POST", `/conversations/${encodeURIComponent(id)}/messages`, { body });
      return SupportMessageSchema.parse(data.message);
    },

    close: async (id: string): Promise<SupportConversation> => {
      const data = await request("POST", `/conversations/${encodeURIComponent(id)}/close`);
      return SupportConversationSchema.parse(data.conversation);
    },

    submitFeedback: async (id: string, feedback: FeedbackInput): Promise<SupportConversation> => {
      const data = await request("POST", `/conversations/${encodeURIComponent(id)}/feedback`, feedback);
      return SupportConversationSchema.parse(data.conversation);
    },

    getQueue: async (id: string): Promise<QueueState> => {
      const data = await request("GET", `/conversations/${encodeURIComponent(id)}/queue`);
      return QueueStateSchema.parse(data);
    },

    getWsTicket: async (id: string): Promise<WsTicket> => {
      const data = await request("POST", `/conversations/${encodeURIComponent(id)}/ws-ticket`);
      return WsTicketSchema.parse(data);
    },

    /** Builds the full WebSocket URL (origin + path + ?ticket=). */
    buildWsUrl: (ticket: WsTicket): string =>
      `${ticket.ws_url}?ticket=${encodeURIComponent(ticket.ticket)}`,

    /** The currently held otto session token, if any. */
    currentSessionToken: (): string | null => sessionToken,
  };
}

export type SupportClient = ReturnType<typeof createSupportClient>;
