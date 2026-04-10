import type { createApiClient } from "./client";
import type { Product, ProductDetail, PaginatedResponse } from "./types";

export interface ListProductsParams {
  status?: string;
  low_stock?: string;
  search?: string;
  cursor?: string;
  limit?: string;
}

export interface CreateProductBody {
  name: string;
  description?: string;
  price: number;
  compare_at_price?: number;
  sku?: string;
  stock: number;
  category_id?: string;
  tags?: string[];
  status: string;
}

export function createProductsApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListProductsParams) =>
      client.get<PaginatedResponse<Product>>("/products", params as Record<string, string>),
    get: (id: string) => client.get<ProductDetail>(`/products/${id}`),
    create: (body: CreateProductBody) => client.post<ProductDetail>("/products", body),
    update: (id: string, body: Partial<CreateProductBody>) =>
      client.patch<ProductDetail>(`/products/${id}`, body),
    uploadMedia: async (productId: string, uri: string) => {
      const formData = new FormData();
      const filename = uri.split("/").pop() ?? "photo.jpg";
      formData.append("file", { uri, name: filename, type: "image/jpeg" } as unknown as Blob);
      return client.uploadMedia(`/products/${productId}/media`, formData);
    },
    deleteMedia: (productId: string, mediaId: string) =>
      client.delete(`/products/${productId}/media/${mediaId}`),
    reorderMedia: (productId: string, mediaIds: string[]) =>
      client.patch(`/products/${productId}/media/reorder`, { media_ids: mediaIds }),
    listVariants: (productId: string) => client.get(`/products/${productId}/variants`),
    createVariant: (productId: string, body: { name: string; sku?: string; price: number; stock: number }) =>
      client.post(`/products/${productId}/variants`, body),
    updateVariant: (productId: string, variantId: string, body: { price?: number; stock?: number }) =>
      client.patch(`/products/${productId}/variants/${variantId}`, body),
  };
}
