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

const MARKETPLACE_INTERNAL_AUTH =
  process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";

/** Session headers as they arrive from Next middleware. */
export interface SessionHeaders {
  userId: string;
  tenantId: string;
  /** User's email for audit-log attribution. Optional for backward
   *  compatibility — when present it's forwarded to marketplace-api as
   *  X-User-Email and recorded against any audit event the request
   *  emits. */
  email?: string;
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
    headers: readHeaders(session),
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
  // PATCH /products/:id endpoint. The backend reuses CreateProductOptionInput
  // for PATCH, which is `{name, values: []string}` — no ids on options or
  // values. Media keeps id so the aggregate service can match existing rows.
  options?: Array<{ name: string; values: string[] }>;
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
  featured: boolean;
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
  const headers: Record<string, string> = {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    Accept: "application/json",
    "Content-Type": "application/json",
  };
  if (session.email) {
    headers["X-User-Email"] = session.email;
  }
  if (MARKETPLACE_INTERNAL_AUTH) {
    headers["X-Internal-Auth"] = MARKETPLACE_INTERNAL_AUTH;
  }
  return headers;
}

function readHeaders(session: SessionHeaders): HeadersInit {
  const headers: Record<string, string> = {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    Accept: "application/json",
  };
  if (session.email) {
    headers["X-User-Email"] = session.email;
  }
  if (MARKETPLACE_INTERNAL_AUTH) {
    headers["X-Internal-Auth"] = MARKETPLACE_INTERNAL_AUTH;
  }
  return headers;
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
      headers: readHeaders(session),
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
      headers: readHeaders(session),
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
      headers: readHeaders(session),
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

export interface CreateCategoryInput {
  name: string;
  slug?: string;
  description?: string;
  position?: number;
}

export async function createCategory(
  storeId: string,
  input: CreateCategoryInput,
  session: SessionHeaders,
): Promise<MutationResult<AdminCategory>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/categories`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(input),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const raw = (await res.json()) as
    | { data: AdminCategory }
    | AdminCategory;
  const data =
    raw && typeof raw === "object" && "data" in raw && raw.data
      ? (raw as { data: AdminCategory }).data
      : (raw as AdminCategory);
  if (!data || !data.id) {
    return {
      ok: false,
      error: {
        code: "malformed_response",
        message: "Category created but response was malformed.",
      },
    };
  }
  return { ok: true, data };
}

export interface UpdateCategoryInput {
  name?: string;
  slug?: string;
  description?: string | null;
  image_url?: string | null;
  position?: number;
  is_active?: boolean;
  featured?: boolean;
}

export async function updateCategory(
  storeId: string,
  categoryId: string,
  input: UpdateCategoryInput,
  session: SessionHeaders,
): Promise<MutationResult<AdminCategory>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/categories/${categoryId}`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(input),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const raw = (await res.json()) as
    | { data: AdminCategory }
    | AdminCategory;
  const data =
    raw && typeof raw === "object" && "data" in raw && raw.data
      ? (raw as { data: AdminCategory }).data
      : (raw as AdminCategory);
  return { ok: true, data };
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

export type BrandingImageKind = "logo" | "favicon" | "hero" | "aside" | "section";

export interface RequestBrandingUploadUrlInput {
  filename: string;
  content_type: string;
  kind: BrandingImageKind;
}

export interface BrandingUploadUrl {
  upload_url: string;
  public_url: string;
  storage_key: string;
  expires_at: string;
}

export async function requestBrandingUploadUrl(
  storeId: string,
  body: RequestBrandingUploadUrlInput,
  session: SessionHeaders,
): Promise<MutationResult<BrandingUploadUrl>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/branding/upload-url`,
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
  const raw = (await res.json()) as BrandingUploadUrl;
  return { ok: true, data: raw };
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
      headers: readHeaders(session),
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
      headers: readHeaders(session),
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
// Pages CMS — admin store content pages
// ─────────────────────────────────────────────────────────────────────────

export interface AdminPage {
  id: string;
  store_id: string;
  slug: string;
  title: string;
  body: string;
  seo_title: string | null;
  seo_description: string | null;
  published: boolean;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

// AdminPageSummary is the list-view DTO returned by GET .../pages.
// Body and SEO fields are omitted to keep list payloads small.
export interface AdminPageSummary {
  id: string;
  slug: string;
  title: string;
  published: boolean;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export interface CreatePageInput {
  slug: string;
  title: string;
  body?: string;
  seo_title?: string | null;
  seo_description?: string | null;
  published?: boolean;
  sort_order?: number;
}

export interface UpdatePageInput {
  slug?: string;
  title?: string;
  body?: string;
  seo_title?: string | null;
  seo_description?: string | null;
  published?: boolean;
  sort_order?: number;
}

export async function listPages(
  storeId: string,
  session: SessionHeaders,
): Promise<AdminPageSummary[]> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/pages`,
    {
      cache: "no-store",
      headers: readHeaders(session),
    },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return [];
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: listPages ${res.status}`);
  }
  // Backend returns a bare array (c.JSON(http.StatusOK, resp) where resp is []PageSummaryResponse).
  return (await res.json()) as AdminPageSummary[];
}

export async function getPage(
  storeId: string,
  pageId: string,
  session: SessionHeaders,
): Promise<AdminPage | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/pages/${pageId}`,
    {
      cache: "no-store",
      headers: readHeaders(session),
    },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: getPage ${res.status}`);
  }
  return (await res.json()) as AdminPage;
}

export async function createPage(
  storeId: string,
  input: CreatePageInput,
  session: SessionHeaders,
): Promise<MutationResult<AdminPage>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/pages`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(input),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminPage };
}

