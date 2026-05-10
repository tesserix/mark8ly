import { ApiError } from "./client";

export interface StorefrontClientConfig {
  baseUrl: string;
  storeSlug: string;
  /** Returns the cached GIP id_token, or null when browsing as guest. */
  getToken?: () => Promise<string | null>;
  /**
   * Mints a fresh GIP id_token. Called once on a 401 before treating
   * the session as terminated. Anonymous browse paths can omit this.
   */
  refreshToken?: () => Promise<string | null>;
  /**
   * Called when the API rejects the (possibly refreshed) token with 401.
   * The caller normally signs the customer out and routes to /sign-in.
   */
  onUnauthorized?: () => void | Promise<void>;
}

interface SendInit {
  method: string;
  url: string;
  body?: unknown;
  formData?: FormData;
}

export function createStorefrontClient(config: StorefrontClientConfig) {
  // Single-flight refresh — parallel requests share one refresh call so
  // we don't churn through Firebase's rate limit on a token spike.
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

  function buildUrl(path: string, params?: Record<string, string>): string {
    const url = new URL(
      `/api/v1/mobile/storefront/stores/${config.storeSlug}${path}`,
      config.baseUrl,
    );
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        url.searchParams.set(k, v);
      }
    }
    return url.toString();
  }

  async function send(token: string | null, init: SendInit): Promise<Response> {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (token) headers["Authorization"] = `Bearer ${token}`;
    let body: BodyInit | undefined;
    if (init.formData) {
      body = init.formData;
    } else if (init.body !== undefined) {
      headers["Content-Type"] = "application/json";
      body = JSON.stringify(init.body);
    }
    return fetch(init.url, { method: init.method, headers, body });
  }

  async function execute(init: SendInit): Promise<Response> {
    const token = config.getToken ? await config.getToken() : null;

    let res = await send(token, init);
    if (res.status === 401 && token) {
      const refreshed = await refresh();
      if (refreshed) {
        res = await send(refreshed, init);
      }
      if (res.status === 401) {
        await config.onUnauthorized?.();
        throw new ApiError(401, "unauthorized", "Session expired");
      }
    }
    return res;
  }

  async function request<T>(
    method: string,
    path: string,
    options?: { body?: unknown; params?: Record<string, string> },
  ): Promise<T> {
    const url = buildUrl(path, options?.params);
    const res = await execute({ method, url, body: options?.body });

    if (!res.ok) {
      const err = await res
        .json()
        .catch(() => ({ error: "unknown", message: res.statusText }));
      throw new ApiError(
        res.status,
        err.error ?? "unknown",
        err.message ?? res.statusText,
      );
    }

    // 204 No Content — no JSON body to parse.
    if (res.status === 204) return undefined as T;
    return res.json() as Promise<T>;
  }

  return {
    get: <T>(path: string, params?: Record<string, string>) =>
      request<T>("GET", path, { params }),
    post: <T>(path: string, body?: unknown) =>
      request<T>("POST", path, { body }),
    put: <T>(path: string, body?: unknown) =>
      request<T>("PUT", path, { body }),
    patch: <T>(path: string, body?: unknown) =>
      request<T>("PATCH", path, { body }),
    delete: <T>(path: string) => request<T>("DELETE", path),
    head: async (path: string): Promise<{ ok: boolean; status: number }> => {
      const res = await execute({ method: "HEAD", url: buildUrl(path) });
      return { ok: res.ok, status: res.status };
    },
  };
}
