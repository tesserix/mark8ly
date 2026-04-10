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
  variant_id: string | null;
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

// ─────────────────────────────────────────────────────────────────────────
// M7b: product + category CRUD
// ─────────────────────────────────────────────────────────────────────────

export interface CreateProductVariantInput {
  sku: string;
  barcode?: string;
  price: string;
  compare_at_price?: string;
  cost_price?: string;
  currency_code: string;
  weight_grams?: number;
  inventory_quantity: number;
  inventory_policy?: "deny" | "continue";
  low_stock_threshold?: number;
  option_values?: Array<{ option_name: string; value: string }>;
  position?: number;
}

export interface CreateProductRequest {
  handle?: string;
  title: string;
  description?: string;
  status?: "draft" | "active" | "archived";
  tags?: string[];
  seo_title?: string;
  seo_description?: string;
  primary_category_id?: string;
  options?: Array<{ name: string; values: string[] }>;
  variants: CreateProductVariantInput[];
  media?: Array<{
    storage_key: string;
    url: string;
    alt?: string;
    position?: number;
    media_type?: "image" | "video";
  }>;
  category_ids?: string[];
}

export type UpdateProductRequest = Partial<
  Omit<CreateProductRequest, "options" | "variants" | "media">
> & {
  // M7c aggregate PATCH fields — forwarded as-is to the backend
  // PATCH /products/:id endpoint. Wider shapes than the M7b create
  // endpoint to accommodate option-value ids and media ids.
  options?: Array<{
    id?: string;
    name: string;
    values: Array<{ id?: string; value: string }>;
  }>;
  variants?: Array<{
    id?: string;
    sku?: string;
    price?: string;
    inventory_quantity?: number;
    weight_grams?: number;
    currency_code?: string;
    inventory_policy?: "deny" | "continue";
    position?: number;
    option_values?: Array<{ option_name: string; value: string }>;
  }>;
  media?: Array<{
    id?: string;
    storage_key: string;
    url: string;
    alt?: string;
    position?: number;
    media_type?: "image" | "video";
    variant_id?: string | null;
  }>;
  removed_variant_ids?: string[];
};

export interface AdminCategory {
  id: string;
  store_id: string;
  parent_id: string | null;
  name: string;
  slug: string;
  description: string | null;
  image_url: string | null;
  position: number;
  is_active: boolean;
}

export interface ListCategoriesResponse {
  data: AdminCategory[];
}

/** Typed error returned from mutating endpoints. Callers pattern-match on `code`. */
export interface MutationError {
  code: string;
  message: string;
  field?: string;
  details?: Record<string, unknown>;
}

export type MutationResult<T> = { ok: true; data: T } | { ok: false; error: MutationError };

function commonHeaders(session: SessionHeaders): HeadersInit {
  return {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    Accept: "application/json",
    "Content-Type": "application/json",
  };
}

async function parseMutationError(res: Response): Promise<MutationError> {
  const body = (await res.json().catch(() => null)) as ApiError | null;
  return {
    code: body?.error ?? "unknown_error",
    message: body?.message ?? `marketplace-api returned ${res.status}`,
    field:
      typeof body?.details?.field === "string"
        ? (body.details.field as string)
        : undefined,
    details: body?.details,
  };
}

export async function getProduct(
  storeId: string,
  productId: string,
  session: SessionHeaders,
): Promise<AdminProduct | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/${productId}`,
    {
      cache: "no-store",
      headers: {
        "X-User-Id": session.userId,
        "X-Tenant-Id": session.tenantId,
        Accept: "application/json",
      },
    },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: getProduct ${res.status}`);
  }
  return (await res.json()) as AdminProduct;
}

export async function createProduct(
  storeId: string,
  body: CreateProductRequest,
  session: SessionHeaders,
): Promise<MutationResult<AdminProduct>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminProduct };
}

export async function updateProduct(
  storeId: string,
  productId: string,
  body: UpdateProductRequest,
  session: SessionHeaders,
): Promise<MutationResult<AdminProduct>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/${productId}`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminProduct };
}

export async function deleteProduct(
  storeId: string,
  productId: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/${productId}`,
    {
      method: "DELETE",
      cache: "no-store",
      headers: {
        "X-User-Id": session.userId,
        "X-Tenant-Id": session.tenantId,
      },
    },
  );
  if (res.status === 204 || res.ok) {
    return { ok: true, data: true };
  }
  return { ok: false, error: await parseMutationError(res) };
}

