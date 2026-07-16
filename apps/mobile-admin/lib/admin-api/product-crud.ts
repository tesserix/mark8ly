import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as FileSystem from "expo-file-system/legacy";
import {
  createProductsApi,
  type CreateProductBody,
  type UpdateProductBody,
  type UpdateVariantBody,
  type ProductMedia,
} from "@repo/mobile-shared/api/products";
import { createCategoriesApi } from "@repo/mobile-shared/api/categories";
import { useApiClient } from "@/lib/api-client";
import {
  computeContentHash,
  inferContentType,
  type PickedMediaAsset,
} from "@/lib/media-upload";

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

export interface AddProductMediaArgs {
  productId: string;
  asset: PickedMediaAsset;
  position: number;
  alt?: string;
}

/**
 * PUTs the raw file bytes to the GCS signed URL from step 1. Injectable so
 * tests never touch the network. This step must NOT go through the api
 * client — it targets `storage.googleapis.com`, not our API, and must not
 * carry our `Authorization` header.
 */
export type UploadFn = (
  signedUrl: string,
  fileUri: string,
  contentType: string,
) => Promise<{ status: number }>;

async function defaultUploadFn(
  signedUrl: string,
  fileUri: string,
  contentType: string,
): Promise<{ status: number }> {
  const result = await FileSystem.uploadAsync(signedUrl, fileUri, {
    httpMethod: "PUT",
    uploadType: FileSystem.FileSystemUploadType.BINARY_CONTENT,
    headers: { "Content-Type": contentType },
  });
  return { status: result.status };
}

/**
 * The only api-client methods this orchestration needs. Narrowed (rather
 * than the full `ReturnType<typeof createProductsApi>`) so tests can pass a
 * plain object of two `jest.fn()`s with no cast.
 */
type MediaApi = Pick<ReturnType<typeof createProductsApi>, "createMediaUploadUrl" | "createMedia">;

/**
 * The 3-step media upload recipe, verified end-to-end against prod with curl
 * (see the media brief): request a signed URL, PUT the bytes to GCS, then
 * finalize the media row.
 *
 * Step 3 sends `url: storage_key` — NOT a CDN URL. The backend builds the
 * public URL itself from `storage_key` and ignores whatever this field
 * contains whenever it has its own base URL configured
 * (`service_single_media.go:91-97`). Hardcoding a CDN URL here would be dead
 * code that merely looks load-bearing.
 */
export async function uploadProductMedia(
  productsApi: MediaApi,
  args: AddProductMediaArgs,
  uploadFn: UploadFn = defaultUploadFn,
): Promise<ProductMedia> {
  const contentType = inferContentType(args.asset.mimeType);
  const contentHash = computeContentHash(args.asset, Date.now());

  const signed = await productsApi.createMediaUploadUrl(args.productId, {
    content_hash: contentHash,
    filename: args.asset.fileName ?? "upload.jpg",
    content_type: contentType,
  });

  const putResult = await uploadFn(signed.url, args.asset.uri, contentType);
  if (putResult.status < 200 || putResult.status >= 300) {
    throw new Error(`Image upload failed (status ${putResult.status}). Please try again.`);
  }

  return productsApi.createMedia(args.productId, {
    storage_key: signed.storage_key,
    // MUST be the raw storage key, never a CDN URL — see doc comment above.
    url: signed.storage_key,
    position: args.position,
    media_type: "image",
    alt: args.alt,
  });
}

export function useAddProductMedia() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (args: AddProductMediaArgs) => uploadProductMedia(productsApi, args),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ["product", variables.productId] });
    },
  });
}

/**
 * Categories change rarely and the picker needs them on every product edit —
 * a 5-minute staleTime keeps the sheet instant without going stale enough to
 * mislead.
 */
export function useCategories() {
  const client = useApiClient();
  const categoriesApi = createCategoriesApi(client);

  return useQuery({
    queryKey: ["categories"],
    queryFn: async () => {
      const res = await categoriesApi.list();
      return res.data;
    },
    staleTime: 5 * 60 * 1000,
  });
}