export async function updatePage(
  storeId: string,
  pageId: string,
  input: UpdatePageInput,
  session: SessionHeaders,
): Promise<MutationResult<AdminPage>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/pages/${pageId}`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(input),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminPage };
}

export async function deletePage(
  storeId: string,
  pageId: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/pages/${pageId}`,
    {
      method: "DELETE",
      cache: "no-store",
      headers: readHeaders(session),
    },
  );
  if (res.status === 204 || res.ok) {
    return { ok: true, data: true };
  }
  return { ok: false, error: await parseMutationError(res) };
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

export interface AdminOrderTaxLine {
  description: string; // "CGST 9%", "IGST 18%", "VAT 20%", "CA State Tax"
  rate: string;        // decimal as string
  amount: string;      // decimal as string
  jurisdiction?: string;
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
  // tax_lines: per-jurisdiction breakdown surfaced by Get-by-id endpoint.
  // Empty/undefined for orders pre-dating the persistence wiring; the
  // PDF then falls back to the aggregate Tax line.
  tax_lines?: AdminOrderTaxLine[];
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
  /** Free-text search (order number, customer name/email). Backend does
   *  ILIKE matching across those columns. */
  search?: string;
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
  if (query.search) params.set("search", query.search);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders${
    qs ? `?${qs}` : ""
  }`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: readHeaders(session),
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
    headers: readHeaders(session),
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

// ─────────────────────────────────────────────────────────────────────────
// Orders Phase 2 — admin mutations: confirm, fulfill, cancel, refund.
// All return MutationResult<AdminOrder> so callers (server actions) can
// pattern-match on .ok and surface typed errors to the form UI.
// ─────────────────────────────────────────────────────────────────────────

export interface ConfirmOrderInput {
  /** Optional payment status to set in the same step (e.g. "paid"). */
  payment_status?: PaymentStatus;
  reason?: string;
}

export async function confirmOrder(
  storeId: string,
  orderId: string,
  body: ConfirmOrderInput,
  session: SessionHeaders,
): Promise<MutationResult<AdminOrder>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/confirm`,
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
  return { ok: true, data: (await res.json()) as AdminOrder };
}

export async function fulfillOrder(
  storeId: string,
  orderId: string,
  session: SessionHeaders,
): Promise<MutationResult<AdminOrder>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/fulfill`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminOrder };
}

export interface CancelOrderInput {
  reason: string;
}

export async function cancelOrder(
  storeId: string,
  orderId: string,
  body: CancelOrderInput,
  session: SessionHeaders,
): Promise<MutationResult<AdminOrder>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/cancel`,
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
  return { ok: true, data: (await res.json()) as AdminOrder };
}

