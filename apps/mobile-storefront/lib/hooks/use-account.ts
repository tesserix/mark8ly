import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import type {
  CustomerProfile,
  StorefrontAddress,
} from "@repo/mobile-shared/api/storefront-types";
import { useStorefrontApi } from "@/lib/api-client";

export function useProfile() {
  const api = useStorefrontApi();
  const { user } = useAuth();
  return useQuery<CustomerProfile>({
    queryKey: ["profile"],
    queryFn: () => api.get<CustomerProfile>("/account"),
    enabled: !!user,
  });
}

export function useUpdateProfile() {
  const api = useStorefrontApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: Partial<Pick<CustomerProfile, "first_name" | "last_name" | "phone">>) =>
      api.patch<CustomerProfile>("/account", body),
    onSuccess: (data) => {
      queryClient.setQueryData(["profile"], data);
    },
  });
}

interface AddressesResponse {
  items: StorefrontAddress[];
}

export function useAddresses() {
  const api = useStorefrontApi();
  const { user } = useAuth();
  return useQuery<AddressesResponse>({
    queryKey: ["addresses"],
    queryFn: () => api.get<AddressesResponse>("/account/addresses"),
    enabled: !!user,
  });
}

export type AddressInput = Omit<StorefrontAddress, "id">;

export function useCreateAddress() {
  const api = useStorefrontApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: AddressInput) =>
      api.post<StorefrontAddress>("/account/addresses", body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["addresses"] });
    },
  });
}

export function useUpdateAddress() {
  const api = useStorefrontApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & Partial<AddressInput>) =>
      api.patch<StorefrontAddress>(`/account/addresses/${id}`, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["addresses"] });
    },
  });
}

export function useDeleteAddress() {
  const api = useStorefrontApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/account/addresses/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["addresses"] });
    },
  });
}
