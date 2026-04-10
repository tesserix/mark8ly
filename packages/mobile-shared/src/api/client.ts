import { ApiClientConfig, ApiErrorBody, ApiError } from "./types";

type HttpMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE" | "HEAD";

async function request<T>(
  config: ApiClientConfig,
  method: HttpMethod,
  path: string,
  body?: unknown
): Promise<T> {
  const url = `${config.baseUrl}/api/v1/mobile/storefront/stores/${config.storeSlug}${path}`;

  const headers: Record<string, string> = {};

  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  if (config.getToken) {
    const token = await config.getToken();
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
  }

  let response: Response;
  try {
    response = await fetch(url, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : "Network error";
    throw new ApiError(500, { error: "NETWORK_ERROR", message });
  }

  if (!response.ok) {
    let errorBody: ApiErrorBody;
    try {
      errorBody = (await response.json()) as ApiErrorBody;
    } catch {
      errorBody = {
        error: "UNKNOWN_ERROR",
        message: `Request failed with status ${response.status}`,
      };
    }
    throw new ApiError(response.status, errorBody);
  }

  return response.json() as Promise<T>;
}

export interface ApiClient {
  get<T>(path: string): Promise<T>;
  post<T>(path: string, body: unknown): Promise<T>;
  put<T>(path: string, body: unknown): Promise<T>;
  patch<T>(path: string, body: unknown): Promise<T>;
  delete<T>(path: string): Promise<T>;
  head(path: string): Promise<void>;
}

export function createApiClient(config: ApiClientConfig): ApiClient {
  return {
    get: <T>(path: string) => request<T>(config, "GET", path),
    post: <T>(path: string, body: unknown) =>
      request<T>(config, "POST", path, body),
    put: <T>(path: string, body: unknown) =>
      request<T>(config, "PUT", path, body),
    patch: <T>(path: string, body: unknown) =>
      request<T>(config, "PATCH", path, body),
    delete: <T>(path: string) => request<T>(config, "DELETE", path),
    head: (path: string) =>
      request<void>(config, "HEAD", path).then(() => undefined),
  };
}
