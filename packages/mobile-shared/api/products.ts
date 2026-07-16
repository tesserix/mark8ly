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

export interface UpdateProductBody {
  title?: string;
  description?: string;
  status?: string;
  tags?: string[];
}

/** UpdateVariantRequest (validation.go:43-55). There is no `stock` field. */
export interface UpdateVariantBody {
  sku?: string;
  price?: number;
  inventory_quantity?: number;
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
  };
}

export type { Product, ProductDetail, ProductMedia, MediaUploadUrl };