export interface RefundOrderInput {
  /** Decimal string. */
  amount: string;
  /** Target payment_status: "partially_refunded" or "refunded". */
  payment_status: PaymentStatus;
  reason?: string;
}

export async function refundOrder(
  storeId: string,
  orderId: string,
  body: RefundOrderInput,
  session: SessionHeaders,
): Promise<MutationResult<AdminOrder>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/refund`,
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
  return { ok: true, data: (await res.json()) as AdminOrder };
}

// ─────────────────────────────────────────────────────────────────────────
// Marketing M2: Gift Cards
// ─────────────────────────────────────────────────────────────────────────

export interface AdminGiftCard {
  id: string;
  code: string;
  code_display: string;
  initial_balance: string;
  current_balance: string;
  currency_code: string;
  status: "active" | "disabled" | "depleted" | "pending" | "refunded";
  sender_name?: string;
  sender_email?: string;
  recipient_name?: string;
  recipient_email?: string;
  message?: string;
  purchased_at?: string;
  expires_at?: string;
  created_at: string;
  // Storefront purchase metadata. Falsy/absent for admin-issued cards.
  purchased_via_storefront?: boolean;
  payment_status?: "pending" | "paid" | "failed" | "refunded";
  payment_provider?: string;
  purchased_by_name?: string;
  purchased_by_email?: string;
}

export interface AdminGiftCardTransaction {
  id: string;
  type: "purchase" | "redeem" | "refund" | "adjustment";
  amount: string;
  balance_after: string;
  order_id?: string;
  note?: string;
  created_at: string;
}

export interface AdminGiftCardDetail extends AdminGiftCard {
  transactions: AdminGiftCardTransaction[];
}

export interface ListGiftCardsQuery {
  status?: "active" | "disabled" | "depleted";
  page?: number;
  pageSize?: number;
}

export interface ListGiftCardsResponse {
  data: AdminGiftCard[];
  meta: ListProductsMeta;
}

export interface IssueGiftCardInput {
  initial_balance: string;
  currency_code: string;
  sender_name?: string;
  sender_email?: string;
  recipient_name?: string;
  recipient_email?: string;
  message?: string;
  expires_at?: string;
}

export async function listGiftCards(
  storeId: string,
  query: ListGiftCardsQuery,
  session: SessionHeaders,
): Promise<ListGiftCardsResponse | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/gift-cards${
    qs ? `?${qs}` : ""
  }`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: readHeaders(session),
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: listGiftCards ${res.status}: ${errBody?.message ?? "unknown error"}`,
    );
  }
  return (await res.json()) as ListGiftCardsResponse;
}

export async function getGiftCard(
  storeId: string,
  giftCardId: string,
  session: SessionHeaders,
): Promise<{ data: AdminGiftCardDetail } | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/gift-cards/${giftCardId}`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: readHeaders(session),
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: getGiftCard ${res.status}: ${errBody?.message ?? "unknown error"}`,
    );
  }
  return (await res.json()) as { data: AdminGiftCardDetail };
}

// ─────────────────────────────────────────────────────────────────────────
// Customers C2 — admin customer profiles
// ─────────────────────────────────────────────────────────────────────────

export type CustomerStatus = "active" | "blocked";

export interface AdminCustomer {
  id: string;
  email: string;
  first_name?: string;
  last_name?: string;
  phone?: string;
  avatar_url?: string;
  tags: string[];
  status: CustomerStatus;
  block_reason?: string;
  notes?: string;
  marketing_opt_in: boolean;
  order_count: number;
  total_spent: number;
  last_order_at?: string;
  created_at: string;
  updated_at: string;
}

export interface AdminCustomerAddress {
  id: string;
  label?: string;
  is_default: boolean;
  name: string;
  line1: string;
  line2?: string;
  city: string;
  region?: string;
  postal_code?: string;
  country_code: string;
  phone?: string;
}

export interface AdminCustomerDetail extends AdminCustomer {
  addresses: AdminCustomerAddress[];
}

export interface ListCustomersMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface ListCustomersResponse {
  data: AdminCustomer[];
  meta: ListCustomersMeta;
}

export interface ListCustomersQuery {
  search?: string;
  status?: CustomerStatus;
  tag?: string;
  sortBy?: string;
  sortDir?: string;
  page?: number;
  pageSize?: number;
}

export async function listCustomers(
  storeId: string,
  query: ListCustomersQuery,
  session: SessionHeaders,
): Promise<ListCustomersResponse | null> {
  const params = new URLSearchParams();
  if (query.search) params.set("search", query.search);
  if (query.status) params.set("status", query.status);
  if (query.tag) params.set("tag", query.tag);
  if (query.sortBy) params.set("sort_by", query.sortBy);
  if (query.sortDir) params.set("sort_dir", query.sortDir);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/customers${
    qs ? `?${qs}` : ""
  }`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: readHeaders(session),
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: listCustomers ${res.status}: ${
        errBody?.message ?? "unknown error"
      }`,
    );
  }
  return (await res.json()) as ListCustomersResponse;
}

export async function getCustomer(
  storeId: string,
  customerId: string,
  session: SessionHeaders,
): Promise<AdminCustomerDetail | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/customers/${customerId}`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: getCustomer ${res.status}: ${
        errBody?.message ?? "unknown error"
      }`,
    );
  }
  return (await res.json()) as AdminCustomerDetail;
}

export async function updateCustomerTags(
  storeId: string,
  customerId: string,
  tags: string[],
  session: SessionHeaders,
): Promise<MutationResult<AdminCustomer>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/customers/${customerId}/tags`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ tags }),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminCustomer };
}