export async function listCategories(
  storeId: string,
  session: SessionHeaders,
): Promise<AdminCategory[]> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/categories`,
    {
      cache: "no-store",
      headers: {
        "X-User-Id": session.userId,
        "X-Tenant-Id": session.tenantId,
        Accept: "application/json",
      },
    },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return [];
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: listCategories ${res.status}`);
  }
  const body = (await res.json()) as ListCategoriesResponse;
  return body.data ?? [];
}

// ─────────────────────────────────────────────────────────────────────────
// M7c: media CRUD + signed URL flow
// ─────────────────────────────────────────────────────────────────────────

export interface MediaUploadUrl {
  upload_url: string;
  storage_key: string;
  expires_at: string;
}

export interface RequestMediaUploadUrlInput {
  content_hash: string;
  filename: string;
  content_type: "image/png" | "image/jpeg" | "image/webp";
}

export async function requestMediaUploadUrl(
  storeId: string,
  productId: string,
  body: RequestMediaUploadUrlInput,
  session: SessionHeaders,
): Promise<MutationResult<MediaUploadUrl>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/${productId}/media/upload-url`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  // Wire DTO uses `url`; we expose as `upload_url` for clarity.
  const raw = (await res.json()) as {
    url: string;
    storage_key: string;
    expires_at: string;
  };
  return {
    ok: true,
    data: {
      upload_url: raw.url,
      storage_key: raw.storage_key,
      expires_at: raw.expires_at,
    },
  };
}

export interface FinalizeMediaInput {
  storage_key: string;
  url: string;
  alt?: string;
  position?: number;
  media_type?: "image" | "video";
  variant_id?: string;
}

export async function finalizeMedia(
  storeId: string,
  productId: string,
  body: FinalizeMediaInput,
  session: SessionHeaders,
): Promise<MutationResult<AdminMediaResponse>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/${productId}/media`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminMediaResponse };
}

export interface UpdateMediaInput {
  alt?: string;
  position?: number;
  url?: string;
  variant_id?: string | null;
  storage_key?: string;
}

export async function updateMedia(
  storeId: string,
  productId: string,
  mediaId: string,
  body: UpdateMediaInput,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/${productId}/media/${mediaId}`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (res.status === 204 || res.ok) {
    return { ok: true, data: true };
  }
  return { ok: false, error: await parseMutationError(res) };
}

export async function deleteMedia(
  storeId: string,
  productId: string,
  mediaId: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/${productId}/media/${mediaId}`,
    {
      method: "DELETE",
      cache: "no-store",
      headers: {
        "X-User-Id": session.userId,
        "X-Tenant-Id": session.tenantId,
      },
    },
  );
  if (res.status === 204 || res.ok) {
    return { ok: true, data: true };
  }
  return { ok: false, error: await parseMutationError(res) };
}

export interface RecropMediaInput {
  crop_box: { x: number; y: number; width: number; height: number };
  rotation?: number;
  filename?: string;
}

export interface RecropMediaResult {
  source_original_url: string;
  upload_url: string;
  new_storage_key: string;
  expires_at: string;
}

export async function recropMedia(
  storeId: string,
  productId: string,
  mediaId: string,
  body: RecropMediaInput,
  session: SessionHeaders,
): Promise<MutationResult<RecropMediaResult>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/${productId}/media/${mediaId}/recrop`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as RecropMediaResult };
}

// ─────────────────────────────────────────────────────────────────────────
// M7d: copy-to-store + bulk actions
// ─────────────────────────────────────────────────────────────────────────

export interface AdminStore {
  id: string;
  name: string;
  slug: string;
  country_code: string;
  currency_code: string;
  status: string;
}

export interface ListStoresResponse {
  data: AdminStore[];
}

export interface CopyProductInput {
  target_store_id: string;
  copy_media: boolean;
}

export interface CopyProductResult {
  new_product_id: string;
  new_store_id: string;
}

export interface BulkActionInput {
  action: string;
  product_ids: string[];
  params?: Record<string, unknown>;
}

export interface BulkResultRow {
  id: string;
  status: "ok" | "error";
  error?: string;
}

export interface BulkActionResult {
  results: BulkResultRow[];
}

/**
 * List stores the current user has access to.
 */
export async function listMyStores(
  session: SessionHeaders,
): Promise<AdminStore[]> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores`,
    {
      cache: "no-store",
      headers: {
        "X-User-Id": session.userId,
        "X-Tenant-Id": session.tenantId,
        Accept: "application/json",
      },
    },
  );
  if (res.status === 401 || res.status === 403) {
    return [];
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: listMyStores ${res.status}`);
  }
  const body = (await res.json()) as ListStoresResponse;
  return body.data ?? [];
}

/**
 * Copy a product to another store.
 */
export async function copyProduct(
  storeId: string,
  productId: string,
  body: CopyProductInput,
  session: SessionHeaders,
): Promise<MutationResult<CopyProductResult>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/${productId}/copy`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as CopyProductResult };
}

