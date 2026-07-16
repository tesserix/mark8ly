import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createProductsApi,
  type CreateProductBody,
  type UpdateProductBody,
  type UpdateVariantBody,
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
    mutationFn: ({ id, body }: { id: string; body: UpdateProductBody }) =>
      productsApi.update(id, body),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ["products"] });
      queryClient.invalidateQueries({ queryKey: ["product", variables.id] });
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
      body: UpdateVariantBody;
    }) => productsApi.updateVariant(productId, variantId, body),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ["product", variables.productId] });
    },
  });
}