export async function updateCustomerNotes(
  storeId: string,
  customerId: string,
  notes: string,
  session: SessionHeaders,
): Promise<MutationResult<AdminCustomer>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/customers/${customerId}/notes`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ notes }),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminCustomer };
}

export async function blockCustomer(
  storeId: string,
  customerId: string,
  reason: string,
  session: SessionHeaders,
): Promise<MutationResult<AdminCustomer>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/customers/${customerId}/block`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ reason }),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminCustomer };
}

export async function unblockCustomer(
  storeId: string,
  customerId: string,
  session: SessionHeaders,
): Promise<MutationResult<AdminCustomer>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/customers/${customerId}/unblock`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminCustomer };
}

export async function issueGiftCard(
  storeId: string,
  input: IssueGiftCardInput,
  session: SessionHeaders,
): Promise<{ data: AdminGiftCard }> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/gift-cards`;
  const res = await fetch(url, {
    method: "POST",
    cache: "no-store",
    headers: commonHeaders(session),
    body: JSON.stringify(input),
  });

  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: issueGiftCard ${res.status}: ${errBody?.message ?? "unknown error"}`,
    );
  }
  return (await res.json()) as { data: AdminGiftCard };
}

// ─────────────────────────────────────────────────────────────────────────
// D1: Dashboard
// ─────────────────────────────────────────────────────────────────────────

export interface DashboardStats {
  revenue_today: number;
  revenue_week: number;
  revenue_month: number;
  revenue_change_pct: number;
  revenue_trend: number[];
  orders_today: number;
  orders_pending: number;
  orders_fulfilled: number;
  orders_cancelled: number;
  customers_total: number;
  customers_new_this_week: number;
  pending_reviews: number;
}

export interface RecentOrder {
  id: string;
  order_number: string;
  customer_email: string;
  grand_total: number;
  status: OrderStatus;
  created_at: string;
}

export interface TopProduct {
  id: string;
  title: string;
  image_url: string | null;
  revenue: number;
  units_sold: number;
}

export interface LowStockItem {
  id: string;
  title: string;
  variant_title: string;
  quantity: number;
  low_stock_threshold: number;
}

export interface SetupChecklist {
  has_store: boolean;
  has_brand_assets: boolean;
  has_product: boolean;
  has_storefront_theme: boolean;
  has_payment_provider: boolean;
  has_shipping_carrier: boolean;
  has_return_policy: boolean;
  has_custom_domain: boolean;
}

export interface DashboardResponse {
  stats: DashboardStats;
  recent_orders: RecentOrder[];
  top_products: TopProduct[];
  low_stock: LowStockItem[];
  setup_checklist: SetupChecklist;
}

/**
 * Fetch the admin dashboard overview for a store.
 */
export async function fetchDashboard(
  storeId: string,
  session: SessionHeaders,
): Promise<DashboardResponse | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/dashboard`,
    {
      cache: "no-store",
      headers: readHeaders(session),
    },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: fetchDashboard ${res.status}: ${errBody?.message ?? "unknown error"}`,
    );
  }
  return (await res.json()) as DashboardResponse;
}

// ─────────────────────────────────────────────────────────────────────────
// D1b: Dashboard Metrics (range-scoped analytics tabs)
// ─────────────────────────────────────────────────────────────────────────

export type AnalyticsRange = "7d" | "30d" | "90d";

export interface TimeSeriesPoint {
  date: string;
  value: number;
}

export interface SalesMetricsResponse {
  range: AnalyticsRange;
  revenue_series: TimeSeriesPoint[];
  net_revenue_series: TimeSeriesPoint[];
  gross_revenue: number;
  total_refunded: number;
  net_revenue: number;
  aov: number;
  aov_prev: number;
  aov_delta_pct: number | null;
  coupon_redemptions: number;
  coupon_discount_total: number;
}

export interface OrderStatusBreakdown {
  pending: number;
  confirmed: number;
  in_progress: number;
  fulfilled: number;
  cancelled: number;
  returned: number;
  refunded: number;
}

export interface OrderStatusSeriesPoint {
  date: string;
  pending: number;
  confirmed: number;
  in_progress: number;
  fulfilled: number;
  cancelled: number;
}

export interface OrdersMetricsResponse {
  range: AnalyticsRange;
  orders_series: TimeSeriesPoint[];
  status_series: OrderStatusSeriesPoint[];
  status_totals: OrderStatusBreakdown;
  avg_hours_to_fulfill: number | null;
  fulfillment_rate: number;
  cancel_rate: number;
  total_orders: number;
}

export interface CustomerSegmentPoint {
  date: string;
  new: number;
  returning: number;
}

export interface TopCustomer {
  email: string;
  spend: number;
  order_count: number;
}

export interface CustomersMetricsResponse {
  range: AnalyticsRange;
  series: CustomerSegmentPoint[];
  new_total: number;
  returning_total: number;
  unique_buyers: number;
  top_customers: TopCustomer[];
}

export interface RatingDistribution {
  r1: number;
  r2: number;
  r3: number;
  r4: number;
  r5: number;
}

export interface ReviewsMetricsResponse {
  range: AnalyticsRange;
  avg_rating: number;
  total_reviews: number;
  distribution: RatingDistribution;
  series: TimeSeriesPoint[];
}

export type MetricsTab = "sales" | "orders" | "customers" | "reviews";

async function fetchMetrics<T>(
  tab: MetricsTab,
  storeId: string,
  range: AnalyticsRange,
  session: SessionHeaders,
): Promise<T | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/dashboard/metrics/${tab}?range=${range}`,
    { cache: "no-store", headers: readHeaders(session) },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: fetch ${tab} metrics ${res.status}: ${errBody?.message ?? "unknown error"}`,
    );
  }
  return (await res.json()) as T;
}

export function fetchSalesMetrics(
  storeId: string,
  range: AnalyticsRange,
  session: SessionHeaders,
): Promise<SalesMetricsResponse | null> {
  return fetchMetrics<SalesMetricsResponse>("sales", storeId, range, session);
}

export function fetchOrdersMetrics(
  storeId: string,
  range: AnalyticsRange,
  session: SessionHeaders,
): Promise<OrdersMetricsResponse | null> {
  return fetchMetrics<OrdersMetricsResponse>("orders", storeId, range, session);
}

export function fetchCustomersMetrics(
  storeId: string,
  range: AnalyticsRange,
  session: SessionHeaders,
): Promise<CustomersMetricsResponse | null> {
  return fetchMetrics<CustomersMetricsResponse>(
    "customers",
    storeId,
    range,
    session,
  );
}

export function fetchReviewsMetrics(
  storeId: string,
  range: AnalyticsRange,
  session: SessionHeaders,
): Promise<ReviewsMetricsResponse | null> {
  return fetchMetrics<ReviewsMetricsResponse>(
    "reviews",
    storeId,
    range,
    session,
  );
}

// ─────────────────────────────────────────────────────────────────────────
// D2: Support Tickets
// ─────────────────────────────────────────────────────────────────────────

export type TicketStatus = "open" | "in_progress" | "resolved" | "closed";
export type TicketPriority = "low" | "medium" | "high";

export interface AdminTicketReply {
  id: string;
  author_type: "merchant" | "platform" | "customer";
  author_name: string;
  author_email?: string;
  content: string;
  created_at: string;
}

export interface AdminTicket {
  id: string;
  store_id: string;
  ticket_number: string;
  subject: string;
  description: string;
  status: TicketStatus;
  priority: TicketPriority;
  author_email: string;
  replies: AdminTicketReply[];
  created_at: string;
  updated_at: string;
}

export interface ListTicketsQuery {
  status?: TicketStatus;
  search?: string;
  page?: number;
  pageSize?: number;
}

export interface ListTicketsResponse {
  data: AdminTicket[];
  meta: ListProductsMeta;
  counts: { open: number; resolved: number; closed: number };
}

export interface CreateTicketInput {
  subject: string;
  description: string;
  priority: TicketPriority;
}

export interface TicketReplyInput {
  content: string;
}

/**
 * List support tickets for a store with optional status/search filtering.
 */
export async function listTickets(
  storeId: string,
  query: ListTicketsQuery,
  session: SessionHeaders,
): Promise<ListTicketsResponse | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.search) params.set("search", query.search);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/tickets${
    qs ? `?${qs}` : ""
  }`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: listTickets ${res.status}: ${errBody?.message ?? "unknown error"}`,
    );
  }
  return (await res.json()) as ListTicketsResponse;
}

/**
 * Fetch a single ticket by ID with all replies.
 */
export async function getTicket(
  storeId: string,
  ticketId: string,
  session: SessionHeaders,
): Promise<AdminTicket | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/tickets/${ticketId}`,
    {
      cache: "no-store",
      headers: readHeaders(session),
    },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: getTicket ${res.status}`);
  }
  return (await res.json()) as AdminTicket;
}

/**
 * Create a new support ticket.
 */
export async function createTicket(
  storeId: string,
  input: CreateTicketInput,
  session: SessionHeaders,
): Promise<MutationResult<AdminTicket>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/tickets`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(input),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminTicket };
}

