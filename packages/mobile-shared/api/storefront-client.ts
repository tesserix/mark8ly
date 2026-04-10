import { ApiError } from "./client";

export interface StorefrontClientConfig {
  baseUrl: string;
  storeSlug: string;
  getToken?: () => Promise<string | null>;
}

export function createStorefrontClient(config: StorefrontClientConfig) {
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

  async function buildHeaders(): Promise<Record<string, string>> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      Accept: "application/json",
    };
    if (config.getToken) {
      const token = await config.getToken();
      if (token) {
        headers["Authorization"] = `Bearer ${token}`;
      }
    }
    return headers;
  }

  async function request<T>(
    method: string,
    path: string,
    options?: { body?: unknown; params?: Record<string, string> },
  ): Promise<T> {
    const url = buildUrl(path, options?.params);
    const headers = await buildHeaders();

    const res = await fetch(url, {
      method,
      headers,
      body: options?.body ? JSON.stringify(options.body) : undefined,
    });

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
      const url = buildUrl(path);
      const headers = await buildHeaders();

      const res = await fetch(url, { method: "HEAD", headers });
      return { ok: res.ok, status: res.status };
    },
  };
}
