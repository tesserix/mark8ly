import { z } from "zod";

export interface ApiClientConfig {
  baseUrl: string;
  getToken: () => Promise<string | null>;
  getStoreId: () => string | null;
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

export function createApiClient(config: ApiClientConfig) {
  async function request<T>(
    method: string,
    path: string,
    options?: { body?: unknown; schema?: z.ZodType<T>; params?: Record<string, string> },
  ): Promise<T> {
    const token = await config.getToken();
    if (!token) throw new ApiError(401, "unauthorized", "Not authenticated");

    const storeId = config.getStoreId();
    const url = new URL(
      `/api/v1/mobile/admin${storeId ? `/stores/${storeId}` : ""}${path}`,
      config.baseUrl,
    );

    if (options?.params) {
      for (const [k, v] of Object.entries(options.params)) {
        url.searchParams.set(k, v);
      }
    }

    const res = await fetch(url.toString(), {
      method,
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: options?.body ? JSON.stringify(options.body) : undefined,
    });

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: "unknown", message: res.statusText }));
      throw new ApiError(res.status, err.error ?? "unknown", err.message ?? res.statusText);
    }

    const data = await res.json();
    if (options?.schema) return options.schema.parse(data);
    return data as T;
  }

  return {
    get: <T>(path: string, params?: Record<string, string>, schema?: z.ZodType<T>) =>
      request<T>("GET", path, { params, schema }),
    post: <T>(path: string, body?: unknown, schema?: z.ZodType<T>) =>
      request<T>("POST", path, { body, schema }),
    patch: <T>(path: string, body?: unknown, schema?: z.ZodType<T>) =>
      request<T>("PATCH", path, { body, schema }),
    delete: <T>(path: string) => request<T>("DELETE", path),
    uploadMedia: async (path: string, formData: FormData) => {
      const token = await config.getToken();
      if (!token) throw new ApiError(401, "unauthorized", "Not authenticated");
      const storeId = config.getStoreId();
      const url = `${config.baseUrl}/api/v1/mobile/admin/stores/${storeId}${path}`;
      const res = await fetch(url, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
        body: formData,
      });
      if (!res.ok) throw new ApiError(res.status, "upload_failed", "Media upload failed");
      return res.json();
    },
  };
}