/**
 * Reply to an existing ticket.
 */
export async function replyToTicket(
  storeId: string,
  ticketId: string,
  input: TicketReplyInput,
  session: SessionHeaders,
): Promise<MutationResult<AdminTicketReply>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/tickets/${ticketId}/reply`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(input),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminTicketReply };
}

/**
 * Update a ticket's status (resolve, close, reopen).
 */
export async function updateTicketStatus(
  storeId: string,
  ticketId: string,
  status: TicketStatus,
  session: SessionHeaders,
): Promise<MutationResult<AdminTicket>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/tickets/${ticketId}`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ status }),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminTicket };
}

// ─── B1: Storefront Branding ────────────────────────────────────────

export interface FooterLinkItem {
  label: string;
  kind: "page" | "url";
  page_slug?: string;
  url?: string;
}

export interface FooterSection {
  label: string;
  items: FooterLinkItem[];
}

export interface HomepageHero {
  enabled: boolean;
  image_url?: string | null;
  heading?: string | null;
  subheading?: string | null;
  cta_label?: string | null;
  cta_url?: string | null;
  eyebrow?: string | null;
  cta_secondary_label?: string | null;
  cta_secondary_url?: string | null;
  aside_image_url?: string | null;
  aside_image_alt?: string | null;
}

export type HomepageSection =
  | { type: "text"; heading?: string | null; markdown: string }
  | { type: "image"; url: string; alt?: string | null; caption?: string | null; heading?: string | null }
  | {
      type: "featured_products";
      collection_slug?: string;
      product_slugs?: string[];
      limit?: number;
      heading?: string | null;
    }
  | { type: "quote"; text: string; attribution?: string | null; heading?: string | null }
  | { type: "marquee"; items: string[]; speed?: "slow" | "normal" | "fast" }
  | { type: "pull_quote"; text: string; attribution?: string | null; heading?: string | null }
  | {
      type: "letter";
      eyebrow?: string | null;
      title: string;
      body: string;
      cta_label?: string | null;
      cta_url?: string | null;
    };

