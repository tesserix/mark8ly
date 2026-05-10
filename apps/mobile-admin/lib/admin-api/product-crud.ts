import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createProductsApi,
  type CreateProductBody,
} from "@repo/mobile-shared/api/products";
import { useApiClient } from "@/lib/api-client";

export function useCreateProduct() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: CreateProductBody) => productsApi.create(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["products"] });
    },
  });
}

export function useUpdateProduct() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: string;
      body: Record<string, unknown>;
    }) => productsApi.update(id, body),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ["products"] });
      queryClient.invalidateQueries({ queryKey: ["product", variables.id] });
    },
  });
}

export function useUploadMedia() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ productId, uri }: { productId: string; uri: string }) =>
      productsApi.uploadMedia(productId, uri),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["product", variables.productId],
      });
    },
  });
}

export function useDeleteMedia() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      productId,
      mediaId,
    }: {
      productId: string;
      mediaId: string;
    }) => productsApi.deleteMedia(productId, mediaId),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["product", variables.productId],
      });
    },
  });
}

export function useCreateVariant() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      productId,
      body,
    }: {
      productId: string;
      body: { name: string; sku?: string; price: number; stock: number };
    }) => productsApi.createVariant(productId, body),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["product", variables.productId],
      });
    },
  });
}

export function useUpdateVariant() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      productId,
      variantId,
      body,
    }: {
      productId: string;
      variantId: string;
      body: Record<string, unknown>;
    }) => productsApi.updateVariant(productId, variantId, body),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["product", variables.productId],
      });
    },
  });
}
