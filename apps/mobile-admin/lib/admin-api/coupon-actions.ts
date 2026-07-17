import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createCouponsApi,
  type CreateCouponBody,
  type PatchCouponBody,
} from "@repo/mobile-shared/api/coupons";
import { useApiClient } from "@/lib/api-client";

/** Every coupon mutation invalidates the ["coupons"] prefix (list + detail). */
function useCouponMutation<TVars>(
  run: (api: ReturnType<typeof createCouponsApi>, vars: TVars) => Promise<unknown>,
) {
  const client = useApiClient();
  const couponsApi = createCouponsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (vars: TVars) => run(couponsApi, vars),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["coupons"] });
    },
  });
}

export function useCreateCoupon() {
  return useCouponMutation<CreateCouponBody>((api, body) => api.create(body));
}

export function usePatchCoupon() {
  return useCouponMutation<{ id: string; body: PatchCouponBody }>((api, { id, body }) =>
    api.patch(id, body),
  );
}

export function useDeleteCoupon() {
  return useCouponMutation<string>((api, id) => api.remove(id));
}
