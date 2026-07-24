import { useMutation } from "@tanstack/react-query";
import { createAccountApi } from "@repo/mobile-shared/api/account";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useApiClient } from "@/lib/api-client";

export function useDeleteAccount() {
  const client = useApiClient();
  const api = createAccountApi(client);
  const { signOut, refreshToken } = useAuth();

  return useMutation({
    mutationFn: async () => {
      await refreshToken(); // fresh GIP id_token so the server call authenticates
      await api.deleteAccount();
    },
    onSuccess: async () => {
      await signOut(); // AuthGate redirects to /login + clears cache
    },
  });
}