export interface HomepageContent {
  hero?: HomepageHero;
  sections?: HomepageSection[];
}

export interface StoreBranding {
  id: string;
  store_id: string;
  logo_url: string | null;
  favicon_url: string | null;
  tagline: string | null;
  color_background: string;
  color_text: string;
  color_accent: string;
  color_button_bg: string;
  color_button_text: string;
  heading_font: string;
  body_font: string;
  layout_variant: string;
  hero_image_url: string | null;
  announcement_text: string | null;
  announcement_link: string | null;
  announcement_bg: string | null;
  announcement_active: boolean;
  footer_tagline: string | null;
  footer_copyright: string | null;
  footer_sections: FooterSection[];
  homepage_content?: HomepageContent;
  social_instagram: string | null;
  social_twitter: string | null;
  social_facebook: string | null;
  social_tiktok: string | null;
  social_youtube: string | null;
  custom_css: string | null;
  show_powered_by: boolean;
  // SEO + AI SEO
  seo_title_template?: string | null;
  seo_default_description?: string | null;
  seo_og_image_url?: string | null;
  seo_twitter_handle?: string | null;
  seo_google_verification?: string | null;
  seo_bing_verification?: string | null;
  seo_json_ld?: string | null;
  seo_ai_policy: "allow" | "deny" | "training-only-denied";
  seo_llms_txt?: string | null;
  return_policy?: string | null;
  created_at: string;
  updated_at: string;
}

