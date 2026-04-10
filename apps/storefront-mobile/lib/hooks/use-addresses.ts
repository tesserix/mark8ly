import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApiClient, useStoreSlug } from "./use-api-client";
import {
  listAddresses,
  createAddress,
  updateAddress,
  deleteAddress,
} from "@/lib/storefront-api/addresses";
import type { AddressInput } from "@/lib/storefront-api/addresses";

export function useAddresses() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery({
    queryKey: ["store", slug, "addresses"],
    queryFn: () => listAddresses(client),
    select: (res) => res.data,
  });
}

export function useCreateAddress() {
  const client = useApiClient();
  const slug = useStoreSlug();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: AddressInput) => createAddress(client, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["store", slug, "addresses"] });
    },
  });
}

export function useUpdateAddress() {
  const client = useApiClient();
  const slug = useStoreSlug();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<AddressInput> }) =>
      updateAddress(client, id, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["store", slug, "addresses"] });
    },
  });
}

export function useDeleteAddress() {
  const client = useApiClient();
  const slug = useStoreSlug();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => deleteAddress(client, id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["store", slug, "addresses"] });
    },
  });
}
