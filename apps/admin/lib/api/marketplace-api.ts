// apps/admin/lib/api/marketplace-api.ts
//
// marketplace-api client for server components in the admin app.
//
// M7a ships only the admin products list endpoint. M7b adds product
// detail, create, update, delete. M7c adds media + variants. M7d adds
// copy-to-store. Categories are fetched for the inline picker in M7b.
//
// Calling convention: every server component that talks to marketplace-
// api receives the session headers from its caller (usually a page
// component) and passes them into the client functions. The client does
// the header rename dance (x-session-user-id → X-User-Id).

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

/** Session headers as they arrive from Next middleware. */
export interface SessionHeaders {
  userId: string;
  tenantId: string;
}

/** Admin product row as returned by GET /api/v1/admin/stores/:storeId/products. */
export interface AdminProduct {
  id: string;
  store_id: string;
  handle: string;
  title: string;
  description: string | null;
  status: "draft" | "active" | "archived";
  tags: string[];
  seo_title: string | null;
  seo_description: string | null;
  primary_category_id: string | null;
  copy_source_product_id: string | null;
  categories: AdminCategoryRef[];
  options: AdminProductOption[];
  variants: AdminVariantResponse[];
  media: AdminMediaResponse[];
  published_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface AdminCategoryRef {
  id: string;
  name: string;
  slug: string;
}

export interface AdminProductOption {
  id: string;
  name: string;
  position: number;
  values: AdminProductOptionValue[];
}

export interface AdminProductOptionValue {
  id: string;
  value: string;
  position: number;
}

export interface AdminVariantResponse {
  id: string;
  sku: string;
  barcode: string | null;
  price: string; // decimal as string
  compare_at_price: string | null;
  cost_price: string | null;
  currency_code: string;
  weight_grams: number | null;
  inventory_quantity: number;
  inventory_policy: "deny" | "continue";
  low_stock_threshold: number | null;
  option_values: AdminVariantOptionRef[];
  position: number;
}

export interface AdminVariantOptionRef {
  option_name: string;
  option_value_id: string;
  value: string;
}

export interface AdminMediaResponse {
  id: string;
  url: string;
  storage_key: string;
  alt: string | null;
  position: number;
  media_type: "image" | "video";
  width: number | null;
  height: number | null;
  bytes: number | null;
}

export interface ListProductsMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface ListProductsResponse {
  data: AdminProduct[];
  meta: ListProductsMeta;
}

export interface ListProductsQuery {
  status?: "draft" | "active" | "archived";
  search?: string;
  page?: number;
  pageSize?: number;
}

export interface ApiError {
  error: string;
  message: string;
  details?: Record<string, unknown>;
}

/**
 * Lists products under a store. Server-component only — the session
 * headers come from the caller's serverSession context.
 *
 * Returns `{ data, meta }` on success. On 401/403/404 returns null so
 * callers can render an empty or not-found state without try/catch
 * scaffolding. On unexpected errors throws so the Next error boundary
 * handles the rendering.
 */
export async function listProducts(
  storeId: string,
  query: ListProductsQuery,
  session: SessionHeaders,
): Promise<ListProductsResponse | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.search) params.set("search", query.search);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products${
    qs ? `?${qs}` : ""
  }`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: {
      "X-User-Id": session.userId,
      "X-Tenant-Id": session.tenantId,
      Accept: "application/json",
    },
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res
      .json()
      .catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: listProducts ${res.status}: ${
        errBody?.message ?? "unknown error"
      }`,
    );
  }
  return (await res.json()) as ListProductsResponse;
}

// M7b will add: getProduct, createProduct, updateProduct, deleteProduct, copyProduct
// M7b will add: listCategories, createCategory, updateCategory, deleteCategory
// M7c will add: uploadUrl, createMedia, updateMedia, deleteMedia, updateVariantBasics