export type UpdateBrandingInput = Partial<
  Omit<StoreBranding, "id" | "store_id" | "created_at" | "updated_at">
>;

export async function getBranding(
  storeId: string,
  session: SessionHeaders,
): Promise<StoreBranding | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/branding`,
    {
      cache: "no-store",
      headers: readHeaders(session),
    },
  );
  if (!res.ok) return null;
  return (await res.json()) as StoreBranding;
}

export async function updateBranding(
  storeId: string,
  input: UpdateBrandingInput,
  session: SessionHeaders,
): Promise<MutationResult<StoreBranding>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/branding`,
    {
      method: "PUT",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(input),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as StoreBranding };
}

// ─────────────────────────────────────────────────────────────────────────
// Reviews — admin moderation endpoints backed by marketplace-api
// /api/v1/admin/stores/:storeId/reviews
// ─────────────────────────────────────────────────────────────────────────

export type ReviewStatus = "pending" | "approved" | "rejected";

export interface AdminReviewMedia {
  id: string;
  url: string;
  alt?: string;
  position: number;
  media_type: string;
  width?: number;
  height?: number;
  file_size?: number;
}

export interface AdminReviewReply {
  id: string;
  parent_reply_id?: string | null;
  author_type: string;
  author_name: string;
  author_email?: string;
  content: string;
  created_at: string;
}

export interface AdminReview {
  id: string;
  product_id: string;
  customer_profile_id?: string;
  customer_name: string;
  customer_email: string;
  rating: number;
  title?: string;
  content: string;
  status: ReviewStatus;
  verified_purchase: boolean;
  featured: boolean;
  helpful_count: number;
  not_helpful_count: number;
  published_at?: string;
  created_at: string;
  updated_at: string;
  media: AdminReviewMedia[];
  replies: AdminReviewReply[];
}

export interface ListReviewsQuery {
  status?: ReviewStatus;
  page?: number;
  pageSize?: number;
}

export interface ListReviewsResponse {
  data: AdminReview[];
  meta: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
  };
}

export async function listReviews(
  storeId: string,
  query: ListReviewsQuery,
  session: SessionHeaders,
): Promise<ListReviewsResponse | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/reviews${
    qs ? `?${qs}` : ""
  }`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: readHeaders(session),
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: listReviews ${res.status}: ${
        errBody?.message ?? "unknown error"
      }`,
    );
  }
  return (await res.json()) as ListReviewsResponse;
}