/**
 * Execute a bulk action on multiple products.
 */
export async function bulkProductAction(
  storeId: string,
  body: BulkActionInput,
  session: SessionHeaders,
): Promise<MutationResult<BulkActionResult>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/bulk`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as BulkActionResult };
}

// ─────────────────────────────────────────────────────────────────────────
// Orders slice 1 — admin orders, returns, abandoned-carts (M4 backend).
// Phase 1: list types + listOrders + getOrder. Detail/mutation methods land
// in Phase 2.
// ─────────────────────────────────────────────────────────────────────────

export type OrderStatus = "pending" | "confirmed" | "fulfilled" | "cancelled";
export type PaymentStatus =
  | "pending"
  | "authorized"
  | "paid"
  | "failed"
  | "refunded"
  | "partially_refunded";
export type FulfillmentStatus = "unfulfilled" | "partial" | "fulfilled";

export interface AdminOrderItem {
  id: string;
  product_id?: string;
  variant_id?: string;
  title_snapshot: string;
  sku_snapshot: string;
  option_summary?: string;
  unit_price: string; // decimal as string
  quantity: number;
  line_total: string;
  currency_code: string;
}

export interface AdminOrderAddress {
  kind: string; // "shipping" | "billing"
  name: string;
  line1: string;
  line2?: string;
  city: string;
  region?: string;
  postal_code?: string;
  country_code: string;
  phone?: string;
}

export interface AdminOrder {
  id: string;
  tenant_id: string;
  store_id: string;
  order_number: string;
  idempotency_key: string;
  customer_email: string;
  customer_name?: string;
  status: OrderStatus;
  payment_status: PaymentStatus;
  fulfillment_status: FulfillmentStatus;
  subtotal: string;
  shipping_total: string;
  tax_total: string;
  discount_total: string;
  grand_total: string;
  refunded_amount: string;
  currency_code: string;
  items: AdminOrderItem[];
  addresses: AdminOrderAddress[];
  placed_at: string;
  cancelled_at?: string;
  fulfilled_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ListOrdersMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface ListOrdersResponse {
  data: AdminOrder[];
  meta: ListOrdersMeta;
}

export interface ListOrdersQuery {
  status?: OrderStatus;
  paymentStatus?: PaymentStatus;
  page?: number;
  pageSize?: number;
}

/**
 * Fetch a paginated list of admin orders for a store. Returns null on
 * 401/403/404 (no-leak pattern matching listProducts) so the page can
 * render an empty state without surfacing the auth failure to the user.
 */
export async function listOrders(
  storeId: string,
  query: ListOrdersQuery,
  session: SessionHeaders,
): Promise<ListOrdersResponse | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.paymentStatus) params.set("payment_status", query.paymentStatus);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders${
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
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: listOrders ${res.status}: ${
        errBody?.message ?? "unknown error"
      }`,
    );
  }
  return (await res.json()) as ListOrdersResponse;
}

/**
 * Fetch a single admin order by id. Returns null on not-found / auth
 * failure for the same no-leak reason as listOrders.
 */
export async function getOrder(
  storeId: string,
  orderId: string,
  session: SessionHeaders,
): Promise<AdminOrder | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}`;
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
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: getOrder ${res.status}: ${
        errBody?.message ?? "unknown error"
      }`,
    );
  }
  return (await res.json()) as AdminOrder;
}
