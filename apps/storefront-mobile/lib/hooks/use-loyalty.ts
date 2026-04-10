import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApiClient, useStoreSlug } from "./use-api-client";
import {
  getLoyaltyProgram,
  getLoyaltyMe,
  enrollLoyalty,
} from "@/lib/storefront-api/loyalty";

export function useLoyaltyProgram() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery({
    queryKey: ["store", slug, "loyalty-program"],
    queryFn: () => getLoyaltyProgram(client),
    select: (res) => res.data,
  });
}

export function useLoyaltyMembership() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery({
    queryKey: ["store", slug, "loyalty-me"],
    queryFn: () => getLoyaltyMe(client),
    select: (res) => res.data,
  });
}

export function useEnrollLoyalty() {
  const client = useApiClient();
  const slug = useStoreSlug();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => enrollLoyalty(client),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["store", slug, "loyalty-me"] });
    },
  });
}