export async function approveReview(
  storeId: string,
  reviewId: string,
  session: SessionHeaders,
): Promise<MutationResult<AdminReview>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/reviews/${reviewId}/approve`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminReview };
}

export async function rejectReview(
  storeId: string,
  reviewId: string,
  session: SessionHeaders,
): Promise<MutationResult<AdminReview>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/reviews/${reviewId}/reject`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminReview };
}

export async function getReview(
  storeId: string,
  reviewId: string,
  session: SessionHeaders,
): Promise<AdminReview | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/reviews/${reviewId}`,
    {
      cache: "no-store",
      headers: readHeaders(session),
    },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: getReview ${res.status}: ${
        errBody?.message ?? "unknown error"
      }`,
    );
  }
  return (await res.json()) as AdminReview;
}

export async function setReviewFeatured(
  storeId: string,
  reviewId: string,
  featured: boolean,
  session: SessionHeaders,
): Promise<MutationResult<AdminReview>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/reviews/${reviewId}/featured`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ featured }),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminReview };
}

export async function replyToReview(
  storeId: string,
  reviewId: string,
  content: string,
  session: SessionHeaders,
): Promise<MutationResult<AdminReview>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/reviews/${reviewId}/reply`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ content }),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as AdminReview };
}

// ─────────────────────────────────────────────────────────────────────────
// Returns — RMA inbox (M5 follow-on).
// ─────────────────────────────────────────────────────────────────────────

export interface AdminReturnItem {
  id: string;
  order_item_id: string;
  quantity: number;
  reason?: string;
}

export type AdminReturnStatus =
  | "requested"
  | "approved"
  | "received"
  | "refunded"
  | "rejected";

export type AdminReturnType = "return" | "replace";

export interface AdminReturn {
  id: string;
  tenant_id: string;
  store_id: string;
  order_id: string;
  return_number: string;
  type: AdminReturnType;
  status: AdminReturnStatus;
  reason?: string;
  notes?: string;
  pickup_details?: string;
  reject_reason?: string;
  refund_amount?: string;
  currency_code: string;
  items: AdminReturnItem[];
  requested_at: string;
  approved_at?: string;
  rejected_at?: string;
  received_at?: string;
  refunded_at?: string;
}

/** List all returns for a store. Backend returns the 100 most recent. */
export async function listReturns(
  storeId: string,
  session: SessionHeaders,
): Promise<AdminReturn[] | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/returns`,
    { cache: "no-store", headers: commonHeaders(session) },
  );
  if (!res.ok) return null;
  const body = (await res.json()) as { data?: AdminReturn[] };
  return body.data ?? [];
}

export async function getReturn(
  storeId: string,
  returnId: string,
  session: SessionHeaders,
): Promise<AdminReturn | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/returns/${returnId}`,
    { cache: "no-store", headers: commonHeaders(session) },
  );
  if (!res.ok) return null;
  return (await res.json()) as AdminReturn;
}

export async function approveReturn(
  storeId: string,
  returnId: string,
  pickupDetails: string,
  session: SessionHeaders,
): Promise<MutationResult<AdminReturn>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/returns/${returnId}/approve`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ pickup_details: pickupDetails }),
    },
  );
  if (!res.ok) return { ok: false, error: await parseMutationError(res) };
  return { ok: true, data: (await res.json()) as AdminReturn };
}

export async function rejectReturn(
  storeId: string,
  returnId: string,
  reason: string,
  session: SessionHeaders,
): Promise<MutationResult<AdminReturn>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/returns/${returnId}/reject`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ reason }),
    },
  );
  if (!res.ok) return { ok: false, error: await parseMutationError(res) };
  return { ok: true, data: (await res.json()) as AdminReturn };
}

export async function setReturnPickup(
  storeId: string,
  returnId: string,
  pickupDetails: string,
  session: SessionHeaders,
): Promise<MutationResult<AdminReturn>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/returns/${returnId}/pickup`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ pickup_details: pickupDetails }),
    },
  );
  if (!res.ok) return { ok: false, error: await parseMutationError(res) };
  return { ok: true, data: (await res.json()) as AdminReturn };
}
