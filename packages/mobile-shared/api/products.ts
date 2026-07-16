import type { createApiClient } from "./client";
import {
  productDetailSchema,
  productListSchema,
  productMediaSchema,
  mediaUploadUrlSchema,
  type Product,
  type ProductDetail,
  type ProductListResponse,
  type ProductMedia,
  type MediaUploadUrl,
} from "./schemas/products";

export interface ListProductsParams {
  /** draft | active | archived. "inactive" is a 400 — it is not a real status. */
  status?: string;
  search?: string;
  page?: string;
  page_size?: string;
}

/**
 * CreateProductRequest (validation.go:231-249) requires `title` and at least
 * one variant, each with a required `sku` and `price`. The old body — name /
 * price / stock — was an unconditional 400.
 */
export interface CreateProductVariantBody {
  sku: string;
  price: number;
  currency_code?: string;
  inventory_quantity?: number;
  position?: number;
}

export interface CreateProductBody {
  title: string;
  description?: string;
  status?: string;
  tags?: string[];
  variants: CreateProductVariantBody[];
}

/**
 * REQUEST shape for a product option (CreateProductOptionInput,
 * validation.go:251-254). `values` is `string[]` HERE.
 *
 * The RESPONSE (productOptionSchema) sends `[{id, value, position}]` for the
 * same field name. These are two different shapes and must never be swapped —
 * doing so would blank the product list the moment any product has options.
 */
export interface UpdateProductOptionBody {
  name: string;
  values: string[];
}

/**
 * PATCH /products/:id body (UpdateProductRequest, validation.go:296).
 *
 * `variants` is deliberately absent. UpdateAggregateRequest.Variants is a FULL
 * DESIRED MATRIX — applyVariantsDiff soft-deletes any existing variant missing
 * from it. Variant edits go through updateVariant() instead. Do not add it here.
 *
 * Sending `options`, `removed_variant_ids` or `category_ids` routes the handler
 * through the aggregate path (products.go:172); a body of scalars alone routes
 * through basics. Send only what changed.
 */
export interface UpdateProductBody {
  title?: string;
  description?: string;
  status?: string;
  tags?: string[];
  options?: UpdateProductOptionBody[];
  removed_variant_ids?: string[];
  category_ids?: string[];
  primary_category_id?: string;
}

/**
 * UpdateVariantRequest (validation.go:43-58) — the variant quick-PATCH. There
 * is no `stock` field; it is `inventory_quantity`.
 *
 * This endpoint accepts SKU and all the shipping fields, which is why weight
 * and dimensions need no aggregate call.
 */
export interface UpdateVariantBody {
  sku?: string;
  barcode?: string;
  price?: number;
  compare_at_price?: number;
  cost_price?: number;
  weight_grams?: number;
  length_cm?: number;
  width_cm?: number;
  height_cm?: number;
  inventory_quantity?: number;
  /** deny | continue */
  inventory_policy?: string;
  low_stock_threshold?: number;
  position?: number;
}

/** Request body for `POST /products/{id}/media/upload-url`. */
export interface CreateMediaUploadUrlBody {
  content_hash: string;
  filename: string;
  content_type: string;
}

/**
 * Request body for `POST /products/{id}/media`. `url` must be the raw
 * `storage_key` — the backend builds the public CDN URL itself and ignores
 * whatever else is sent here (`service_single_media.go:91-97`). Never send a
 * CDN URL in this field.
 */
export interface CreateMediaBody {
  storage_key: string;
  url: string;
  position: number;
  media_type?: string;
  alt?: string;
}

/**
 * PATCH /products/{id}/media/{mediaId} (UpdateMediaWireRequest,
 * validation.go:85). Returns 204 No Content — there is no body to parse, so
 * no schema is passed. Reorder is a `position` patch; position 0 is the hero.
 */
export interface UpdateMediaBody {
  alt?: string;
  position?: number;
}

export function createProductsApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListProductsParams) =>
      client.get<ProductListResponse>(
        "/products",
        params as Record<string, string>,
        productListSchema,
      ),
    get: (id: string) =>
      client.get<ProductDetail>(`/products/${id}`, undefined, productDetailSchema),
    create: (body: CreateProductBody) =>
      client.post<ProductDetail>("/products", body, productDetailSchema),
    update: (id: string, body: UpdateProductBody) =>
      client.patch<ProductDetail>(`/products/${id}`, body, productDetailSchema),
    /**
     * `inventory_quantity`, NOT `stock`. UpdateVariantRequest has no `stock`
     * field, so the old body's stock edits were silently discarded with a 200.
     */
    updateVariant: (productId: string, variantId: string, body: UpdateVariantBody) =>
      client.patch(`/products/${productId}/variants/${variantId}`, body),
    deleteMedia: (productId: string, mediaId: string) =>
      client.delete(`/products/${productId}/media/${mediaId}`),
    createMediaUploadUrl: (productId: string, body: CreateMediaUploadUrlBody) =>
      client.post<MediaUploadUrl>(
        `/products/${productId}/media/upload-url`,
        body,
        mediaUploadUrlSchema,
      ),
    createMedia: (productId: string, body: CreateMediaBody) =>
      client.post<ProductMedia>(`/products/${productId}/media`, body, productMediaSchema),
    updateMedia: (productId: string, mediaId: string, body: UpdateMediaBody) =>
      client.patch(`/products/${productId}/media/${mediaId}`, body),
  };
}

export type { Product, ProductDetail, ProductMedia, MediaUploadUrl };
