import { z } from "zod";

/**
 * Why a request was rejected as unauthenticated.
 *
 * - `no-session`   — no token, or the refresh could not mint one.
 * - `access-denied` — a FRESHLY minted token was still rejected. The token is
 *   valid; the server is refusing this identity. This is NOT expiry.
 */
export type UnauthorizedReason = "no-session" | "access-denied";

export interface ApiClientConfig {
  baseUrl: string;
  /** Returns the cached GIP id_token, or null when signed out. */
  getToken: () => Promise<string | null>;
  /**
   * Returns a freshly-minted GIP id_token. Called once on a 401 before
   * we give up and sign the user out. When omitted we treat the first
   * 401 as terminal.
   */
  refreshToken?: () => Promise<string | null>;
  getStoreId: () => string | null;
  /**
   * Called when the API rejects the (possibly refreshed) token with a
   * 401. The caller normally signs the user out and routes back to /login.
   */
  onUnauthorized?: (reason: UnauthorizedReason) => void | Promise<void>;
  /**
   * Called when the API rejects a request scoped to the current active
   * store with 403/404 (membership revoked, store deleted, etc.). The
   * caller drops the active store and re-runs the tenant resolver, the
   * mobile equivalent of the web's middleware redirecting to /pick-tenant.
   */
  onTenantInvalid?: (status: number) => void | Promise<void>;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

interface RawRequestInit {
  method: string;
  body?: unknown;
  isStoreScoped: boolean;
  url: string;
  headers?: Record<string, string>;
  formData?: FormData;
}

export function createApiClient(config: ApiClientConfig) {
  // Single-flight token refresh — if two parallel requests both get a
  // 401, only one fetches a fresh token. The other awaits the same
  // promise so we don't blow past Firebase's refresh-rate limits.
  let inflightRefresh: Promise<string | null> | null = null;
  function refresh(): Promise<string | null> {
    if (!config.refreshToken) return Promise.resolve(null);
    if (!inflightRefresh) {
      inflightRefresh = config
        .refreshToken()
        .finally(() => {
          inflightRefresh = null;
        });
    }
    return inflightRefresh;
  }

  async function send(token: string, init: RawRequestInit): Promise<Response> {
    const headers: Record<string, string> = {
      Authorization: `Bearer ${token}`,
      Accept: "application/json",
      ...(init.headers ?? {}),
    };
    let body: BodyInit | undefined;
    if (init.formData) {
      body = init.formData;
    } else if (init.body !== undefined) {
      headers["Content-Type"] = "application/json";
      body = JSON.stringify(init.body);
    }
    return fetch(init.url, { method: init.method, headers, body });
  }

  async function execute(init: RawRequestInit): Promise<Response> {
    const token = await config.getToken();
    if (!token) {
      // No token at all — surface immediately so the caller can route to /login.
      await config.onUnauthorized?.("no-session");
      throw new ApiError(401, "unauthorized", "Not authenticated");
    }

    let res = await send(token, init);
    if (res.status === 401) {
      // Try a single forced token refresh. GIP id_tokens expire after an
      // hour and may have stale custom claims — refresh first, then retry.
      const refreshed = await refresh();
      if (refreshed) {
        res = await send(refreshed, init);
      }
      if (res.status === 401) {
        // A fresh token that is STILL rejected is not expiry — the server is
        // refusing this identity. Only a failed refresh means "session gone".
        const reason: UnauthorizedReason = refreshed ? "access-denied" : "no-session";
        await config.onUnauthorized?.(reason);
        throw new ApiError(
          401,
          "unauthorized",
          refreshed ? "Access denied" : "Session expired",
        );
      }
    }

    if (init.isStoreScoped && (res.status === 403 || res.status === 404)) {
      // Active store became invalid for this user (role revoked, store
      // deleted, member removed). Self-correct by re-resolving the tenant.
      await config.onTenantInvalid?.(res.status);
    }

    return res;
  }

  async function request<T>(
    method: string,
    path: string,
    options?: {
      body?: unknown;
      schema?: z.ZodType<T>;
      params?: Record<string, string>;
      /**
       * When true, the path is mounted under `/api/v1/mobile/admin`
       * directly, skipping the active-store prefix. Use this for
       * tenant-wide endpoints like the stores membership list.
       */
      tenantScope?: boolean;
    },
  ): Promise<T> {
    const storeId = config.getStoreId();
    const useStorePrefix = !options?.tenantScope && storeId !== null;
    const url = new URL(
      `/api/v1/mobile/admin${useStorePrefix ? `/stores/${storeId}` : ""}${path}`,
      config.baseUrl,
    );

    if (options?.params) {
      for (const [k, v] of Object.entries(options.params)) {
        url.searchParams.set(k, v);
      }
    }

    const res = await execute({
      method,
      body: options?.body,
      isStoreScoped: useStorePrefix,
      url: url.toString(),
    });

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: "unknown", message: res.statusText }));
      throw new ApiError(res.status, err.error ?? "unknown", err.message ?? res.statusText);
    }

    const data = await res.json();
    if (options?.schema) {
      const parsed = options.schema.safeParse(data);
      if (!parsed.success) {
        // A contract break must name the field. Silent `undefined` is what let
        // 31 mismatches hide for two months. 500 (not 401/403) because this is
        // our bug, not the user's — it must never reach the sign-out paths.
        const issue = parsed.error.issues[0];
        const fieldPath = issue?.path.join(".") || "(root)";
        const detail = `${fieldPath}: ${issue?.message ?? "Invalid input"}`;
        console.error(`[api] contract mismatch on ${path}: ${detail}`);
        throw new ApiError(500, "contract_mismatch", detail);
      }
      return parsed.data;
    }
    return data as T;
  }

  return {
    get: <T>(path: string, params?: Record<string, string>, schema?: z.ZodType<T>) =>
      request<T>("GET", path, { params, schema }),
    /** GET against a tenant-wide path (skips the `/stores/{id}` prefix). */
    getTenant: <T>(path: string, params?: Record<string, string>, schema?: z.ZodType<T>) =>
      request<T>("GET", path, { params, schema, tenantScope: true }),
    post: <T>(path: string, body?: unknown, schema?: z.ZodType<T>) =>
      request<T>("POST", path, { body, schema }),
    patch: <T>(path: string, body?: unknown, schema?: z.ZodType<T>) =>
      request<T>("PATCH", path, { body, schema }),
    delete: <T>(path: string) => request<T>("DELETE", path),
    uploadMedia: async (path: string, formData: FormData) => {
      const storeId = config.getStoreId();
      if (!storeId) {
        throw new ApiError(400, "no_active_store", "No active store");
      }
      const url = `${config.baseUrl}/api/v1/mobile/admin/stores/${storeId}${path}`;
      const res = await execute({
        method: "POST",
        formData,
        isStoreScoped: true,
        url,
      });
      if (!res.ok) throw new ApiError(res.status, "upload_failed", "Media upload failed");
      return res.json();
    },
  };
}
